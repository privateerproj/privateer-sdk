package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// ErrorKind categorizes provider failures so callers never depend on the exact
// wording or status code a provider returned. See docs/ai-client.md.
type ErrorKind string

const (
	// ErrorKindUnauthorized: credential missing, invalid, or unpermitted.
	ErrorKindUnauthorized ErrorKind = "unauthorized"
	// ErrorKindRateLimited: provider asked us to back off.
	ErrorKindRateLimited ErrorKind = "rate_limited"
	// ErrorKindTimeout: request exceeded Timeout or the context was cancelled.
	ErrorKindTimeout ErrorKind = "timeout"
	// ErrorKindProviderError: catch-all upstream failure (5xx, unknown 4xx).
	ErrorKindProviderError ErrorKind = "provider_error"
	// ErrorKindInvalidRequest: provider rejected the request (400, 422).
	ErrorKindInvalidRequest ErrorKind = "invalid_request"
	// ErrorKindInvalidResponse: provider returned data we could not parse.
	ErrorKindInvalidResponse ErrorKind = "invalid_response"
	// ErrorKindUnsupportedConfig: caller-side config problem (e.g. schema
	// without a Name).
	ErrorKindUnsupportedConfig ErrorKind = "unsupported_config"
)

// Error normalizes provider-specific failures into a uniform type. Callers
// inspect Kind and, when relevant, StatusCode; Err is preserved so
// errors.Is / errors.As reach the underlying error.
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

// classifyHTTPError maps an upstream HTTP status code into an ErrorKind so
// callers can react to common failure modes without parsing provider messages.
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

// classifyTransportError maps Go HTTP client errors into ErrorKinds: context
// deadlines and net.Error timeouts become ErrorKindTimeout, everything else
// ErrorKindProviderError. Err is preserved for errors.Is/As.
func classifyTransportError(provider Provider, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return &Error{Kind: ErrorKindTimeout, Provider: provider, Err: err, Message: err.Error()}
	}

	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &Error{Kind: ErrorKindTimeout, Provider: provider, Err: err, Message: err.Error()}
	}

	return &Error{Kind: ErrorKindProviderError, Provider: provider, Err: err, Message: err.Error()}
}
