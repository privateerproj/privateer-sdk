package assist

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gemaraproj/go-gemara"
	"github.com/privateerproj/privateer-sdk/ai/provider"
)

// stubClient is an in-package Client for exercising Assist without a network.
type stubClient struct {
	resp *provider.AnalyzeResponse
	err  error

	gotPrompt  string
	gotContent string
	gotSchema  *provider.Schema
}

func (s *stubClient) Analyze(_ context.Context, prompt, content string, schema *provider.Schema) (*provider.AnalyzeResponse, error) {
	s.gotPrompt, s.gotContent, s.gotSchema = prompt, content, schema
	return s.resp, s.err
}

func jsonResp(body string) *provider.AnalyzeResponse {
	return &provider.AnalyzeResponse{
		JSON: json.RawMessage(body),
		Metadata: provider.ResponseMetadata{
			Provider:  provider.Provider("openai"),
			Model:     "gpt-4o-mini",
			RequestID: "req-123",
		},
	}
}

func TestAssist_ParsesResponseAndBuildsEvidence(t *testing.T) {
	client := &stubClient{resp: jsonResp(
		`{"result":"pass","confidence":"high","reasoning":"README documents a user guide","citations":["README.md"]}`)}

	response, ev, err := Assist(context.Background(), client, Question{
		Prompt:   "Does this repo document a user guide?",
		Material: "README body",
	})
	if err != nil {
		t.Fatalf("Assist: %v", err)
	}

	// The SDK-owned schema is what reaches the provider, not something the caller wrote.
	if client.gotSchema != responseSchema {
		t.Errorf("expected responseSchema to be sent, got %#v", client.gotSchema)
	}
	if client.gotPrompt != "Does this repo document a user guide?" || client.gotContent != "README body" {
		t.Errorf("prompt/content not forwarded verbatim: %q / %q", client.gotPrompt, client.gotContent)
	}

	if response.Result != "pass" || response.Confidence != "high" {
		t.Errorf("response = %+v", response)
	}
	if response.GemaraResult() != gemara.Passed || response.GemaraConfidence() != gemara.High {
		t.Errorf("mapping = %v / %v", response.GemaraResult(), response.GemaraConfidence())
	}

	if ev.Type != EvidenceType {
		t.Errorf("evidence type = %q, want %q", ev.Type, EvidenceType)
	}
	if ev.Id != "req-123" {
		t.Errorf("evidence id = %q, want provider request id", ev.Id)
	}
	if ev.CollectedAt == "" {
		t.Error("expected CollectedAt to be stamped")
	}
	payload, ok := ev.Payload.(EvidencePayload)
	if !ok {
		t.Fatalf("payload type = %T, want EvidencePayload", ev.Payload)
	}
	if payload.Response.Result != "pass" || payload.Model != "gpt-4o-mini" || payload.RequestID != "req-123" {
		t.Errorf("payload = %+v", payload)
	}
	// The question itself is preserved so the response is auditable without
	// provider-side request logs.
	if payload.Prompt != "Does this repo document a user guide?" || payload.Material != "README body" {
		t.Errorf("payload prompt/material = %q / %q, want the question verbatim", payload.Prompt, payload.Material)
	}
}

func TestAssist_ProviderErrorYieldsNeedsReview(t *testing.T) {
	client := &stubClient{err: errors.New("boom")}

	response, ev, err := Assist(context.Background(), client, Question{Prompt: "check"})
	if err == nil {
		t.Fatal("expected error on provider failure")
	}
	if response.GemaraResult() != gemara.NeedsReview {
		t.Errorf("result = %v, want NeedsReview so a failed check never passes", response.GemaraResult())
	}
	// No response was ever obtained, so there is nothing to record as evidence.
	if ev.Type != "" {
		t.Errorf("expected no evidence record on provider error, got type %q", ev.Type)
	}
}

func TestAssist_NilResponseYieldsNeedsReview(t *testing.T) {
	// An adapter that returns (nil, nil) must degrade to needs_review, not panic.
	client := &stubClient{}

	response, ev, err := Assist(context.Background(), client, Question{Prompt: "check"})
	if err == nil {
		t.Fatal("expected error for nil response")
	}
	if response.GemaraResult() != gemara.NeedsReview {
		t.Errorf("result = %v, want NeedsReview", response.GemaraResult())
	}
	if ev.Type != "" {
		t.Errorf("expected no evidence record for a nil response, got type %q", ev.Type)
	}
}

func TestAssist_InvalidJSONYieldsNeedsReview(t *testing.T) {
	client := &stubClient{resp: jsonResp(`not json`)}

	response, _, err := Assist(context.Background(), client, Question{Prompt: "check"})
	if err == nil {
		t.Fatal("expected parse error")
	}
	if response.GemaraResult() != gemara.NeedsReview {
		t.Errorf("result = %v, want NeedsReview", response.GemaraResult())
	}
}

// GemaraResult must match verdicts exactly: a value that merely contains
// "pass" or "fail" (possible when callers build Response by hand) must fall
// through to NeedsReview, never to Passed.
func TestGemaraResult_ExactMatchOnly(t *testing.T) {
	cases := map[string]gemara.Result{
		"pass":         gemara.Passed,
		" PASS ":       gemara.Passed,
		"fail":         gemara.Failed,
		"surpass":      gemara.NeedsReview,
		"failed":       gemara.NeedsReview,
		"needs_review": gemara.NeedsReview,
		"":             gemara.NeedsReview,
	}
	for verdict, want := range cases {
		if got := (Response{Result: verdict}).GemaraResult(); got != want {
			t.Errorf("GemaraResult(%q) = %v, want %v", verdict, got, want)
		}
	}
}

func TestAssist_NilClient(t *testing.T) {
	response, _, err := Assist(context.Background(), nil, Question{Prompt: "check"})
	if err == nil {
		t.Fatal("expected error for nil client")
	}
	if response.GemaraResult() != gemara.NeedsReview {
		t.Errorf("result = %v, want NeedsReview", response.GemaraResult())
	}
}

// assistTarget is a gemara payload that collects evidence via method promotion.
type assistTarget struct {
	gemara.EvidenceCollector
}

// TestAssist_FlowsIntoAssessmentLog exercises the intended use case: a step runs
// an AI-assisted check, records the returned Evidence into the payload at its own
// discretion, and the AssessmentLog harvests it — no bespoke on-disk packet.
func TestAssist_FlowsIntoAssessmentLog(t *testing.T) {
	client := &stubClient{resp: jsonResp(
		`{"result":"pass","confidence":"high","reasoning":"guide documented in README"}`)}

	step := func(payload any) (gemara.Result, string, gemara.ConfidenceLevel) {
		target := payload.(*assistTarget)
		response, ev, err := Assist(context.Background(), client, Question{Prompt: "docs?", Material: "README"})
		if err != nil {
			return gemara.Unknown, err.Error(), gemara.Undetermined
		}
		target.AddEvidence(ev)
		return response.GemaraResult(), response.Reasoning, response.GemaraConfidence()
	}

	assessment, err := gemara.NewAssessment(
		"OSPS-DO-01", "user guide is documented", []string{"tlp_green"},
		[]gemara.AssessmentStep{step},
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}

	if result := assessment.Run(&assistTarget{}); result != gemara.Passed {
		t.Fatalf("assessment result = %v, want Passed", result)
	}
	if len(assessment.Evidence) != 1 {
		t.Fatalf("harvested %d evidence records, want 1", len(assessment.Evidence))
	}
	if assessment.Evidence[0].Type != EvidenceType {
		t.Errorf("harvested evidence type = %q, want %q", assessment.Evidence[0].Type, EvidenceType)
	}
}
