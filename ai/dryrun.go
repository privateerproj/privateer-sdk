package ai

import (
	"context"
	"fmt"
	"io"
	"log"
)

// dryRunClient implements Client without contacting any provider. It logs
// what would be sent and returns a predictable response so callers can
// inspect prompt content and model settings without spending tokens.
type dryRunClient struct {
	config Config
	// validator runs the target provider's request validation so dry-run
	// rejects the same malformed requests a live run would, including
	// provider-specific rules. Nil when the provider exposes no validator.
	validator requestValidator
	// logOutput lets tests capture log output. When nil, log.Default()
	// is used so dry-run details appear on the standard logger.
	logOutput io.Writer
}

func newDryRunClient(config Config) *dryRunClient {
	// Build the real adapter (a cheap struct construction, no network) purely
	// to borrow its request validation, so dry-run mirrors the provider the
	// caller would hit live. NewClient already rejects unregistered providers
	// before reaching here.
	var validator requestValidator
	if factory, ok := clientFactories[config.Provider]; ok {
		if v, ok := factory(config).(requestValidator); ok {
			validator = v
		}
	}
	return &dryRunClient{config: config, validator: validator}
}

// Analyze records prompt details and returns a dry-run result without
// making any network call. Callers can distinguish dry-run responses via
// Metadata.FinishReason == FinishReasonDryRun.
func (c *dryRunClient) Analyze(ctx context.Context, prompt, content string, schema *Schema) (*AnalyzeResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Run the target provider's validation (falling back to the package-level
	// schema check) so a malformed request fails here instead of slipping
	// through dry-run only to break when the user switches to a live run.
	if c.validator != nil {
		if err := c.validator.ValidateRequest(prompt, content, schema); err != nil {
			return nil, err
		}
	} else if err := validateStructuredSchema(c.config.Provider, schema); err != nil {
		return nil, err
	}

	schemaName := "<none>"
	if schema != nil {
		schemaName = schema.Name
	}

	summary := fmt.Sprintf(
		"[ai dry-run] provider=%s model=%s max_tokens=%d schema=%s prompt_bytes=%d content_bytes=%d",
		c.config.Provider, c.config.Model, c.config.MaxTokens, schemaName, len(prompt), len(content),
	)
	c.logf("%s", summary)
	c.logf("[ai dry-run] prompt: %s", prompt)
	c.logf("[ai dry-run] content: %s", content)

	return &AnalyzeResponse{
		Text: summary,
		Metadata: ResponseMetadata{
			Provider:     c.config.Provider,
			Model:        c.config.Model,
			FinishReason: FinishReasonDryRun,
		},
	}, nil
}

func (c *dryRunClient) logf(format string, args ...any) {
	if c.logOutput != nil {
		_, _ = fmt.Fprintf(c.logOutput, format+"\n", args...)
		return
	}
	log.Printf(format, args...)
}
