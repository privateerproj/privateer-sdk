package registry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestNewClient_DefaultBaseURL(t *testing.T) {
	orig := os.Getenv("PVTR_REGISTRY_URL")
	defer os.Setenv("PVTR_REGISTRY_URL", orig)
	os.Unsetenv("PVTR_REGISTRY_URL")

	c := NewClient()
	if c.BaseURL != DefaultBaseURL {
		t.Errorf("BaseURL: got %q, want %q", c.BaseURL, DefaultBaseURL)
	}
	if c.HTTPClient == nil {
		t.Error("HTTPClient should be set")
	}
}

func TestNewClient_EnvOverride(t *testing.T) {
	orig := os.Getenv("PVTR_REGISTRY_URL")
	defer os.Setenv("PVTR_REGISTRY_URL", orig)
	os.Setenv("PVTR_REGISTRY_URL", "https://custom.example.com/registry")

	c := NewClient()
	want := "https://custom.example.com/registry"
	if c.BaseURL != want {
		t.Errorf("BaseURL: got %q, want %q", c.BaseURL, want)
	}
}

func TestNewClient_TrimTrailingSlash(t *testing.T) {
	orig := os.Getenv("PVTR_REGISTRY_URL")
	defer os.Setenv("PVTR_REGISTRY_URL", orig)
	os.Setenv("PVTR_REGISTRY_URL", "https://example.com/")

	c := NewClient()
	if c.BaseURL != "https://example.com" {
		t.Errorf("BaseURL should be trimmed: got %q", c.BaseURL)
	}
}

func TestClient_GetVettedList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vetted-plugins.json" {
			http.NotFound(w, r)
			return
		}
		body := VettedListResponse{
			Message: "ok",
			Updated: "2025-01-01",
			Plugins: []string{"ossf/pvtr-github-repo-scanner"},
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()

	c := &Client{BaseURL: server.URL, HTTPClient: server.Client()}
	out, err := c.GetVettedList()
	if err != nil {
		t.Fatalf("GetVettedList: %v", err)
	}
	if out.Message != "ok" || out.Updated != "2025-01-01" {
		t.Errorf("unexpected response: %+v", out)
	}
	if len(out.Plugins) != 1 || out.Plugins[0] != "ossf/pvtr-github-repo-scanner" {
		t.Errorf("Plugins: got %v", out.Plugins)
	}
}

func TestClient_GetVettedList_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := &Client{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := c.GetVettedList()
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestClient_GetPluginData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/plugin-data/ossf/pvtr-github-repo-scanner.json" {
			http.NotFound(w, r)
			return
		}
		body := PluginData{
			Name:    "ossf/pvtr-github-repo-scanner",
			Source:  "https://github.com/ossf/pvtr-github-repo-scanner",
			Latest:  "0.19.2",
			DownloadURL: "https://example.com/dl.tar.gz",
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()

	c := &Client{BaseURL: server.URL, HTTPClient: server.Client()}
	out, err := c.GetPluginData("ossf/pvtr-github-repo-scanner")
	if err != nil {
		t.Fatalf("GetPluginData: %v", err)
	}
	if out.Name != "ossf/pvtr-github-repo-scanner" || out.Latest != "0.19.2" || out.DownloadURL != "https://example.com/dl.tar.gz" {
		t.Errorf("unexpected data: %+v", out)
	}
}

func TestClient_GetPluginData_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := &Client{BaseURL: server.URL, HTTPClient: server.Client()}
	_, err := c.GetPluginData("ossf/nonexistent")
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestClient_GetPluginData_InvalidName(t *testing.T) {
	c := NewClient()
	_, err := c.GetPluginData("")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	_, err = c.GetPluginData("owner/../repo")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}
