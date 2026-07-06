package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// openAIClient is the Client implementation for OpenAI, embedding the shared
// providerClient for HTTP, base URL, and error handling.
type openAIClient struct {
	providerClient
}

// openAIChatCompletionsRequest is the JSON body POSTed to /v1/chat/completions.
// Only the fields this adapter populates are modeled.
type openAIChatCompletionsRequest struct {
	Model          string                `json:"model"`
	Messages       []openAIMessage       `json:"messages"`
	MaxTokens      int                   `json:"max_tokens,omitempty"`
	ResponseFormat *openAIResponseFormat `json:"response_format,omitempty"`
}

// openAIMessage is one entry in the "messages" array. The adapter sends a
// system message (prompt) and a user message (content).
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIResponseFormat constrains the response shape. With a Schema, the
// adapter sets Type="json_schema" and fills JSONSchema.
type openAIResponseFormat struct {
	Type       string                `json:"type"`
	JSONSchema *openAIJSONSchemaWrap `json:"json_schema,omitempty"`
}

// openAIJSONSchemaWrap is OpenAI's required wrapper around a raw JSON Schema.
type openAIJSONSchemaWrap struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Strict      bool            `json:"strict,omitempty"`
	Schema      json.RawMessage `json:"schema"`
}

// openAIChatCompletionsResponse models the slice of the response the adapter
// reads: request id, served model, the first choice, and a failure envelope.
type openAIChatCompletionsResponse struct {
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

// newOpenAIClient constructs the OpenAI adapter, wired into clientFactories.
func newOpenAIClient(config Config) *openAIClient {
	return &openAIClient{
		providerClient: newProviderClient(ProviderOpenAI, config, defaultOpenAIBaseURL),
	}
}

// ValidateRequest rejects malformed requests before any network call, applying
// the package-level schema checks plus OpenAI's rule that a structured-output
// schema must carry a Name (its json_schema wrapper label).
func (c *openAIClient) ValidateRequest(prompt, content string, schema *Schema) error {
	if err := validateStructuredSchema(ProviderOpenAI, schema); err != nil {
		return err
	}
	if schema != nil && strings.TrimSpace(schema.Name) == "" {
		return &Error{
			Kind:     ErrorKindUnsupportedConfig,
			Provider: ProviderOpenAI,
			Message:  "structured output schema name is required",
		}
	}
	return nil
}

// Analyze implements the Client contract against OpenAI Chat Completions:
// validate, build the request body (translating Schema into response_format),
// POST with a Bearer token, classify non-200s, and map the first choice back
// into an AnalyzeResponse (parsing JSON when a schema was requested).
func (c *openAIClient) Analyze(ctx context.Context, prompt, content string, schema *Schema) (*AnalyzeResponse, error) {
	request := analyzeRequest{
		Prompt:  prompt,
		Content: content,
		Schema:  schema,
	}
	if err := c.ValidateRequest(request.Prompt, request.Content, request.Schema); err != nil {
		return nil, err
	}

	reqBody := openAIChatCompletionsRequest{
		Model: c.config.Model,
		Messages: []openAIMessage{
			{Role: "system", Content: request.Prompt},
			{Role: "user", Content: request.Content},
		},
		MaxTokens: c.config.MaxTokens,
	}

	if request.Schema != nil {
		reqBody.ResponseFormat = &openAIResponseFormat{
			Type: "json_schema",
			JSONSchema: &openAIJSONSchemaWrap{
				Name:        strings.TrimSpace(request.Schema.Name),
				Description: strings.TrimSpace(request.Schema.Description),
				Strict:      request.Schema.Strict,
				Schema:      request.Schema.Value,
			},
		}
	}

	httpReq, err := c.newJSONRequest(ctx, http.MethodPost, "/chat/completions", reqBody, requestOptions{
		Headers: map[string]string{
			"Authorization": "Bearer " + c.config.APIKey,
		},
	})

	if err != nil {
		return nil, err
	}

	respBody, httpResp, err := c.do(httpReq)
	if err != nil {
		return nil, err
	}

	if httpResp.StatusCode != http.StatusOK {
		message := strings.TrimSpace(string(respBody))
		var failure openAIChatCompletionsResponse
		if err := json.Unmarshal(respBody, &failure); err == nil && failure.Error != nil && strings.TrimSpace(failure.Error.Message) != "" {
			message = failure.Error.Message
		}
		if message == "" {
			message = fmt.Sprintf("openai returned status %d", httpResp.StatusCode)
		}
		return nil, classifyHTTPError(ProviderOpenAI, httpResp.StatusCode, message)
	}

	var parsed openAIChatCompletionsResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, &Error{Kind: ErrorKindInvalidResponse, Provider: ProviderOpenAI, Err: err, Message: fmt.Sprintf("decode openai response: %v", err)}
	}
	if len(parsed.Choices) == 0 {
		return nil, &Error{Kind: ErrorKindInvalidResponse, Provider: ProviderOpenAI, Message: "openai response did not include any choices"}
	}

	messageContent := parsed.Choices[0].Message.Content
	response := &AnalyzeResponse{
		Text: messageContent,
		Metadata: ResponseMetadata{
			Provider:     ProviderOpenAI,
			Model:        parsed.Model,
			RequestID:    firstNonEmpty(httpResp.Header.Get("x-request-id"), parsed.ID),
			FinishReason: parsed.Choices[0].FinishReason,
		},
	}

	if request.Schema == nil {
		return response, nil
	}

	response.JSON, err = parseStructuredOutput(ProviderOpenAI, messageContent)
	if err != nil {
		return nil, err
	}

	return response, nil
}
