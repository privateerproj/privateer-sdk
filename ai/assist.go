package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gemaraproj/go-gemara"
)

// EvidenceType is stamped on every gemara.Evidence produced by Assist
const EvidenceType gemara.EvidenceType = "ai-assessment"

// Question is the input to an AI-assisted assessment step
// This will go into the final output as part of the evidence
type Question struct {
	// Prompt tells the model what to decide and how to decide it
	Prompt string
	// Material is the content the model inspects
	Material string
}

// Response is a structured answer from the assistant
type Response struct {
	// Result is the model's disposition: "pass", "fail", or "needs_review".
	Result string `json:"result" yaml:"result"`
	// Confidence is how sure the model is: "low", "medium", or "high".
	Confidence string `json:"confidence" yaml:"confidence"`
	// Reasoning is the model's short justification for the verdict.
	Reasoning string `json:"reasoning" yaml:"reasoning"`
	// Citations optionally points at where in Content the model found support
	Citations []string `json:"citations,omitempty" yaml:"citations,omitempty"`
}

// responseSchema pins the model to the Response shape
var responseSchema = &Schema{
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

// EvidencePayload is the structured body stored in gemara.Evidence.Payload
type EvidencePayload struct {
	Response  Response `json:"verdict" yaml:"verdict"`
	Prompt    string   `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	Material  string   `json:"material,omitempty" yaml:"material,omitempty"`
	Provider  Provider `json:"provider,omitempty" yaml:"provider,omitempty"`
	Model     string   `json:"model,omitempty" yaml:"model,omitempty"`
	RequestID string   `json:"request-id,omitempty" yaml:"request-id,omitempty"`
}

// Assist runs an AI-assisted assessment: it asks client for a structured Response
// answering q, then packages that verdict as a gemara.Evidence
func Assist(ctx context.Context, client Client, q Question) (Response, gemara.Evidence, error) {
	if client == nil {
		return Response{}, gemara.Evidence{}, fmt.Errorf("AI assessment skipped: no client configured")
	}

	aResp, err := client.Analyze(ctx, q.Prompt, q.Material, responseSchema)
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

	return response, newEvidence(response, aResp, q), nil
}

// newEvidence assembles and timestamps the gemara.Evidence for a completed call
func newEvidence(v Response, resp *AnalyzeResponse, q Question) gemara.Evidence {
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
func evidenceID(resp *AnalyzeResponse, timeNow string) string {
	if resp != nil {
		if id := strings.TrimSpace(resp.Metadata.RequestID); id != "" {
			return id
		}
	}
	return fmt.Sprintf("ai-%s", timeNow)
}

// GemaraResult maps the assistant's verdict onto a gemara.Result. Anything other
// than an explicit pass/fail maps to NeedsReview.
func (v Response) GemaraResult() gemara.Result {
	if strings.Contains(strings.ToLower(strings.TrimSpace(v.Result)), "pass") {
		return gemara.Passed
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(v.Result)), "fail") {
		return gemara.Failed
	}
	return gemara.NeedsReview
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
