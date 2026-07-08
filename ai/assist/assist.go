// Package assist is the plugin-facing accelerator for AI-assisted assessment
// steps: it asks a provider-neutral client for a structured verdict against an
// SDK-owned schema and packages the answer as auditable gemara.Evidence. The
// caller decides whether to record the evidence and how the verdict folds into
// the step result. See docs/ai-assist.md.
package assist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gemaraproj/go-gemara"
	"github.com/privateerproj/privateer-sdk/ai/provider"
)

// EvidenceType is stamped on every gemara.Evidence produced by Assist
const EvidenceType gemara.EvidenceType = "ai-assessment"

const (
	// maxMessageChars is the SDK-owned budget for Response.Message. It is not
	// configurable: the short message must stay shaped like every other
	// assessment message regardless of which plugin asked the question.
	maxMessageChars = 160
	// defaultMaxExplanationChars bounds Response.Explanation when the Question
	// does not set its own budget.
	defaultMaxExplanationChars = 1500
)

// Question is the input to an AI-assisted assessment step
// This will go into the final output as part of the evidence
type Question struct {
	// Prompt tells the model what to decide and how to decide it
	Prompt string
	// Material is the content the model inspects
	Material string
	// MaxExplanationChars bounds the long-form Explanation in the response.
	// Zero uses defaultMaxExplanationChars. The short Message budget is
	// SDK-owned and cannot be changed.
	MaxExplanationChars int
}

// Response is a structured answer from the assistant
type Response struct {
	// Result is the model's disposition: "pass", "fail", or "needs_review".
	Result string `json:"result" yaml:"result"`
	// Confidence is how sure the model is: "low", "medium", or "high".
	Confidence string `json:"confidence" yaml:"confidence"`
	// Message is a model-authored single sentence stating what was found or
	// missing, guaranteed single-line and at most maxMessageChars long.
	Message string `json:"message" yaml:"message"`
	// Explanation is the model's verbose justification for the verdict,
	// bounded by Question.MaxExplanationChars.
	Explanation string `json:"explanation" yaml:"explanation"`
	// Citations optionally points at where in Content the model found support
	Citations []string `json:"citations,omitempty" yaml:"citations,omitempty"`
}

// responseSchema pins the model to the Response shape. The char budgets are
// stated in field descriptions rather than maxLength keywords because provider
// strict modes support different JSON Schema subsets and may reject the
// request outright on an unsupported keyword; normalize enforces the budgets
// after parsing so they are guarantees either way.
func responseSchema(maxExplanationChars int) *provider.Schema {
	return &provider.Schema{
		Name:        "assessment_verdict",
		Description: "Structured verdict for an AI-assisted control assessment.",
		Strict:      true,
		Value: json.RawMessage(fmt.Sprintf(`{
			"type": "object",
			"properties": {
				"result":      {"type": "string", "enum": ["pass", "fail", "needs_review"]},
				"confidence":  {"type": "string", "enum": ["low", "medium", "high"]},
				"message":     {"type": "string", "description": "One short sentence of at most %d characters stating what was found or what was missing. Do not restate the verdict or confidence."},
				"explanation": {"type": "string", "description": "Verbose justification of at most %d characters explaining how the material supports the verdict."},
				"citations":   {"type": ["array", "null"], "items": {"type": "string"}}
			},
			"required": ["result", "confidence", "message", "explanation", "citations"],
			"additionalProperties": false
		}`, maxMessageChars, maxExplanationChars)),
	}
}

// normalize enforces the response shape the schema descriptions request:
// Message becomes a single line hard-capped at maxMessageChars, Explanation is
// capped at maxExplanationChars. Models usually respect the budgets; this makes
// them guarantees.
func (v *Response) normalize(maxExplanationChars int) {
	v.Message = truncate(strings.Join(strings.Fields(v.Message), " "), maxMessageChars)
	v.Explanation = truncate(strings.TrimSpace(v.Explanation), maxExplanationChars)
}

// truncate caps s at max runes, marking any cut with an ellipsis.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// EvidencePayload is the structured body stored in gemara.Evidence.Payload
type EvidencePayload struct {
	Response  Response          `json:"verdict" yaml:"verdict"`
	Prompt    string            `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	Material  string            `json:"material,omitempty" yaml:"material,omitempty"`
	Provider  provider.Provider `json:"provider,omitempty" yaml:"provider,omitempty"`
	Model     string            `json:"model,omitempty" yaml:"model,omitempty"`
	RequestID string            `json:"request-id,omitempty" yaml:"request-id,omitempty"`
}

// Assist runs an AI-assisted assessment: it asks client for a structured Response
// answering q, then packages that verdict as a gemara.Evidence
func Assist(ctx context.Context, client provider.Client, q Question) (Response, gemara.Evidence, error) {
	if client == nil {
		return Response{}, gemara.Evidence{}, fmt.Errorf("AI assessment skipped: no client configured")
	}

	maxExplanationChars := q.MaxExplanationChars
	if maxExplanationChars <= 0 {
		maxExplanationChars = defaultMaxExplanationChars
	}

	aResp, err := client.Analyze(ctx, q.Prompt, q.Material, responseSchema(maxExplanationChars))
	if err != nil {
		return Response{}, gemara.Evidence{}, fmt.Errorf("AI assessment failed: %w", err)
	}
	if aResp == nil {
		return Response{}, gemara.Evidence{}, fmt.Errorf("AI assessment failed: provider returned no response")
	}

	var response Response
	if err := json.Unmarshal(aResp.JSON, &response); err != nil {
		return Response{}, gemara.Evidence{}, fmt.Errorf("AI response was not valid structured JSON: %w", err)
	}
	response.normalize(maxExplanationChars)

	return response, newEvidence(response, aResp, q), nil
}

// newEvidence assembles and timestamps the gemara.Evidence for a completed call
func newEvidence(v Response, resp *provider.AnalyzeResponse, q Question) gemara.Evidence {
	payload := EvidencePayload{Response: v, Prompt: q.Prompt, Material: q.Material}
	if resp != nil {
		payload.Provider = resp.Metadata.Provider
		payload.Model = resp.Metadata.Model
		payload.RequestID = resp.Metadata.RequestID
	}

	timeNow := time.Now().Format(time.RFC3339)

	return gemara.Evidence{
		Id:          evidenceID(resp, timeNow),
		Type:        EvidenceType,
		CollectedAt: gemara.Datetime(timeNow),
		Description: "AI Assisted Review",
		Payload:     payload,
	}
}

// evidenceID prefers the provider's request id and falls back to a timestamped id when none is reported
func evidenceID(resp *provider.AnalyzeResponse, timeNow string) string {
	if resp != nil {
		if id := strings.TrimSpace(resp.Metadata.RequestID); id != "" {
			return id
		}
	}
	return fmt.Sprintf("ai-%s", timeNow)
}

// Summary renders the canonical single-line assessment message for an
// AI-assisted step, e.g. "[AI-Assisted] No evidence found for when tests are
// run." It prefers the model-authored Message and falls back to the bare
// verdict when the message is empty. The explanation, citations, and exact
// prompt are already recorded in the Evidence; the message must not restate them.
func (v Response) Summary() string {
	if message := strings.TrimSpace(v.Message); message != "" {
		return "[AI-Assisted] " + message
	}

	result := strings.ToLower(strings.TrimSpace(v.Result))
	if result == "" {
		result = "needs_review"
	}
	confidence := strings.ToLower(strings.TrimSpace(v.Confidence))
	if confidence == "" {
		return fmt.Sprintf("[AI-Assisted] verdict: %s", result)
	}
	return fmt.Sprintf("[AI-Assisted] verdict: %s (%s confidence)", result, confidence)
}

// GemaraResult maps the assistant's verdict onto a gemara.Result. Anything other
// than an explicit pass/fail maps to NeedsReview.
func (v Response) GemaraResult() gemara.Result {
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
func (v Response) GemaraConfidence() gemara.ConfidenceLevel {
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
