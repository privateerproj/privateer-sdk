package registry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const defaultBaseURL = "https://revanite.io/privateer"

// Client is an HTTP client for the Privateer plugin registry.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// VettedListResponse is the response from the vetted plugins endpoint.
type VettedListResponse struct {
	Message string   `json:"message"`
	Updated string   `json:"updated"`
	Plugins []string `json:"plugins"`
}

// PluginData is the metadata for a single plugin in the registry.
type PluginData struct {
	Name              string   `json:"name"`
	Source            string   `json:"source"`
	Image             string   `json:"image"`
	Latest            string   `json:"latest"`
	SupportedVersions []string `json:"supported-versions"`
	Download          string   `json:"download"`
	BinaryPath        string   `json:"binaryPath"`
}

// NewClient creates a new registry client.
// The base URL can be overridden with the PVTR_REGISTRY_URL environment variable.
func NewClient() *Client {
	base := os.Getenv("PVTR_REGISTRY_URL")
	if base == "" {
		base = defaultBaseURL
	}
	return &Client{
		baseURL: base,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// GetVettedList fetches the list of vetted plugins from the registry.
func (c *Client) GetVettedList() (*VettedListResponse, error) {
	url := fmt.Sprintf("%s/vetted-plugins.json", c.baseURL)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned status %d", url, resp.StatusCode)
	}

	var result VettedListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response from %s: %w", url, err)
	}
	return &result, nil
}

// GetPluginData fetches metadata for a specific plugin from the registry.
func (c *Client) GetPluginData(owner, repo string) (*PluginData, error) {
	url := fmt.Sprintf("%s/plugin-data/%s/%s.json", c.baseURL, owner, repo)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("plugin %s/%s not found in registry", owner, repo)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned status %d", url, resp.StatusCode)
	}

	var result PluginData
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response from %s: %w", url, err)
	}
	return &result, nil
}
