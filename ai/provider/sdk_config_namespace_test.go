package provider

import (
	"strings"
	"testing"

	sdkconfig "github.com/privateerproj/privateer-sdk/config"
)

// ai_api_key_env is the one setting that reaches the environment outside
// Viper's PVTR_ prefix, because it dereferences a caller-supplied name with
// os.Getenv. Left unbounded it lets a configuration file read any secret the
// process holds and pair it with an endpoint of the file's choosing, which
// matters because Privateer searches the working directory ahead of
// ~/.privateer: running inside an untrusted repository is enough.
func TestAPIKeyFromEnv_BoundedToThePrivateerNamespace(t *testing.T) {
	tests := []struct {
		name     string
		envName  string
		envValue string
		wantErr  string
	}{{
		name:     "refuses a credential outside the namespace",
		envName:  "GITHUB_TOKEN",
		envValue: "victim-github-token",
		wantErr:  "must begin with PVTR_AI_",
	}, {
		name:     "refuses a cloud credential outside the namespace",
		envName:  "AWS_SECRET_ACCESS_KEY",
		envValue: "victim-aws-secret",
		wantErr:  "must begin with PVTR_AI_",
	}, {
		name:     "refuses a provider credential the operator never scoped to Privateer",
		envName:  "OPENAI_API_KEY",
		envValue: "victim-openai-key",
		wantErr:  "must begin with PVTR_AI_",
	}, {
		name:     "accepts the shared name",
		envName:  "PVTR_AI_API_KEY",
		envValue: "operator-key",
	}, {
		name:     "accepts a per-target name",
		envName:  "PVTR_AI_KEY_REPO_ONE",
		envValue: "operator-key",
	}, {
		name:    "an empty name still reports the empty name",
		envName: "",
		wantErr: "must name a non-empty environment variable",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envName != "" {
				t.Setenv(tt.envName, tt.envValue)
			}

			key, err := apiKeyFromEnv(tt.envName)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("apiKeyFromEnv(%q) error = %v, want none", tt.envName, err)
				}
				if key != tt.envValue {
					t.Fatalf("apiKeyFromEnv(%q) = %q, want %q", tt.envName, key, tt.envValue)
				}
				return
			}
			if err == nil {
				t.Fatalf("apiKeyFromEnv(%q) = %q, want an error mentioning %q", tt.envName, key, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to mention %q", err, tt.wantErr)
			}
			if key != "" {
				t.Fatalf("apiKeyFromEnv(%q) returned %q alongside an error, want no credential", tt.envName, key)
			}
		})
	}
}

// The refusal must not leak the secret it just declined to read.
func TestAPIKeyFromEnv_RefusalWithholdsTheCredential(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "victim-github-token")

	_, err := apiKeyFromEnv("GITHUB_TOKEN")
	if err == nil {
		t.Fatal("apiKeyFromEnv() = nil error, want a refusal")
	}
	if strings.Contains(err.Error(), "victim-github-token") {
		t.Fatalf("error = %v, want it to withhold the credential", err)
	}
	if !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Fatalf("error = %v, want it to name the rejected variable", err)
	}
}

// The end-to-end shape of the attack the bound exists to stop: a configuration
// picks both halves, naming a secret it does not own and a host it does.
func TestConfigFromSDKConfig_ConfigCannotPairAForeignSecretWithItsOwnEndpoint(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "victim-github-token")

	cfg := sdkconfig.Config{ServiceName: "repo", Vars: map[string]interface{}{
		"ai_provider":    "openai",
		"ai_model":       "model",
		"ai_base_url":    "https://attacker.example/v1",
		"ai_api_key_env": "GITHUB_TOKEN",
	}}

	got, _, err := ConfigFromSDKConfig(cfg)
	if err == nil {
		t.Fatal("ConfigFromSDKConfig() = nil error, want a refusal")
	}
	if got.APIKey != "" {
		t.Fatalf("APIKey = %q, want no credential carried to %q", got.APIKey, got.BaseURL)
	}
	if strings.Contains(err.Error(), "victim-github-token") {
		t.Fatalf("error = %v, want it to withhold the credential", err)
	}
}
