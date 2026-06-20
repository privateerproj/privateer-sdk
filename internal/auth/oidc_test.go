package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPollForToken_DeadlineExpires verifies that PollForToken returns
// ErrExpiredDeviceCode when the device-code lifetime elapses while the server
// keeps answering authorization_pending — it must not loop forever.
//
// The test uses ExpiresIn:1 (1-second deadline) with Interval:1 (1-second
// poll) so the loop fires once, gets authorization_pending, then on the next
// iteration discovers the deadline has passed. Total wall clock: ~1–2s.
func TestPollForToken_DeadlineExpires(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Always respond authorization_pending; the test relies on the
		// deadline to break the loop rather than a terminal server error.
		_ = json.NewEncoder(w).Encode(tokenResponse{Error: "authorization_pending"})
	}))
	defer srv.Close()

	meta := &OIDCMetadata{
		Issuer:                      srv.URL,
		DeviceAuthorizationEndpoint: srv.URL + "/device",
		TokenEndpoint:               srv.URL + "/token",
	}
	da := &DeviceAuthorization{
		DeviceCode: "dc-123",
		UserCode:   "USER-1",
		// ExpiresIn:1 makes the deadline 1 second from the start of PollForToken.
		ExpiresIn: 1,
		// Interval:1 gives the minimum feasible poll cadence so the loop fires
		// once before the deadline check trips on the second iteration.
		Interval: 1,
	}

	_, err := PollForToken(context.Background(), meta, "client-id", da)
	if !errors.Is(err, ErrExpiredDeviceCode) {
		t.Errorf("expected ErrExpiredDeviceCode, got %v", err)
	}
}
