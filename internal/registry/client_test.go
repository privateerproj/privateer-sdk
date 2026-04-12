package registry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient_DefaultBaseURL(t *testing.T) {
	t.Setenv("PVTR_REGISTRY_URL", "")
	client := NewClient()
	if client.baseURL != defaultBaseURL {
		t.Errorf("expected default base URL %q, got %q", defaultBaseURL, client.baseURL)
	}
}

func TestNewClient_CustomBaseURL(t *testing.T) {
	t.Setenv("PVTR_REGISTRY_URL", "https://custom.registry.io")
	client := NewClient()
	if client.baseURL != "https://custom.registry.io" {
		t.Errorf("expected custom base URL, got %q", client.baseURL)
	}
}

func TestGetVettedList(t *testing.T) {
	var tests = []struct {
		name          string
		statusCode    int
		body          VettedListResponse
		expectError   bool
		expectPlugins []string
	}{
		{
			name:       "successful response with plugins",
			statusCode: http.StatusOK,
			body: VettedListResponse{
				Message: "ok",
				Updated: "2025-03-14",
				Plugins: []string{"ossf/pvtr-github-repo-scanner", "privateerproj/pvtr-example"},
			},
			expectError:   false,
			expectPlugins: []string{"ossf/pvtr-github-repo-scanner", "privateerproj/pvtr-example"},
		},
		{
			name:        "server error",
			statusCode:  http.StatusInternalServerError,
			body:        VettedListResponse{},
			expectError: true,
		},
		{
			name:        "404 not found",
			statusCode:  http.StatusNotFound,
			body:        VettedListResponse{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/vetted-plugins.json" {
					t.Errorf("unexpected request path: %s", r.URL.Path)
				}
				w.WriteHeader(tt.statusCode)
				if tt.statusCode == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tt.body)
				}
			}))
			defer server.Close()

			client := &Client{baseURL: server.URL, httpClient: server.Client()}
			resp, err := client.GetVettedList()

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(resp.Plugins) != len(tt.expectPlugins) {
				t.Fatalf("expected %d plugins, got %d", len(tt.expectPlugins), len(resp.Plugins))
			}
			for i, name := range tt.expectPlugins {
				if resp.Plugins[i] != name {
					t.Errorf("plugin[%d]: expected %q, got %q", i, name, resp.Plugins[i])
				}
			}
		})
	}
}

func TestGetPluginData(t *testing.T) {
	var tests = []struct {
		name        string
		owner       string
		repo        string
		statusCode  int
		body        PluginData
		expectError bool
		expectPath  string
	}{
		{
			name:       "successful response",
			owner:      "ossf",
			repo:       "pvtr-github-repo-scanner",
			statusCode: http.StatusOK,
			body: PluginData{
				Name:   "ossf/pvtr-github-repo-scanner",
				Source: "https://github.com/ossf/pvtr-github-repo-scanner",
				Latest: "0.19.2",
			},
			expectError: false,
			expectPath:  "/plugin-data/ossf/pvtr-github-repo-scanner.json",
		},
		{
			name:        "plugin not found",
			owner:       "unknown",
			repo:        "missing-plugin",
			statusCode:  http.StatusNotFound,
			body:        PluginData{},
			expectError: true,
			expectPath:  "/plugin-data/unknown/missing-plugin.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.expectPath {
					t.Errorf("unexpected request path: got %s, expected %s", r.URL.Path, tt.expectPath)
				}
				w.WriteHeader(tt.statusCode)
				if tt.statusCode == http.StatusOK {
					_ = json.NewEncoder(w).Encode(tt.body)
				}
			}))
			defer server.Close()

			client := &Client{baseURL: server.URL, httpClient: server.Client()}
			data, err := client.GetPluginData(tt.owner, tt.repo)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if data.Name != tt.body.Name {
				t.Errorf("expected name %q, got %q", tt.body.Name, data.Name)
			}
			if data.Source != tt.body.Source {
				t.Errorf("expected source %q, got %q", tt.body.Source, data.Source)
			}
			if data.Latest != tt.body.Latest {
				t.Errorf("expected latest %q, got %q", tt.body.Latest, data.Latest)
			}
		})
	}
}
