package oci

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowse_ParsesCoordinates(t *testing.T) {
	const body = `{"items":[
		{"namespace":"finos-ccc","plugin_id":"ccc-evaluator","latest_version":"2.1.0","signed":true},
		{"namespace":"ossf","plugin_id":"pvtr-github-repo","latest_version":"1.4.0","signed":true}
	]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != browsePath {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	items, err := c.Browse(context.Background())
	if err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Coordinate() != "finos-ccc/ccc-evaluator" {
		t.Errorf("coordinate = %q", items[0].Coordinate())
	}
	if items[1].Coordinate() != "ossf/pvtr-github-repo" {
		t.Errorf("coordinate = %q", items[1].Coordinate())
	}
}

func TestBrowse_Non200FailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := &Client{baseURL: srv.URL, httpClient: srv.Client()}
	if _, err := c.Browse(context.Background()); err == nil {
		t.Fatal("expected error on non-200 browse")
	}
}
