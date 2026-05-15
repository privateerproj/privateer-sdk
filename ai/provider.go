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

// analyzeRequest is the internal, provider-neutral form of an Analyze call.
// Adapters translate it into whatever the underlying API expects.
type analyzeRequest struct {
	Prompt  string
	Content string
	Schema  *Schema
}

// providerClient is the shared base type embedded by each adapter. It owns
// the HTTP client, the resolved base URL, and the provider identity used
// when constructing normalized errors.
type providerClient struct {
	provider   Provider
	config     Config
	httpClient *http.Client
	baseURL    string
}

// requestOptions carries per-call HTTP extras (auth headers, query params)
// that adapters layer on top of the JSON body built by newJSONRequest.
type requestOptions struct {
	Headers map[string]string
	Query   url.Values
}

// newProviderClient constructs the shared base used by an adapter. It
// resolves the effective base URL (caller override or the adapter's
// default) and the HTTP client (caller-supplied or Timeout-honoring).
func newProviderClient(provider Provider, config Config, defaultBaseURL string) providerClient {
	return providerClient{
		provider:   provider,
		config:     config,
		httpClient: config.httpClient(),
		baseURL:    normalizeBaseURL(config.BaseURL, defaultBaseURL),
	}
}

// newJSONRequest builds an *http.Request with a JSON body and the supplied
// headers/query params. Marshal or request-construction failures are
// returned as *Error with ErrorKindInvalidRequest so callers see a uniform
// failure shape regardless of which provider produced them.
func (c providerClient) newJSONRequest(ctx context.Context, method, requestPath string, payload any, options requestOptions) (*http.Request, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, &Error{
			Kind:     ErrorKindInvalidRequest,
			Provider: c.provider,
			Err:      err,
			Message:  fmt.Sprintf("marshal %s request: %v", c.provider, err),
		}
	}

	requestURL := c.baseURL + requestPath
	req, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(body))
	if err != nil {
		return nil, &Error{
			Kind:     ErrorKindInvalidRequest,
			Provider: c.provider,
			Err:      err,
			Message:  fmt.Sprintf("build %s request: %v", c.provider, err),
		}
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

// do executes a prepared request and returns the body bytes alongside the
// response. Transport-level failures (DNS, TCP, TLS, timeouts) are mapped
// through classifyTransportError so callers can branch on ErrorKind
// (e.g. ErrorKindTimeout) without inspecting raw network errors.
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

// normalizeBaseURL picks the caller-supplied base URL when set, otherwise
// falls back to the adapter's default. Trailing slashes are trimmed so
// adapters can safely concatenate a leading-slash request path.
func normalizeBaseURL(baseURL, defaultBaseURL string) string {
	normalized := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalized != "" {
		return normalized
	}
	return defaultBaseURL
}

// validateStructuredSchema rejects obviously unusable Schema values before
// they reach the provider, surfacing them as ErrorKindUnsupportedConfig.
// A nil schema means "no structured output requested" and is accepted.
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
	if strings.TrimSpace(schema.Name) == "" {
		return &Error{
			Kind:     ErrorKindUnsupportedConfig,
			Provider: provider,
			Message:  "structured output schema name is required",
		}
	}
	return nil
}

// parseStructuredOutput validates that a provider's structured-output
// content is actually valid JSON and returns it as json.RawMessage for
// callers to unmarshal into their own types. Invalid JSON becomes an
// ErrorKindInvalidResponse so callers can distinguish provider hiccups
// from caller-side parsing bugs.
func parseStructuredOutput(provider Provider, content string) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(content)
	raw := json.RawMessage(trimmed)
	if !json.Valid(raw) {
		return nil, &Error{
			Kind:     ErrorKindInvalidResponse,
			Provider: provider,
			Message:  fmt.Sprintf("%s structured response was not valid JSON", provider),
		}
	}
	return raw, nil
}

// firstNonEmpty returns the first non-blank string from values. Adapters
// use it to prefer one source of a metadata field (e.g. an HTTP header)
// over a fallback (e.g. a body field) without nested conditionals.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
