// Package oci holds the grc.store OCI mechanics shared by `pvtr publish`
// (push a signed plugin index) and `pvtr install` (pull + verify one).
//
// This file is the first, deliberately small member: hub discovery. A user
// configures exactly ONE endpoint — the hub base URL — and the OCI registry
// host is learned from the hub's /.well-known/ext.grc-store document
// (ADR-0026). Nothing in pvtr hardcodes the registry host, mirroring how the
// hub advertises it via HUB_OCI_PUBLIC_URL.
package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// DefaultHubURL is the production grc.store hub base, used when no hub URL is
// configured. Override it with the "hub-url" config key — set in config.yml, or
// via the PVTR_HUB_URL environment variable (e.g. http://localhost:8088 against
// the local dev stack, or https://hub.preview.grc.store against preview).
const DefaultHubURL = "https://hub.grc.store"

// hubURLKey is the viper/config.yml key for the hub base URL. It is a first-class
// config option like binaries-path: settable in config.yml and overridable by the
// PVTR_HUB_URL environment variable (viper's PVTR_ prefix + "-"→"_" replacer maps
// the hub-url key onto PVTR_HUB_URL).
const hubURLKey = "hub-url"

// hubURLEnv is the explicit environment variable name. HubURL reads it directly
// as a fallback for callers that resolve the hub URL before viper config has been
// initialized (e.g. unit tests); in a normal CLI run viper.GetString(hubURLKey)
// already honors it via AutomaticEnv.
const hubURLEnv = "PVTR_HUB_URL"

// wellKnownPath is the discovery document path served by the hub (ADR-0026)
// this should remain consistent for self-hosted registries as well
const wellKnownPath = "/.well-known/ext.grc-store"

// Discovery is the subset of the hub's /.well-known/ext.grc-store document
// that pvtr consumes. Only the fields pvtr acts on are decoded — registry_url
// (push/pull target), hub_url (for the claim-namespace hint), and the OIDC
// coordinates publish/login need; unknown fields are ignored.
type Discovery struct {
	// RegistryURL is the OCI registry origin WITH scheme (e.g.
	// http://localhost:5050). Use RegistryHost to get the scheme-stripped
	// host an oras/Docker reference needs.
	RegistryURL  string `json:"registry_url"`
	HubURL       string `json:"hub_url"`
	OIDCIssuer   string `json:"oidc_issuer,omitempty"`
	OIDCClientID string `json:"oidc_cli_client_id,omitempty"`
}

// HubURL returns the configured hub base URL with no trailing slash. Resolution
// precedence: the "hub-url" config key (config.yml or PVTR_HUB_URL env, via
// viper) first, then the PVTR_HUB_URL environment variable read directly (a
// fallback for pre-viper-init callers such as unit tests), then DefaultHubURL.
func HubURL() string {
	base := viper.GetString(hubURLKey)
	if base == "" {
		base = os.Getenv(hubURLEnv)
	}
	if base == "" {
		base = DefaultHubURL
	}
	return strings.TrimRight(base, "/")
}

// Client fetches the hub discovery document. It is intentionally tiny — a
// base URL and an HTTP client — so both publish and install share one
// resolution path.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient builds a discovery client against the configured hub
// (PVTR_HUB_URL, default DefaultHubURL).
func NewClient() *Client {
	return newHubClient(HubURL(), defaultHubTimeout)
}

// defaultHubTimeout bounds a single hub JSON API call.
const defaultHubTimeout = 15 * time.Second

// newHubClient returns a hub Client targeting baseURL with the given per-request
// timeout. Callers needing a longer bound than the default (e.g. /sync, which does
// server-side signature verification) pass their own.
func newHubClient(baseURL string, timeout time.Duration) *Client {
	return &Client{baseURL: baseURL, httpClient: &http.Client{Timeout: timeout}}
}

// BaseURL returns the hub base URL this client targets.
func (c *Client) BaseURL() string { return c.baseURL }

// httpStatusError is a non-200 response from the hub. It carries the status so
// callers can map a specific code (e.g. 404 → ErrPluginNotFound). body holds
// a bounded snippet of the response body when one was captured (e.g. from POST
// endpoints that return actionable JSON error codes); it is empty for plain GET
// discovery/browse paths where the body adds nothing useful.
type httpStatusError struct {
	method   string
	endpoint string
	status   int
	body     string // optional: non-empty snippet of the error response body
}

func (e *httpStatusError) Error() string {
	method := e.method
	if method == "" {
		method = http.MethodGet
	}
	if e.body != "" {
		return fmt.Sprintf("%s %s returned status %d: %s", method, e.endpoint, e.status, e.body)
	}
	return fmt.Sprintf("%s %s returned status %d", method, e.endpoint, e.status)
}

// doJSON is the shared HTTP request helper for all hub API calls. It:
//   - builds a request with the given method and path (relative to c.baseURL);
//   - marshals reqBody as JSON and sets Content-Type when reqBody is non-nil;
//   - sets Authorization: Bearer <bearer> when bearer is non-empty;
//   - on non-200, captures a bounded body snippet (4 KiB) and returns an
//     *httpStatusError — preserving actionable hub error codes (e.g.
//     plugin_unsigned) in error messages for POST endpoints;
//   - on 200, decodes the response body into out (when out is non-nil).
//
// It is the one place the build-request → Do → status → decode dance lives.
// getJSON is a thin convenience wrapper for anonymous GETs that decode into out.
func (c *Client) doJSON(ctx context.Context, method, path, bearer string, reqBody, out any) error {
	endpoint := c.baseURL + path
	var bodyReader io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("encoding request body for %s: %w", endpoint, err)
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bodyReader)
	if err != nil {
		return fmt.Errorf("building request for %s: %w", endpoint, err)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return &httpStatusError{method: method, endpoint: endpoint, status: resp.StatusCode, body: string(bytes.TrimSpace(detail))}
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decoding response from %s: %w", endpoint, err)
		}
	}
	return nil
}

// getJSON performs an anonymous GET against the hub and decodes a 200 response
// body into out. It is a thin wrapper over doJSON for the common anonymous-GET
// case (Discover, Browse, GetPluginDetails). A non-200 yields an *httpStatusError
// so a caller can inspect the code.
func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, "", nil, out)
}

// Discover fetches and decodes the hub's /.well-known/ext.grc-store document.
func (c *Client) Discover(ctx context.Context) (*Discovery, error) {
	var d Discovery
	if err := c.getJSON(ctx, wellKnownPath, &d); err != nil {
		return nil, err
	}
	if strings.TrimSpace(d.RegistryURL) == "" {
		return nil, fmt.Errorf("discovery document from %s%s has no registry_url", c.baseURL, wellKnownPath)
	}
	return &d, nil
}

// parseRegistryURL normalises registry_url into a *url.URL, applying the same
// whitespace-trimming that RegistryHost uses. It is the shared parse step that
// both RegistryHost and PlainHTTP delegate to so neither can drift.
//
// Values without a "scheme://" prefix are treated as bare host[:port] strings
// (no scheme → neither http:// nor https://). url.Parse mis-parses bare
// "localhost:5050" as scheme="localhost", opaque="5050", so we only call it
// when the separator is present.
func (d *Discovery) parseRegistryURL() (*url.URL, error) {
	raw := strings.TrimSpace(d.RegistryURL)
	if raw == "" {
		return nil, fmt.Errorf("registry_url is empty")
	}
	if !strings.Contains(raw, "://") {
		// No scheme present — treat as a bare host[:port] with an implied https.
		return &url.URL{Scheme: "https", Host: strings.TrimRight(raw, "/")}, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing registry_url %q: %w", raw, err)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("registry_url %q has no host", raw)
	}
	return u, nil
}

// RegistryHost returns the OCI registry host (no scheme, no trailing slash)
// to build an oras/Docker reference from. registry_url is advertised WITH a
// scheme (ADR-0026) but OCI references are host[:port]-only, so the scheme is
// stripped here — part of the same single-point-of-truth as PlainHTTP.
func (d *Discovery) RegistryHost() (string, error) {
	u, err := d.parseRegistryURL()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(u.Host, "/"), nil
}

// PlainHTTP reports whether the discovered registry_url uses http:// (local
// dev registries) and therefore requires plain-HTTP transport instead of TLS.
// Part of the same single-point-of-truth as RegistryHost — both delegate to
// parseRegistryURL so the scheme interpretation can never drift between them.
func (d *Discovery) PlainHTTP() bool {
	u, err := d.parseRegistryURL()
	if err != nil {
		return false
	}
	return u.Scheme == "http"
}
