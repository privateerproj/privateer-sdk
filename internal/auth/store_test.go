package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_PutGetDelete(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "credentials.json")}

	// Missing entry → ErrNoCredentials.
	if _, err := s.Get("https://auth.example/realms/gemara"); err != ErrNoCredentials {
		t.Fatalf("expected ErrNoCredentials, got %v", err)
	}

	creds := &Credentials{
		Issuer:      "https://auth.example/realms/gemara/", // trailing slash
		AccessToken: "tok-1",
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := s.Put(creds); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Canonical issuer (slash trimmed) round-trips.
	got, err := s.Get("https://auth.example/realms/gemara")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.AccessToken != "tok-1" {
		t.Errorf("access token = %q", got.AccessToken)
	}

	if err := s.Delete("https://auth.example/realms/gemara"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get("https://auth.example/realms/gemara"); err != ErrNoCredentials {
		t.Errorf("expected ErrNoCredentials after delete, got %v", err)
	}
}

func TestStore_File0600(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Path: filepath.Join(dir, "credentials.json")}
	if err := s.Put(&Credentials{Issuer: "https://i", AccessToken: "t", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("credential file perms = %o, want 600", perm)
	}
}

func TestCredentials_Expired(t *testing.T) {
	if !(&Credentials{ExpiresAt: time.Now().Add(10 * time.Second)}).Expired() {
		t.Error("token within the 60s renewal window must be Expired()")
	}
	if (&Credentials{ExpiresAt: time.Now().Add(time.Hour)}).Expired() {
		t.Error("token an hour out must not be Expired()")
	}
}

func TestCanonicalIssuer(t *testing.T) {
	for in, want := range map[string]string{
		"https://i/realms/g/": "https://i/realms/g",
		" https://i ":         "https://i",
		"https://i":           "https://i",
	} {
		if got := canonicalIssuer(in); got != want {
			t.Errorf("canonicalIssuer(%q) = %q, want %q", in, got, want)
		}
	}
}
