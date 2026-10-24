package provider

import (
	"fmt"
	"strings"
	"testing"
)

func TestConfigValidate_APIKeyRequirementDependsOnBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name:    "default endpoint requires credential",
			config:  Config{Provider: "openai", Model: "gpt-4o-mini"},
			wantErr: true,
		},
		{
			name:   "custom endpoint permits no credential",
			config: Config{Provider: "openai", Model: "gpt-4o-mini", BaseURL: "http://127.0.0.1:8000/v1"},
		},
		{
			name:   "custom endpoint retains credential",
			config: Config{Provider: "openai", Model: "gpt-4o-mini", BaseURL: "https://gateway.example/v1", APIKey: "gateway-key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, tt.wantErr)
			}
		})
	}
}

func TestConfigValidate_BaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantErr bool
	}{
		{name: "HTTPS API root", baseURL: "https://gateway.example/v1"},
		{name: "HTTP local server", baseURL: "http://localhost:8000/v1"},
		{name: "IPv4 local server", baseURL: "http://127.0.0.1:8000/v1"},
		{name: "IPv6 local server", baseURL: "http://[::1]:8000/v1"},
		{name: "IPv6 without port", baseURL: "http://[::1]/v1"},
		{name: "uppercase scheme", baseURL: "HTTPS://gateway.example/v1"},
		{name: "trailing slash", baseURL: "https://gateway.example/v1/"},
		{name: "minimum port", baseURL: "http://localhost:1/v1"},
		{name: "maximum port", baseURL: "http://localhost:65535/v1"},
		{name: "escaped path delimiters", baseURL: "https://gateway.example/api%3Fv1%23root"},
		{name: "malformed URL", baseURL: "://bad", wantErr: true},
		{name: "relative URL", baseURL: "/v1", wantErr: true},
		{name: "scheme-relative URL", baseURL: "//gateway.example/v1", wantErr: true},
		{name: "missing scheme", baseURL: "localhost:8000", wantErr: true},
		{name: "missing host", baseURL: "https:///v1", wantErr: true},
		{name: "port without hostname", baseURL: "http://:8000/v1", wantErr: true},
		{name: "empty IPv6 host", baseURL: "http://[]:8000/v1", wantErr: true},
		{name: "unsupported scheme", baseURL: "ftp://gateway.example/v1", wantErr: true},
		{name: "userinfo", baseURL: "https://secret-user:secret-password@gateway.example/v1", wantErr: true},
		{name: "empty userinfo", baseURL: "https://@gateway.example/v1", wantErr: true},
		{name: "query", baseURL: "https://gateway.example/v1?token=secret-token", wantErr: true},
		{name: "empty query", baseURL: "https://gateway.example/v1?", wantErr: true},
		{name: "fragment", baseURL: "https://gateway.example/v1#secret-fragment", wantErr: true},
		{name: "empty fragment", baseURL: "https://gateway.example/v1#", wantErr: true},
		{name: "invalid escape", baseURL: "https://gateway.example/secret-path%zz", wantErr: true},
		{name: "invalid port", baseURL: "https://gateway.example:secret-port/v1", wantErr: true},
		{name: "empty port", baseURL: "https://gateway.example:/v1", wantErr: true},
		{name: "zero port", baseURL: "https://gateway.example:0/v1", wantErr: true},
		{name: "negative port", baseURL: "https://gateway.example:-1/v1", wantErr: true},
		{name: "out of range port", baseURL: "https://gateway.example:65536/v1", wantErr: true},
		{name: "overflowing port", baseURL: "https://gateway.example:999999999999999999999999/v1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, apiKey := range []string{"", "key"} {
				cfg := Config{Provider: "openai", Model: "model", APIKey: apiKey, BaseURL: tt.baseURL}
				err := cfg.Validate()
				if (err != nil) != tt.wantErr {
					t.Fatalf("Validate() with API key set=%t error = %v, wantErr %t", apiKey != "", err, tt.wantErr)
				}
				if err != nil {
					if !strings.HasPrefix(err.Error(), "ai base url ") {
						t.Errorf("Validate() error = %v, want base URL validation error", err)
					}
					if strings.Contains(err.Error(), tt.baseURL) || strings.Contains(err.Error(), "secret-") {
						t.Errorf("Validate() error includes the URL or its secrets")
					}
				}
			}
		})
	}
}

// Config.String must keep the credential out of any fmt-formatted output, since
// %v/%+v on an adapter (or its embedded Base) recurses into Config.
func TestConfigString_RedactsAPIKey(t *testing.T) {
	config := Config{
		Provider: "openai",
		APIKey:   "sk-super-secret",
		Model:    "gpt-4o-mini",
	}

	for _, formatted := range []string{
		config.String(),
		fmt.Sprintf("%v", config),
		fmt.Sprintf("%+v", config),
	} {
		if strings.Contains(formatted, "sk-super-secret") {
			t.Fatalf("formatted config leaks the api key: %s", formatted)
		}
		if !strings.Contains(formatted, "<redacted>") {
			t.Errorf("formatted config should mark the api key redacted: %s", formatted)
		}
	}

	if got := (Config{}).String(); !strings.Contains(got, "<unset>") {
		t.Errorf("empty config should report the api key unset: %s", got)
	}
}

// A base URL is redirectable per run through PVTR_AI_BASE_URL while the
// credential stays pinned in the configuration, so plain HTTP off the local
// host would put a config-pinned key on the wire in clear text.
func TestConfigValidate_CredentialRequiresHTTPSOffLoopback(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string
		wantKeyErr bool
	}{
		{name: "https remote", baseURL: "https://gateway.example/v1"},
		{name: "http localhost", baseURL: "http://localhost:8000/v1"},
		{name: "http loopback IPv4", baseURL: "http://127.0.0.1:8000/v1"},
		{name: "http loopback in 127/8", baseURL: "http://127.9.9.9:8000/v1"},
		{name: "http loopback IPv6", baseURL: "http://[::1]:8000/v1"},
		{name: "uppercase scheme and host", baseURL: "HTTP://LOCALHOST:8000/v1"},
		{name: "http remote host", baseURL: "http://gateway.example/v1", wantKeyErr: true},
		{name: "http remote IP", baseURL: "http://10.0.0.5:8000/v1", wantKeyErr: true},
		{name: "http lookalike host", baseURL: "http://localhost.example/v1", wantKeyErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withKey := Config{Provider: "openai", Model: "model", APIKey: "key", BaseURL: tt.baseURL}
			err := withKey.Validate()
			if (err != nil) != tt.wantKeyErr {
				t.Fatalf("Validate() with a key: error = %v, wantErr %t", err, tt.wantKeyErr)
			}
			if err != nil && !strings.Contains(err.Error(), "https") {
				t.Errorf("Validate() error = %v, want it to name the https requirement", err)
			}

			// The rule is about the credential: the same endpoint without one
			// is the local-model case Validate deliberately permits.
			keyless := Config{Provider: "openai", Model: "model", BaseURL: tt.baseURL}
			if err := keyless.Validate(); err != nil {
				t.Errorf("Validate() without a key: error = %v, want nil", err)
			}
		})
	}
}
