package ai

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrorKind categorizes provider failures for consistent caller handling.
// Adapters map their native errors into one of these kinds so callers
// never depend on the exact wording or status code a provider returned.
type ErrorKind string

const (
	// ErrorKindUnauthorized indicates the credential was missing, invalid,
	// or lacks permission for the requested model.
	ErrorKindUnauthorized ErrorKind = "unauthorized"
	// ErrorKindRateLimited indicates the provider asked us to back off.
	// Callers may retry with backoff.
	ErrorKindRateLimited ErrorKind = "rate_limited"
	// ErrorKindTimeout indicates the request did not complete within the
	// configured Timeout or was cancelled via context.
	ErrorKindTimeout ErrorKind = "timeout"
	// ErrorKindProviderError is the catch-all for upstream failures that
	// do not fall into a more specific kind (5xx, unknown 4xx, etc.).
	ErrorKindProviderError ErrorKind = "provider_error"
	// ErrorKindInvalidRequest indicates the request body or parameters
	// were rejected by the provider (e.g. 400, 422).
	ErrorKindInvalidRequest ErrorKind = "invalid_request"
	// ErrorKindInvalidResponse indicates the provider returned data we
	// could not parse (malformed JSON, missing choices, etc.).
	ErrorKindInvalidResponse ErrorKind = "invalid_response"
	// ErrorKindUnsupportedConfig indicates a caller-side configuration
	// problem (e.g. schema supplied without a Name).
	ErrorKindUnsupportedConfig ErrorKind = "unsupported_config"
)

// Error normalizes provider-specific failures into a uniform type.
// Callers typically inspect Kind and, when relevant, StatusCode. The
// original error is preserved via Err so errors.Is / errors.As still work
// against the underlying transport-layer error.
type Error struct {
	Kind       ErrorKind
	Provider   Provider
	StatusCode int
	Message    string
	Err        error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s error from %s", e.Kind, e.Provider)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// classifyHTTPError maps an upstream HTTP status code into an ErrorKind
// so callers can react to common failure modes (auth, throttling, bad
// input) without parsing provider-specific error messages.
func classifyHTTPError(provider Provider, statusCode int, message string) error {
	kind := ErrorKindProviderError
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		kind = ErrorKindUnauthorized
	case http.StatusTooManyRequests:
		kind = ErrorKindRateLimited
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		kind = ErrorKindInvalidRequest
	}

	return &Error{
		Kind:       kind,
		Provider:   provider,
		StatusCode: statusCode,
		Message:    message,
	}
}

// classifyTransportError maps Go HTTP client errors into the same set of
// ErrorKinds as HTTP responses. Timeouts (context deadlines and net.Error
// timeouts) become ErrorKindTimeout; everything else falls under
// ErrorKindProviderError. The original error is preserved via Err so
// errors.Is/As continues to work against the underlying error.
func classifyTransportError(provider Provider, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, contextDeadlineExceeded) {
		return &Error{Kind: ErrorKindTimeout, Provider: provider, Err: err, Message: err.Error()}
	}

	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &Error{Kind: ErrorKindTimeout, Provider: provider, Err: err, Message: err.Error()}
	}

	return &Error{Kind: ErrorKindProviderError, Provider: provider, Err: err, Message: err.Error()}
}

// contextDeadlineExceeded is a local marker error used by
// classifyTransportError to recognize context-deadline failures via
// errors.Is without importing context here directly.
var contextDeadlineExceeded = errors.New("context deadline exceeded")
