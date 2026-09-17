// Package oci holds the grc.store OCI mechanics shared by `pvtr publish`
// (push a signed plugin index) and `pvtr install` (pull + verify one).
//
// This file is the first, deliberately small member: hub discovery. A user
// configures exactly ONE endpoint — the hub base URL — and the OCI registry
// host is learned from the hub's /.well-known/grc-store-configuration
// document. Nothing in pvtr hardcodes the registry host, mirroring how the
// hub advertises it via HUB_OCI_PUBLIC_URL.
package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// Client issues pvtr's anonymous hub JSON calls: Browse and GetPluginDetails.
// For the well-known discovery document use grc-store-clientkit's
// hub.Discover(ctx, c.BaseURL()), which owns the fetch, the registry_url check
// and the per-URL cache.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient builds a discovery client against the configured hub
// (PVTR_HUB_URL, default DefaultHubURL).
func NewClient() *Client {
	return &Client{baseURL: HubURL(), httpClient: &http.Client{Timeout: hubTimeout}}
}

// hubTimeout bounds a single hub JSON API call. One bound serves every call this
// client makes: they are all anonymous GETs returning small documents.
const hubTimeout = 15 * time.Second

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

// getJSON performs an anonymous GET against the hub and decodes a 200 response
// body into out. Every hub call from this package is an anonymous GET (Browse,
// GetPluginDetails). A non-200 yields an *httpStatusError so a caller can
// inspect the code, preserving actionable hub error codes in messages.
func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	endpoint := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("building request for %s: %w", endpoint, err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return &httpStatusError{method: http.MethodGet, endpoint: endpoint, status: resp.StatusCode, body: string(bytes.TrimSpace(detail))}
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decoding response from %s: %w", endpoint, err)
		}
	}
	return nil
}
