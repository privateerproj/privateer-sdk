// Package openai is the OpenAI Chat Completions adapter for the provider-neutral
// AI client contract. It is registered in the parent ai package's factory
// registry; callers construct it via ai.NewClient / ai.NewClientWithAIConfig.
package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/privateerproj/privateer-sdk/ai/provider"
)

const (
	// Provider is the registry name and error identity for this adapter.
	Provider provider.Provider = "openai"

	defaultBaseURL = "https://api.openai.com/v1"
)

// Client is the provider.Client implementation for OpenAI, embedding the shared
// provider.Base for HTTP, base URL, and error handling.
type Client struct {
	provider.Base
}

// chatCompletionsRequest is the JSON body POSTed to /v1/chat/completions.
// Only the fields this adapter populates are modeled.
type chatCompletionsRequest struct {
	Model          string          `json:"model"`
	Messages       []message       `json:"messages"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

// message is one entry in the "messages" array. The adapter sends a
// system message (prompt) and a user message (content).
type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// responseFormat constrains the response shape. With a Schema, the
// adapter sets Type="json_schema" and fills JSONSchema.
type responseFormat struct {
	Type       string          `json:"type"`
	JSONSchema *jsonSchemaWrap `json:"json_schema,omitempty"`
}

// jsonSchemaWrap is OpenAI's required wrapper around a raw JSON Schema.
type jsonSchemaWrap struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Strict      bool            `json:"strict,omitempty"`
	Schema      json.RawMessage `json:"schema"`
}

// chatCompletionsResponse models the slice of the response the adapter
// reads: request id, served model, the first choice, and a failure envelope.
type chatCompletionsResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// NewClient constructs the OpenAI adapter; NewBase normalizes the config.
func NewClient(config provider.Config) *Client {
	return &Client{
		Base: provider.NewBase(Provider, config, defaultBaseURL),
	}
}

// validateRequest rejects malformed requests before any network call, applying
// the shared schema checks plus OpenAI's rule that a structured-output schema
// must carry a Name (its json_schema wrapper label).
func (c *Client) validateRequest(schema *provider.Schema) error {
	if err := provider.ValidateStructuredSchema(Provider, schema); err != nil {
		return err
	}
	if schema != nil && strings.TrimSpace(schema.Name) == "" {
		return &provider.Error{
			Kind:     provider.ErrorKindUnsupportedConfig,
			Provider: Provider,
			Message:  "structured output schema name is required",
		}
	}
	return nil
}

// Analyze implements the provider.Client contract against OpenAI Chat
// Completions: validate, build the request body (translating Schema into
// response_format), POST with a Bearer token, classify non-200s, and map the
// first choice back into an AnalyzeResponse (parsing JSON when a schema was
// requested).
func (c *Client) Analyze(ctx context.Context, prompt, content string, schema *provider.Schema) (*provider.AnalyzeResponse, error) {
	if err := c.validateRequest(schema); err != nil {
		return nil, err
	}

	reqBody := chatCompletionsRequest{
		Model: c.Config.Model,
		Messages: []message{
			{Role: "system", Content: prompt},
			{Role: "user", Content: content},
		},
		MaxTokens: c.Config.MaxTokens,
	}

	if schema != nil {
		reqBody.ResponseFormat = &responseFormat{
			Type: "json_schema",
			JSONSchema: &jsonSchemaWrap{
				Name:        strings.TrimSpace(schema.Name),
				Description: strings.TrimSpace(schema.Description),
				Strict:      schema.Strict,
				Schema:      schema.Value,
			},
		}
	}

	httpReq, err := c.NewJSONRequest(ctx, http.MethodPost, "/chat/completions", reqBody, provider.RequestOptions{
		Headers: map[string]string{
			"Authorization": "Bearer " + c.Config.APIKey,
		},
	})

	if err != nil {
		return nil, err
	}

	respBody, httpResp, err := c.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if httpResp.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(respBody))
		var failure chatCompletionsResponse
		if err := json.Unmarshal(respBody, &failure); err == nil && failure.Error != nil && strings.TrimSpace(failure.Error.Message) != "" {
			message = failure.Error.Message
		}
		if message == "" {
			message = fmt.Sprintf("openai returned status %d", httpResp.StatusCode)
		}
		return nil, provider.ClassifyHTTPError(Provider, httpResp.StatusCode, message)
	}

	var parsed chatCompletionsResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, &provider.Error{Kind: provider.ErrorKindInvalidResponse, Provider: Provider, Err: err, Message: fmt.Sprintf("decode openai response: %v", err)}
	}
	if len(parsed.Choices) == 0 {
		return nil, &provider.Error{Kind: provider.ErrorKindInvalidResponse, Provider: Provider, Message: "openai response did not include any choices"}
	}

	messageContent := parsed.Choices[0].Message.Content
	response := &provider.AnalyzeResponse{
		Text: messageContent,
		Metadata: provider.ResponseMetadata{
			Provider:     Provider,
			Model:        parsed.Model,
			RequestID:    provider.FirstNonEmpty(httpResp.Header.Get("x-request-id"), parsed.ID),
			FinishReason: parsed.Choices[0].FinishReason,
		},
	}

	if schema == nil {
		return response, nil
	}

	response.JSON, err = provider.ParseStructuredOutput(Provider, messageContent)
	if err != nil {
		return nil, err
	}

	return response, nil
}
