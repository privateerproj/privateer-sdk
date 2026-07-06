package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// analyzeRequest is the internal, provider-neutral form of an Analyze call
type analyzeRequest struct {
	Prompt  string
	Content string
	Schema  *Schema
}

// providerClient is the shared base embedded by each adapter. It owns the HTTP
// client, resolved base URL, and provider identity used in normalized errors.
type providerClient struct {
	provider   Provider
	config     Config
	httpClient *http.Client
	baseURL    string
}

// requestOptions carries per-call HTTP extras layered on top of the JSON body
type requestOptions struct {
	Headers map[string]string
	Query   url.Values
}

// newProviderClient builds the shared base for an adapter, resolving the
// effective base URL and HTTP client from config.
func newProviderClient(provider Provider, config Config, defaultBaseURL string) providerClient {
	return providerClient{
		provider:   provider,
		config:     config,
		httpClient: config.httpClient(),
		baseURL:    normalizeBaseURL(config.BaseURL, defaultBaseURL),
	}
}

// newJSONRequest builds an *http.Request with a JSON body plus the given
// headers and query params. Marshal or construction failures become
// ErrorKindInvalidRequest.
func (c providerClient) newJSONRequest(ctx context.Context, method, requestPath string, payload any, options requestOptions) (*http.Request, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, c.invalidRequest("marshal", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+requestPath, bytes.NewReader(body))
	if err != nil {
		return nil, c.invalidRequest("build", err)
	}

	if len(options.Query) > 0 {
		query := req.URL.Query()
		for key, values := range options.Query {
			for _, value := range values {
				query.Add(key, value)
			}
		}
		req.URL.RawQuery = query.Encode()
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range options.Headers {
		req.Header.Set(key, value)
	}

	return req, nil
}

// invalidRequest wraps a request-construction failure (e.g. action "marshal" or
// "build") as ErrorKindInvalidRequest, preserving err for errors.Is/As.
func (c providerClient) invalidRequest(action string, err error) error {
	return &Error{
		Kind:     ErrorKindInvalidRequest,
		Provider: c.provider,
		Err:      err,
		Message:  fmt.Sprintf("%s %s request: %v", action, c.provider, err),
	}
}

// do executes a prepared request and returns the body bytes with the response.
// Transport failures are mapped through classifyTransportError so callers can
// branch on ErrorKind (e.g. ErrorKindTimeout).
func (c providerClient) do(req *http.Request) ([]byte, *http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, classifyTransportError(c.provider, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp, &Error{
			Kind:     ErrorKindInvalidResponse,
			Provider: c.provider,
			Err:      err,
			Message:  fmt.Sprintf("read %s response: %v", c.provider, err),
		}
	}

	return body, resp, nil
}

// normalizeBaseURL prefers the caller-supplied base URL, else the adapter
// default. Trailing slashes are trimmed so a leading-slash request path
// concatenates safely.
func normalizeBaseURL(baseURL, defaultBaseURL string) string {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalized != "" {
		return normalized
	}
	return defaultBaseURL
}

// validateStructuredSchema rejects unusable Schema values as
// ErrorKindUnsupportedConfig. A nil schema means no structured output and is accepted.
func validateStructuredSchema(provider Provider, schema *Schema) error {
	if schema == nil {
		return nil
	}
	if len(schema.Value) == 0 {
		return &Error{
			Kind:     ErrorKindUnsupportedConfig,
			Provider: provider,
			Message:  "structured output schema is required when schema is provided",
		}
	}
	return nil
}

// parseStructuredOutput returns content as json.RawMessage, rejecting invalid
// JSON as ErrorKindInvalidResponse.
func parseStructuredOutput(provider Provider, content string) (json.RawMessage, error) {
	raw := json.RawMessage(strings.TrimSpace(content))
	if !json.Valid(raw) {
		return nil, &Error{
			Kind:     ErrorKindInvalidResponse,
			Provider: provider,
			Message:  fmt.Sprintf("%s structured response was not valid JSON", provider),
		}
	}
	return raw, nil
}

// firstNonEmpty returns the first non-blank string from values, letting adapters
// prefer one metadata source over a fallback without nested conditionals.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
