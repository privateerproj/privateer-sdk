package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const httpTimeout = 10 * time.Second

// User-facing sentinel errors from the device flow.
var (
	ErrAccessDenied      = errors.New("authorization denied by the user")
	ErrExpiredDeviceCode = errors.New("device code expired before authorization completed; run `pvtr login` again")
)

// OIDCMetadata is the subset of the issuer's OpenID Connect discovery document
// pvtr's device-grant flow needs.
type OIDCMetadata struct {
	Issuer                      string `json:"issuer"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
}

// FetchOIDCMetadata loads <issuer>/.well-known/openid-configuration. issuerURL
// is the hub-advertised oidc_issuer (NOT the hub URL).
func FetchOIDCMetadata(ctx context.Context, issuerURL string) (*OIDCMetadata, error) {
	issuerURL = strings.TrimRight(strings.TrimSpace(issuerURL), "/")
	if issuerURL == "" {
		return nil, errors.New("OIDC issuer URL is required (the hub discovery doc did not advertise oidc_issuer)")
	}
	discoveryURL := issuerURL + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discoveryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building OIDC discovery request for %s: %w", discoveryURL, err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: httpTimeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", discoveryURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OIDC discovery at %s returned %d: %s", discoveryURL, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	m := &OIDCMetadata{}
	if err := json.Unmarshal(body, m); err != nil {
		return nil, fmt.Errorf("decoding OIDC discovery at %s: %w", discoveryURL, err)
	}
	if m.DeviceAuthorizationEndpoint == "" {
		return nil, fmt.Errorf("OIDC discovery at %s did not advertise device_authorization_endpoint; the auth server is not configured for the device grant", discoveryURL)
	}
	if m.TokenEndpoint == "" {
		return nil, fmt.Errorf("OIDC discovery at %s did not advertise token_endpoint", discoveryURL)
	}
	if m.Issuer == "" {
		m.Issuer = issuerURL
	}
	return m, nil
}

// DeviceAuthorization is RFC 8628 §3.2's device authorization response.
type DeviceAuthorization struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete,omitempty"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// StartDeviceFlow calls the device_authorization_endpoint. The caller displays
// user_code + verification_uri, then hands the result to PollForToken.
func StartDeviceFlow(ctx context.Context, meta *OIDCMetadata, clientID string) (*DeviceAuthorization, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	// No offline_access — an interactive CLI doesn't need it, and requiring it
	// breaks freshly-bootstrapped realms (matches grcli's reasoning).
	form.Set("scope", "openid profile email")

	resp, body, err := postForm(ctx, meta.DeviceAuthorizationEndpoint, form)
	if err != nil {
		return nil, fmt.Errorf("device authorization request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var er struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		_ = json.Unmarshal(body, &er)
		switch er.Error {
		case "invalid_client":
			return nil, fmt.Errorf("device authorization rejected: the auth server does not recognize client_id %q as a public device-grant client — ask whoever runs the hub's auth server to provision it (this is not a credential pvtr can supply)", clientID)
		case "unauthorized_client":
			return nil, fmt.Errorf("device authorization rejected: client_id %q is not allowed to use the device grant", clientID)
		}
		return nil, fmt.Errorf("device_authorization_endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	d := &DeviceAuthorization{}
	if err := json.Unmarshal(body, d); err != nil {
		return nil, fmt.Errorf("decoding device authorization response: %w (body: %s)", err, strings.TrimSpace(string(body)))
	}
	if d.DeviceCode == "" || d.UserCode == "" || d.VerificationURI == "" {
		return nil, fmt.Errorf("device authorization response missing required fields: %s", strings.TrimSpace(string(body)))
	}
	if d.Interval <= 0 {
		d.Interval = 5 // RFC 8628 §3.2 default
	}
	return d, nil
}

type tokenResponse struct {
	AccessToken      string `json:"access_token,omitempty"`
	RefreshToken     string `json:"refresh_token,omitempty"`
	TokenType        string `json:"token_type,omitempty"`
	ExpiresIn        int    `json:"expires_in,omitempty"`
	Error            string `json:"error,omitempty"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// PollForToken blocks polling the token endpoint until the device flow
// completes, returning the issued Credentials.
func PollForToken(ctx context.Context, meta *OIDCMetadata, clientID string, da *DeviceAuthorization) (*Credentials, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", da.DeviceCode)

	expiresIn := da.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 1800 // RFC 8628 servers SHOULD send expires_in; bound the poll regardless
	}
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)

	interval := time.Duration(da.Interval) * time.Second
	for {
		if time.Now().After(deadline) {
			return nil, ErrExpiredDeviceCode
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
		_, body, err := postForm(ctx, meta.TokenEndpoint, form)
		if err != nil {
			return nil, fmt.Errorf("polling token endpoint: %w", err)
		}
		tr := tokenResponse{}
		if err := json.Unmarshal(body, &tr); err != nil {
			return nil, fmt.Errorf("decoding token response: %w (body: %s)", err, strings.TrimSpace(string(body)))
		}
		switch tr.Error {
		case "":
			if tr.AccessToken == "" {
				return nil, fmt.Errorf("token endpoint returned no access_token and no error: %s", strings.TrimSpace(string(body)))
			}
			return credsFromTokenResponse(meta.Issuer, &tr), nil
		case "authorization_pending":
			continue
		case "slow_down":
			interval += 5 * time.Second
			continue
		case "access_denied":
			return nil, ErrAccessDenied
		case "expired_token":
			return nil, ErrExpiredDeviceCode
		default:
			if tr.ErrorDescription != "" {
				return nil, fmt.Errorf("token endpoint error %q: %s", tr.Error, tr.ErrorDescription)
			}
			return nil, fmt.Errorf("token endpoint error %q", tr.Error)
		}
	}
}

// refreshToken exchanges a refresh_token for a fresh access token.
func refreshToken(ctx context.Context, meta *OIDCMetadata, clientID, refresh string) (*Credentials, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refresh)

	_, body, err := postForm(ctx, meta.TokenEndpoint, form)
	if err != nil {
		return nil, fmt.Errorf("refresh request: %w", err)
	}
	tr := tokenResponse{}
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("decoding refresh response: %w (body: %s)", err, strings.TrimSpace(string(body)))
	}
	if tr.Error != "" {
		if tr.ErrorDescription != "" {
			return nil, fmt.Errorf("refresh failed (%s): %s", tr.Error, tr.ErrorDescription)
		}
		return nil, fmt.Errorf("refresh failed: %s", tr.Error)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("refresh response missing access_token: %s", strings.TrimSpace(string(body)))
	}
	return credsFromTokenResponse(meta.Issuer, &tr), nil
}

func postForm(ctx context.Context, endpoint string, form url.Values) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, nil, fmt.Errorf("building request for %s: %w", endpoint, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: httpTimeout}).Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	return resp, body, nil
}

func credsFromTokenResponse(issuer string, tr *tokenResponse) *Credentials {
	// Floor the lifetime safely above the renewal window (Credentials.Expired) so a
	// token with a missing/short expires_in is not born already-expired — which would
	// make the very next BearerToken call refresh a token we just obtained, and fail
	// outright when there is no refresh token. Derived from renewalWindow so the two
	// can't drift apart.
	lifetime := max(time.Duration(tr.ExpiresIn)*time.Second, renewalWindow+30*time.Second)
	return &Credentials{
		Issuer:       issuer,
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		TokenType:    tr.TokenType,
		ExpiresAt:    time.Now().Add(lifetime),
	}
}
