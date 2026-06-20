// Package auth implements pvtr's OIDC device-grant login and credential
// storage for authenticated publishing to grc.store. It mirrors grcli's flow
// (ADR-0028): a device-authorization grant against the hub-advertised OIDC
// issuer + client, with tokens cached in a 0600 file under the XDG data dir.
// The consumer (install) path stays anonymous and does not touch this package.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Credentials is one issuer's saved tokens. Issuer doubles as the store key.
type Credentials struct {
	Issuer       string    `json:"issuer"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// renewalWindow is how far before ExpiresAt a token is treated as expired, so
// callers refresh before a push rather than mid-flight. credsFromTokenResponse
// (oidc.go) floors new-token lifetimes above this so a token is never born
// already-expired.
const renewalWindow = 60 * time.Second

// Expired reports whether the access token is at/near expiry (within the
// renewal window), so callers refresh before a push rather than mid-flight.
func (c *Credentials) Expired() bool {
	return time.Now().Add(renewalWindow).After(c.ExpiresAt)
}

// Store is pvtr's on-disk credential cache: a single JSON file at
// ${XDG_DATA_HOME:-~/.local/share}/pvtr/credentials.json, 0600, one entry per
// issuer. Same posture as grcli/gh/flyctl. Kept separate from grcli's store so
// the two tools don't fight over one file.
type Store struct {
	Path string
}

type storeFile struct {
	Version     int                     `json:"version"`
	Credentials map[string]*Credentials `json:"credentials"`
}

const currentStoreVersion = 1

// ErrNoCredentials is returned by Get when no entry exists for the issuer.
var ErrNoCredentials = errors.New("no stored credentials for this issuer (run `pvtr login`)")

// NewDefaultStore returns a Store at the standard XDG path.
func NewDefaultStore() (*Store, error) {
	dir, err := defaultStoreDir()
	if err != nil {
		return nil, err
	}
	return &Store{Path: filepath.Join(dir, "credentials.json")}, nil
}

func defaultStoreDir() (string, error) {
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "pvtr"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir for credential store: %w", err)
	}
	return filepath.Join(home, ".local", "share", "pvtr"), nil
}

// Get returns the credentials saved for issuer, or ErrNoCredentials.
func (s *Store) Get(issuer string) (*Credentials, error) {
	issuer = canonicalIssuer(issuer)
	if issuer == "" {
		return nil, errors.New("issuer is required")
	}
	f, err := s.load()
	if err != nil {
		return nil, err
	}
	c, ok := f.Credentials[issuer]
	if !ok || c == nil {
		return nil, ErrNoCredentials
	}
	c.Issuer = issuer
	return c, nil
}

// Put writes creds.Issuer → creds, replacing any existing entry. The file is
// rewritten atomically (temp + rename) with 0600 perms.
// NOTE: intentionally does NOT use utils.WriteFileAtomic — credentials require
// os.CreateTemp (kernel-assigned unique name) + os.File.Chmod(0600) before any
// write, so the 0600 mode is set before secret bytes land on disk. The shared
// helper's WriteFile+rename sequence cannot provide that ordering guarantee.
func (s *Store) Put(creds *Credentials) error {
	if creds == nil {
		return errors.New("credentials are required")
	}
	issuer := canonicalIssuer(creds.Issuer)
	if issuer == "" {
		return errors.New("credentials.Issuer is required")
	}
	creds.Issuer = issuer
	f, err := s.load()
	if err != nil {
		return err
	}
	if f.Credentials == nil {
		f.Credentials = map[string]*Credentials{}
	}
	f.Credentials[issuer] = creds
	return s.save(f)
}

// Delete removes the entry for issuer; a missing entry is not an error.
func (s *Store) Delete(issuer string) error {
	issuer = canonicalIssuer(issuer)
	if issuer == "" {
		return errors.New("issuer is required")
	}
	f, err := s.load()
	if err != nil {
		return err
	}
	delete(f.Credentials, issuer)
	return s.save(f)
}

func (s *Store) load() (*storeFile, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &storeFile{Version: currentStoreVersion, Credentials: map[string]*Credentials{}}, nil
		}
		return nil, fmt.Errorf("reading credential store %s: %w", s.Path, err)
	}
	f := &storeFile{}
	if err := json.Unmarshal(data, f); err != nil {
		return nil, fmt.Errorf("decoding credential store %s: %w (delete it to start over)", s.Path, err)
	}
	if f.Version != 0 && f.Version != currentStoreVersion {
		return nil, fmt.Errorf("credential store %s is version %d, pvtr only knows version %d", s.Path, f.Version, currentStoreVersion)
	}
	if f.Credentials == nil {
		f.Credentials = map[string]*Credentials{}
	}
	f.Version = currentStoreVersion
	return f, nil
}

func (s *Store) save(f *storeFile) error {
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating credential store directory %s: %w", dir, err)
	}
	f.Version = currentStoreVersion
	body, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding credential store: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "credentials-*.json.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Rename(tmpPath, s.Path); err != nil {
		return fmt.Errorf("renaming temp file to %s: %w", s.Path, err)
	}
	cleanup = false
	return nil
}

// canonicalIssuer trims trailing slashes/whitespace so the slash and no-slash
// issuer forms hit the same entry (Keycloak emits the no-slash iss).
func canonicalIssuer(s string) string {
	return strings.TrimRight(strings.TrimSpace(s), "/")
}
