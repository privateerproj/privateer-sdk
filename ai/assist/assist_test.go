package assist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
		`{"result":"pass","confidence":"high","message":"A user guide is documented in the README","explanation":"README documents a user guide under Docs","citations":["README.md"]}`)}

	response, ev, err := Assist(context.Background(), client, Question{
		Prompt:   "Does this repo document a user guide?",
		Material: "README body",
	})
	if err != nil {
		t.Fatalf("Assist: %v", err)
	}

	// The SDK-owned schema is what reaches the provider, not something the
	// caller wrote, and it carries the default explanation budget.
	if client.gotSchema == nil || client.gotSchema.Name != "assessment_verdict" {
		t.Fatalf("expected the SDK-owned schema to be sent, got %#v", client.gotSchema)
	}
	if !strings.Contains(string(client.gotSchema.Value), "1500 characters") {
		t.Errorf("schema should state the default explanation budget: %s", client.gotSchema.Value)
	}
	if client.gotPrompt != "Does this repo document a user guide?" || client.gotContent != "README body" {
		t.Errorf("prompt/content not forwarded verbatim: %q / %q", client.gotPrompt, client.gotContent)
	}

	if response.Result != "pass" || response.Confidence != "high" {
		t.Errorf("response = %+v", response)
	}
	if response.Message != "A user guide is documented in the README" {
		t.Errorf("message = %q", response.Message)
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

// Summary must always be a single line so assessment messages keep the same
// shape as every other step's message, whatever the model returned.
func TestResponseSummary(t *testing.T) {
	cases := map[string]struct {
		response Response
		want     string
	}{
		"message preferred": {Response{Result: "fail", Message: "No evidence found for when tests are run."}, "[AI-Assisted] No evidence found for when tests are run."},
		"verdict fallback":  {Response{Result: "fail", Confidence: "medium"}, "[AI-Assisted] verdict: fail (medium confidence)"},
		"no confidence":     {Response{Result: "pass"}, "[AI-Assisted] verdict: pass"},
		"zero value":        {Response{}, "[AI-Assisted] verdict: needs_review"},
		"unclean input":     {Response{Result: " FAIL ", Confidence: "High"}, "[AI-Assisted] verdict: fail (high confidence)"},
	}
	for name, tc := range cases {
		got := tc.response.Summary()
		if got != tc.want {
			t.Errorf("%s: Summary() = %q, want %q", name, got, tc.want)
		}
		if strings.Contains(got, "\n") {
			t.Errorf("%s: Summary() must be a single line, got %q", name, got)
		}
	}
}

// TestAssist_EnforcesTextBudgets covers the strict-shape mechanism: the char
// budgets are requested via the schema but guaranteed in code, so a model that
// ignores them still cannot produce a multi-line or oversized Message, and the
// Explanation budget set by the plugin flows into the schema and the cap.
func TestAssist_EnforcesTextBudgets(t *testing.T) {
	longMessage := "line one\nline two   " + strings.Repeat("x", 300)
	longExplanation := strings.Repeat("e", 300)
	client := &stubClient{resp: jsonResp(fmt.Sprintf(
		`{"result":"fail","confidence":"low","message":%q,"explanation":%q,"citations":null}`,
		longMessage, longExplanation))}

	response, _, err := Assist(context.Background(), client, Question{
		Prompt:              "check",
		Material:            "material",
		MaxExplanationChars: 100,
	})
	if err != nil {
		t.Fatalf("Assist: %v", err)
	}

	if !strings.Contains(string(client.gotSchema.Value), "100 characters") {
		t.Errorf("schema should carry the caller's explanation budget: %s", client.gotSchema.Value)
	}
	if strings.Contains(response.Message, "\n") {
		t.Errorf("message must be single-line, got %q", response.Message)
	}
	if got := len([]rune(response.Message)); got > 160 {
		t.Errorf("message length = %d runes, want <= 160", got)
	}
	if got := len([]rune(response.Explanation)); got > 100 {
		t.Errorf("explanation length = %d runes, want <= 100", got)
	}
	if !strings.HasSuffix(response.Message, "…") || !strings.HasSuffix(response.Explanation, "…") {
		t.Errorf("truncated fields should end with an ellipsis: %q / %q", response.Message, response.Explanation)
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
		`{"result":"pass","confidence":"high","message":"guide documented in README","explanation":"the README links a detailed user guide"}`)}

	step := func(payload any) (gemara.Result, string, gemara.ConfidenceLevel) {
		target := payload.(*assistTarget)
		response, ev, err := Assist(context.Background(), client, Question{Prompt: "docs?", Material: "README"})
		if err != nil {
			return gemara.Unknown, err.Error(), gemara.Undetermined
		}
		target.AddEvidence(ev)
		return response.GemaraResult(), response.Summary(), response.GemaraConfidence()
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
