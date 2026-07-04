package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gemaraproj/go-gemara"
)

// EvidenceType is stamped on every gemara.Evidence produced by Assist. It marks
// the record as software-assisted (an AI verdict) rather than a directly
// observed fact, so a reviewer can weigh it accordingly.
const EvidenceType gemara.EvidenceType = "ai-assessment"

// Question is the input to an AI-assisted assessment step: an instruction to the
// model (Prompt) and the material it should inspect (Content).
type Question struct {
	// Prompt tells the model what to decide and how to decide it.
	Prompt string
	// Content is the material the model inspects — file contents, config, a
	// rendered API response, etc. It is sent to the provider verbatim and
	// recorded verbatim in the evidence payload; the caller decides what (and
	// how much) to include, and must never include secrets.
	Content string
	// Description overrides the human-readable summary on the returned
	// gemara.Evidence. When empty, the model's Reasoning is used.
	Description string
}

// Verdict is the SDK's canonical structured answer. Assist asks the model for
// exactly this shape, so plugin authors never hand-write a JSON Schema, and it
// becomes the gemara.Evidence payload so the record is self-describing.
type Verdict struct {
	// Result is the model's disposition: "pass", "fail", or "needs_review".
	Result string `json:"result" yaml:"result"`
	// Confidence is how sure the model is: "low", "medium", or "high".
	Confidence string `json:"confidence" yaml:"confidence"`
	// Reasoning is the model's short justification for the verdict.
	Reasoning string `json:"reasoning" yaml:"reasoning"`
	// Citations optionally points at where in Content the model found support
	// (e.g. file paths, URLs, line references).
	Citations []string `json:"citations,omitempty" yaml:"citations,omitempty"`
}

// verdictSchema is the SDK-owned JSON Schema that pins the model to the Verdict
// shape. Owning it here is the whole point of the accelerator: callers get a
// structured, mappable answer without describing the schema themselves.
var verdictSchema = &Schema{
	Name:        "assessment_verdict",
	Description: "Structured verdict for an AI-assisted control assessment.",
	Strict:      true,
	Value: json.RawMessage(`{
		"type": "object",
		"properties": {
			"result":     {"type": "string", "enum": ["pass", "fail", "needs_review"]},
			"confidence": {"type": "string", "enum": ["low", "medium", "high"]},
			"reasoning":  {"type": "string"},
			"citations":  {"type": ["array", "null"], "items": {"type": "string"}}
		},
		"required": ["result", "confidence", "reasoning", "citations"],
		"additionalProperties": false
	}`),
}

// EvidencePayload is the structured body stored in gemara.Evidence.Payload. It
// pairs the model's Verdict with the exact question asked (Prompt and Content)
// and the provenance a reviewer needs to audit or reproduce the verdict without
// relying on provider-side request logs. Content lands verbatim in evaluation
// output — callers must not put secrets in Question.Content.
type EvidencePayload struct {
	Verdict   Verdict  `json:"verdict" yaml:"verdict"`
	Prompt    string   `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	Content   string   `json:"content,omitempty" yaml:"content,omitempty"`
	Provider  Provider `json:"provider,omitempty" yaml:"provider,omitempty"`
	Model     string   `json:"model,omitempty" yaml:"model,omitempty"`
	RequestID string   `json:"request-id,omitempty" yaml:"request-id,omitempty"`
}

// Assist runs an AI-assisted assessment: it asks client for a structured Verdict
// answering q, then packages that verdict as a gemara.Evidence the caller can
// hand to its payload's AddEvidence at its own discretion. Assist deliberately
// does NOT call AddEvidence and does NOT decide the step's Result — it returns
// the Verdict (map it with GemaraResult/GemaraConfidence) and the Evidence, so
// the caller stays in control of how the AI outcome folds into its assessment.
//
// Assist never panics. A nil client, provider error, nil response, or
// unparseable response is returned as an error alongside a needs_review
// Verdict and an Evidence record
// noting the attempt, so a step can still show that an AI check was tried.
//
// Under --dry-run-ai the client returns no structured body; Assist recognizes
// this and returns a clean needs_review Verdict with a nil error, so pipelines
// are exercisable without spending tokens or tripping the parse-failure path.
func Assist(ctx context.Context, client Client, q Question) (Verdict, gemara.Evidence, error) {
	if client == nil {
		v := failVerdict("AI assessment skipped: no client configured")
		return v, newEvidence(v, nil, q), fmt.Errorf("ai: nil client")
	}

	resp, err := client.Analyze(ctx, q.Prompt, q.Content, verdictSchema)
	if err != nil {
		v := failVerdict("AI assessment failed: " + err.Error())
		return v, newEvidence(v, resp, q), fmt.Errorf("ai: analyze: %w", err)
	}
	if resp == nil {
		v := failVerdict("AI assessment failed: provider returned no response")
		return v, newEvidence(v, nil, q), fmt.Errorf("ai: analyze returned no response")
	}

	// Dry-run returns only Text (no structured body). Treat it as a clean,
	// no-spend path rather than a parse failure.
	if resp.Metadata.FinishReason == FinishReasonDryRun {
		v := failVerdict("AI dry-run: no live assessment performed")
		return v, newEvidence(v, resp, q), nil
	}

	var v Verdict
	if err := json.Unmarshal(resp.JSON, &v); err != nil {
		bad := failVerdict("AI response was not valid structured JSON")
		return bad, newEvidence(bad, resp, q), fmt.Errorf("ai: parse verdict: %w", err)
	}

	return v, newEvidence(v, resp, q), nil
}

// failVerdict builds the conservative fallback verdict used whenever a real
// verdict cannot be obtained. It is needs_review (never pass) so an ambiguous or
// failed AI check can never silently satisfy a control.
func failVerdict(reason string) Verdict {
	return Verdict{Result: "needs_review", Confidence: "low", Reasoning: reason}
}

// newEvidence assembles the gemara.Evidence for a completed (or failed) Assist
// call. CollectedAt is stamped here so the record is timestamped at the moment
// the verdict was formed.
func newEvidence(v Verdict, resp *AnalyzeResponse, q Question) gemara.Evidence {
	description := strings.TrimSpace(q.Description)
	if description == "" {
		description = v.Reasoning
	}

	payload := EvidencePayload{Verdict: v, Prompt: q.Prompt, Content: q.Content}
	if resp != nil {
		payload.Provider = resp.Metadata.Provider
		payload.Model = resp.Metadata.Model
		payload.RequestID = resp.Metadata.RequestID
	}

	return gemara.Evidence{
		Id:          evidenceID(resp),
		Type:        EvidenceType,
		CollectedAt: gemara.Datetime(time.Now().Format(time.RFC3339)),
		Description: description,
		Payload:     payload,
	}
}

// evidenceID prefers the provider's request id (stable, correlatable with
// provider-side logs) and falls back to a timestamped id when none is reported.
func evidenceID(resp *AnalyzeResponse) string {
	if resp != nil {
		if id := strings.TrimSpace(resp.Metadata.RequestID); id != "" {
			return id
		}
	}
	return fmt.Sprintf("ai-%d", time.Now().UnixNano())
}

// GemaraResult maps the model's verdict onto a gemara.Result. Anything other
// than an explicit pass/fail maps to NeedsReview, so an unrecognized or empty
// value never silently passes.
func (v Verdict) GemaraResult() gemara.Result {
	switch strings.ToLower(strings.TrimSpace(v.Result)) {
	case "pass":
		return gemara.Passed
	case "fail":
		return gemara.Failed
	default:
		return gemara.NeedsReview
	}
}

// GemaraConfidence maps the model's confidence onto a gemara.ConfidenceLevel.
// An unrecognized or empty value maps to Undetermined.
func (v Verdict) GemaraConfidence() gemara.ConfidenceLevel {
	switch strings.ToLower(strings.TrimSpace(v.Confidence)) {
	case "high":
		return gemara.High
	case "medium":
		return gemara.Medium
	case "low":
		return gemara.Low
	default:
		return gemara.Undetermined
	}
}
