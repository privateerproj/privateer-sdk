package command

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestSetListCmdFlags(t *testing.T) {
	cmd := GetListCmd(&flushWriter{Buffer: &bytes.Buffer{}})
	SetListCmdFlags(cmd)
	for _, name := range []string{"all", "installable", "installed"} {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("flag %q not registered", name)
		}
	}
}

func TestContains(t *testing.T) {
	plugins := []*PluginPkg{
		{Name: "a", Available: true},
		{Name: "b", Available: false},
	}
	if !Contains(plugins, "a") {
		t.Error("Contains(plugins, \"a\") should be true")
	}
	if !Contains(plugins, "b") {
		t.Error("Contains(plugins, \"b\") should be true")
	}
	if Contains(plugins, "c") {
		t.Error("Contains(plugins, \"c\") should be false")
	}
	if Contains(nil, "a") {
		t.Error("Contains(nil, \"a\") should be false")
	}
}

func TestFetchVettedPlugins_ObjectShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vetted-pvtr-plugins.json" {
			http.NotFound(w, r)
			return
		}
		body := map[string]interface{}{
			"message": "ok",
			"updated": "2025-01-01",
			"plugins": []string{"ossf/pvtr-github-repo-scanner", "other/plugin"},
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()

	orig := os.Getenv("PVTR_REGISTRY_URL")
	defer os.Setenv("PVTR_REGISTRY_URL", orig)
	os.Setenv("PVTR_REGISTRY_URL", server.URL)

	list, err := fetchVettedPlugins()
	if err != nil {
		t.Fatalf("fetchVettedPlugins: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(list))
	}
	if list[0].Name != "ossf/pvtr-github-repo-scanner" || list[1].Name != "other/plugin" {
		t.Errorf("unexpected names: %q, %q", list[0].Name, list[1].Name)
	}
}

func TestFetchVettedPlugins_ArrayShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vetted-pvtr-plugins.json" {
			http.NotFound(w, r)
			return
		}
		body := []string{"one/plugin", "two/plugin"}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()

	orig := os.Getenv("PVTR_REGISTRY_URL")
	defer os.Setenv("PVTR_REGISTRY_URL", orig)
	os.Setenv("PVTR_REGISTRY_URL", server.URL)

	list, err := fetchVettedPlugins()
	if err != nil {
		t.Fatalf("fetchVettedPlugins: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(list))
	}
	if list[0].Name != "one/plugin" || list[1].Name != "two/plugin" {
		t.Errorf("unexpected names: %q, %q", list[0].Name, list[1].Name)
	}
}

func TestFetchVettedPlugins_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	orig := os.Getenv("PVTR_REGISTRY_URL")
	defer os.Setenv("PVTR_REGISTRY_URL", orig)
	os.Setenv("PVTR_REGISTRY_URL", server.URL)

	_, err := fetchVettedPlugins()
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention 404: %v", err)
	}
}

func TestFetchVettedPlugins_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vetted-pvtr-plugins.json" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	orig := os.Getenv("PVTR_REGISTRY_URL")
	defer os.Setenv("PVTR_REGISTRY_URL", orig)
	os.Setenv("PVTR_REGISTRY_URL", server.URL)

	_, err := fetchVettedPlugins()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

