// Package anthropic is the Anthropic Messages API adapter for the
// provider-neutral AI client contract. It is registered in the parent ai
// package's factory registry; callers construct it via ai.NewClient /
// ai.NewClientWithAIConfig.
package anthropic

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
	Provider provider.Provider = "anthropic"

	defaultBaseURL = "https://api.anthropic.com/v1"

	// apiVersion is the required anthropic-version header value.
	apiVersion = "2023-06-01"

	// oauthTokenPrefix marks an OAuth access token (e.g. from `ant auth
	// print-credentials --access-token`) rather than a console API key.
	// OAuth tokens authenticate via Authorization: Bearer plus a beta header
	// instead of x-api-key.
	oauthTokenPrefix = "sk-ant-oat"
	oauthBetaHeader  = "oauth-2025-04-20"
)

// Client is the provider.Client implementation for Anthropic, embedding the
// shared provider.Base for HTTP, base URL, and error handling.
type Client struct {
	provider.Base
}

// messagesRequest is the JSON body POSTed to /v1/messages. Only the fields
// this adapter populates are modeled.
type messagesRequest struct {
	Model        string        `json:"model"`
	MaxTokens    int           `json:"max_tokens"`
	System       string        `json:"system,omitempty"`
	Messages     []message     `json:"messages"`
	OutputConfig *outputConfig `json:"output_config,omitempty"`
}

// message is one entry in the "messages" array. The adapter sends the prompt
// as the system field and the content as a single user message.
type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// outputConfig constrains the response shape. With a Schema, the adapter sets
// Format to a json_schema output format so the model must return conforming JSON.
type outputConfig struct {
	Format *outputFormat `json:"format,omitempty"`
}

// outputFormat is Anthropic's structured-output format wrapper. Unlike
// OpenAI's json_schema wrapper it carries no name or description.
type outputFormat struct {
	Type   string          `json:"type"`
	Schema json.RawMessage `json:"schema"`
}

// messagesResponse models the slice of the response the adapter reads:
// request id, served model, stop reason, text content blocks, and the error
// envelope Anthropic returns on failures.
type messagesResponse struct {
	ID         string `json:"id"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// NewClient constructs the Anthropic adapter; NewBase normalizes the config.
func NewClient(config provider.Config) *Client {
	return &Client{
		Base: provider.NewBase(Provider, config, defaultBaseURL),
	}
}

// authHeaders returns the auth headers for the configured credential: console
// API keys go in x-api-key; OAuth access tokens use Authorization: Bearer with
// the oauth beta header.
func (c *Client) authHeaders() map[string]string {
	headers := map[string]string{
		"anthropic-version": apiVersion,
	}
	if strings.HasPrefix(c.Config.APIKey, oauthTokenPrefix) {
		headers["Authorization"] = "Bearer " + c.Config.APIKey
		headers["anthropic-beta"] = oauthBetaHeader
	} else {
		headers["x-api-key"] = c.Config.APIKey
	}
	return headers
}

// Analyze implements the provider.Client contract against the Anthropic
// Messages API: validate, build the request body (translating Schema into an
// output_config json_schema format), POST with the appropriate auth headers,
// classify non-200s, and map the text content back into an AnalyzeResponse
// (parsing JSON when a schema was requested).
func (c *Client) Analyze(ctx context.Context, prompt, content string, schema *provider.Schema) (*provider.AnalyzeResponse, error) {
	if err := provider.ValidateStructuredSchema(Provider, schema); err != nil {
		return nil, err
	}

	reqBody := messagesRequest{
		Model:     c.Config.Model,
		MaxTokens: c.Config.MaxTokens,
		System:    prompt,
		Messages: []message{
			{Role: "user", Content: content},
		},
	}

	if schema != nil {
		reqBody.OutputConfig = &outputConfig{
			Format: &outputFormat{
				Type:   "json_schema",
				Schema: schema.Value,
			},
		}
	}

	httpReq, err := c.NewJSONRequest(ctx, http.MethodPost, "/messages", reqBody, provider.RequestOptions{
		Headers: c.authHeaders(),
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
		var failure messagesResponse
		if err := json.Unmarshal(respBody, &failure); err == nil && failure.Error != nil && strings.TrimSpace(failure.Error.Message) != "" {
			message = failure.Error.Message
		}
		if message == "" {
			message = fmt.Sprintf("anthropic returned status %d", httpResp.StatusCode)
		}
		return nil, provider.ClassifyHTTPError(Provider, httpResp.StatusCode, message)
	}

	var parsed messagesResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, &provider.Error{Kind: provider.ErrorKindInvalidResponse, Provider: Provider, Err: err, Message: fmt.Sprintf("decode anthropic response: %v", err)}
	}

	// Safety classifiers can decline a request with HTTP 200 and
	// stop_reason "refusal"; there is no usable verdict in that case.
	if parsed.StopReason == "refusal" {
		return nil, &provider.Error{Kind: provider.ErrorKindProviderError, Provider: Provider, Message: "anthropic declined the request (stop_reason refusal)"}
	}

	// Concatenate text blocks, skipping thinking or other non-text block
	// types some models emit before the answer.
	var text strings.Builder
	for _, block := range parsed.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	messageContent := text.String()
	if strings.TrimSpace(messageContent) == "" {
		return nil, &provider.Error{Kind: provider.ErrorKindInvalidResponse, Provider: Provider, Message: "anthropic response did not include any text content"}
	}

	response := &provider.AnalyzeResponse{
		Text: messageContent,
		Metadata: provider.ResponseMetadata{
			Provider:     Provider,
			Model:        parsed.Model,
			RequestID:    provider.FirstNonEmpty(httpResp.Header.Get("request-id"), parsed.ID),
			FinishReason: parsed.StopReason,
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
