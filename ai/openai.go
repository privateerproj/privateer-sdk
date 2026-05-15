package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const defaultOpenAIBaseURL = "https://api.openai.com/v1"

// openAIClient is the Client implementation for OpenAI. It embeds the
// shared providerClient for HTTP, base URL, and error handling.
type openAIClient struct {
	providerClient
}

// openAIChatCompletionsRequest mirrors the JSON body POSTed to
// /v1/chat/completions. Only the fields this adapter actually populates
// are modeled; the rest of OpenAI's API is intentionally left out.
type openAIChatCompletionsRequest struct {
	Model          string                `json:"model"`
	Messages       []openAIMessage       `json:"messages"`
	MaxTokens      int                   `json:"max_tokens,omitempty"`
	ResponseFormat *openAIResponseFormat `json:"response_format,omitempty"`
}

// openAIMessage is one entry in the Chat Completions "messages" array.
// The adapter sends a system message (prompt) and a user message (content).
type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIResponseFormat asks OpenAI to constrain the response shape. When
// Schema is supplied on Analyze, the adapter sets Type="json_schema" and
// fills JSONSchema with the caller's schema document.
type openAIResponseFormat struct {
	Type       string                `json:"type"`
	JSONSchema *openAIJSONSchemaWrap `json:"json_schema,omitempty"`
}

// openAIJSONSchemaWrap is OpenAI's required wrapper around a raw JSON
// Schema document. It carries a name plus optional description/strict
// flags that providers use when validating model output.
type openAIJSONSchemaWrap struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Strict      bool            `json:"strict,omitempty"`
	Schema      json.RawMessage `json:"schema"`
}

// openAIChatCompletionsResponse models the slice of the Chat Completions
// response the adapter actually reads: the request id, the model that
// actually served the request, the first choice's content and finish
// reason, and an optional error envelope present on failure responses.
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

// newOpenAIClient constructs the OpenAI adapter. Wired into the package
// registry via clientFactories in client.go.
func newOpenAIClient(config Config) *openAIClient {
	return &openAIClient{
		providerClient: newProviderClient(ProviderOpenAI, config, defaultOpenAIBaseURL),
	}
}

// Analyze implements the Client contract against OpenAI Chat Completions.
// At a high level it: validates the optional Schema, builds the request
// body (translating Schema into OpenAI's response_format when present),
// POSTs to /chat/completions with a Bearer token, returns non-200
// responses as classified *Error values, and finally maps the first
// choice back into a provider-neutral AnalyzeResponse (parsing the
// content as JSON when a schema was requested).
func (c *openAIClient) Analyze(ctx context.Context, prompt, content string, schema *Schema) (*AnalyzeResponse, error) {
	request := analyzeRequest{
		Prompt:  prompt,
		Content: content,
		Schema:  schema,
	}
	if err := validateStructuredSchema(ProviderOpenAI, request.Schema); err != nil {
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
