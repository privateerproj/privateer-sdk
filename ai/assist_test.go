package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gemaraproj/go-gemara"
)

// stubClient is an in-package Client for exercising Assist without a network.
type stubClient struct {
	resp *AnalyzeResponse
	err  error

	gotPrompt  string
	gotContent string
	gotSchema  *Schema
}

func (s *stubClient) Analyze(_ context.Context, prompt, content string, schema *Schema) (*AnalyzeResponse, error) {
	s.gotPrompt, s.gotContent, s.gotSchema = prompt, content, schema
	return s.resp, s.err
}

func jsonResp(body string) *AnalyzeResponse {
	return &AnalyzeResponse{
		JSON: json.RawMessage(body),
		Metadata: ResponseMetadata{
			Provider:  ProviderOpenAI,
			Model:     "gpt-4o-mini",
			RequestID: "req-123",
		},
	}
}

func TestAssist_ParsesVerdictAndBuildsEvidence(t *testing.T) {
	client := &stubClient{resp: jsonResp(
		`{"result":"pass","confidence":"high","reasoning":"README documents a user guide","citations":["README.md"]}`)}

	verdict, ev, err := Assist(context.Background(), client, Question{
		Prompt:  "Does this repo document a user guide?",
		Content: "README body",
	})
	if err != nil {
		t.Fatalf("Assist: %v", err)
	}

	// The SDK-owned schema is what reaches the provider, not something the caller wrote.
	if client.gotSchema != verdictSchema {
		t.Errorf("expected verdictSchema to be sent, got %#v", client.gotSchema)
	}
	if client.gotPrompt != "Does this repo document a user guide?" || client.gotContent != "README body" {
		t.Errorf("prompt/content not forwarded verbatim: %q / %q", client.gotPrompt, client.gotContent)
	}

	if verdict.Result != "pass" || verdict.Confidence != "high" {
		t.Errorf("verdict = %+v", verdict)
	}
	if verdict.GemaraResult() != gemara.Passed || verdict.GemaraConfidence() != gemara.High {
		t.Errorf("mapping = %v / %v", verdict.GemaraResult(), verdict.GemaraConfidence())
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
	if ev.Description != "README documents a user guide" {
		t.Errorf("description = %q, want reasoning fallback", ev.Description)
	}
	payload, ok := ev.Payload.(EvidencePayload)
	if !ok {
		t.Fatalf("payload type = %T, want EvidencePayload", ev.Payload)
	}
	if payload.Verdict.Result != "pass" || payload.Model != "gpt-4o-mini" || payload.RequestID != "req-123" {
		t.Errorf("payload = %+v", payload)
	}
	// The question itself is preserved so the verdict is auditable without
	// provider-side request logs.
	if payload.Prompt != "Does this repo document a user guide?" || payload.Content != "README body" {
		t.Errorf("payload prompt/content = %q / %q, want the question verbatim", payload.Prompt, payload.Content)
	}
}

func TestAssist_DescriptionOverride(t *testing.T) {
	client := &stubClient{resp: jsonResp(`{"result":"fail","confidence":"medium","reasoning":"none found"}`)}

	_, ev, err := Assist(context.Background(), client, Question{
		Prompt:      "check",
		Description: "AI follow-up for OSPS-DO-01",
	})
	if err != nil {
		t.Fatalf("Assist: %v", err)
	}
	if ev.Description != "AI follow-up for OSPS-DO-01" {
		t.Errorf("description = %q, want explicit override", ev.Description)
	}
}

func TestAssist_ProviderErrorYieldsNeedsReview(t *testing.T) {
	client := &stubClient{err: errors.New("boom")}

	verdict, ev, err := Assist(context.Background(), client, Question{Prompt: "check"})
	if err == nil {
		t.Fatal("expected error on provider failure")
	}
	if verdict.GemaraResult() != gemara.NeedsReview {
		t.Errorf("result = %v, want NeedsReview so a failed check never passes", verdict.GemaraResult())
	}
	if ev.Type != EvidenceType {
		t.Errorf("expected an evidence record even on failure, got type %q", ev.Type)
	}
}

func TestAssist_NilResponseYieldsNeedsReview(t *testing.T) {
	// An adapter that returns (nil, nil) must degrade to needs_review, not panic.
	client := &stubClient{}

	verdict, ev, err := Assist(context.Background(), client, Question{Prompt: "check"})
	if err == nil {
		t.Fatal("expected error for nil response")
	}
	if verdict.GemaraResult() != gemara.NeedsReview {
		t.Errorf("result = %v, want NeedsReview", verdict.GemaraResult())
	}
	if ev.Type != EvidenceType {
		t.Errorf("expected an evidence record even on nil response, got type %q", ev.Type)
	}
}

func TestAssist_InvalidJSONYieldsNeedsReview(t *testing.T) {
	client := &stubClient{resp: jsonResp(`not json`)}

	verdict, _, err := Assist(context.Background(), client, Question{Prompt: "check"})
	if err == nil {
		t.Fatal("expected parse error")
	}
	if verdict.GemaraResult() != gemara.NeedsReview {
		t.Errorf("result = %v, want NeedsReview", verdict.GemaraResult())
	}
}

func TestAssist_NilClient(t *testing.T) {
	verdict, _, err := Assist(context.Background(), nil, Question{Prompt: "check"})
	if err == nil {
		t.Fatal("expected error for nil client")
	}
	if verdict.GemaraResult() != gemara.NeedsReview {
		t.Errorf("result = %v, want NeedsReview", verdict.GemaraResult())
	}
}

func TestAssist_DryRunIsCleanNoSpendPath(t *testing.T) {
	client, err := NewClient(Config{Provider: ProviderOpenAI, Model: "gpt-4o-mini", DryRun: true})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	verdict, ev, err := Assist(context.Background(), client, Question{Prompt: "check", Content: "x"})
	if err != nil {
		t.Fatalf("dry-run Assist should not error: %v", err)
	}
	if verdict.GemaraResult() != gemara.NeedsReview {
		t.Errorf("dry-run result = %v, want NeedsReview", verdict.GemaraResult())
	}
	if ev.Type != EvidenceType {
		t.Errorf("expected an evidence record for the dry-run attempt, got %q", ev.Type)
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
		verdict, ev, err := Assist(context.Background(), client, Question{Prompt: "docs?", Content: "README"})
		if err != nil {
			return gemara.Unknown, err.Error(), gemara.Undetermined
		}
		target.AddEvidence(ev)
		return verdict.GemaraResult(), verdict.Reasoning, verdict.GemaraConfidence()
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
