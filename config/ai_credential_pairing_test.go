package config

import (
	"strings"
	"testing"
)

// A config-pinned literal must not be sent to an endpoint that the environment
// redirected. Whoever exported PVTR_AI_BASE_URL may be trying a proxy or staging
// endpoint, but they did not author the checked-in credential.
func TestAICredentialEndpointPairing(t *testing.T) {
	const enabled = "ai_provider: openai\nai_model: model\n"

	tests := []struct {
		name       string
		document   string
		envBaseURL string
		envAPIKey  string
		wantErr    string
	}{{
		name:       "environment endpoint refuses to carry a configured literal",
		document:   enabled + "ai_api_key: config-pinned\n",
		envBaseURL: "https://attacker.example/v1",
		wantErr:    "PVTR_AI_BASE_URL",
	}, {
		name:       "environment endpoint that restates the configured one redirects nothing",
		document:   enabled + "ai_api_key: config-pinned\nai_base_url: https://trusted.example/v1\n",
		envBaseURL: "https://trusted.example/v1",
	}, {
		name:       "environment endpoint may carry an environment credential",
		document:   enabled,
		envBaseURL: "https://proxy.example/v1",
		envAPIKey:  "env-key",
	}, {
		name:     "configured endpoint may carry a configured literal",
		document: enabled + "ai_api_key: config-pinned\nai_base_url: https://proxy.example/v1\n",
	}, {
		name:      "process credential without a configured endpoint is left alone",
		document:  enabled,
		envAPIKey: "env-key",
	}, {
		name:      "a disabled run is never blocked",
		document:  enabled + "ai_skip: true\nai_base_url: https://attacker.example/v1\n",
		envAPIKey: "victim-ai-key",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loadConfig(t, tt.document)
			t.Setenv("PVTR_AI_BASE_URL", tt.envBaseURL)
			t.Setenv("PVTR_AI_API_KEY", tt.envAPIKey)

			err := applyAIPrecedenceRules(map[string]interface{}{}, aiSourcesForTarget("repo"))

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("applyAIPrecedenceRules() error = %v, want none", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("applyAIPrecedenceRules() = nil, want an error mentioning %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

// The refusal has to leave the operator somewhere to go, so it names both the
// setting that caused it and a way out.
func TestAICredentialEndpointPairing_ErrorsOfferAWayForward(t *testing.T) {
	t.Run("environment endpoint names the variable and the alternatives", func(t *testing.T) {
		loadConfig(t, "ai_provider: openai\nai_model: model\nai_api_key: config-pinned\n")
		t.Setenv("PVTR_AI_BASE_URL", "https://attacker.example/v1")
		t.Setenv("PVTR_AI_API_KEY", "")

		err := applyAIPrecedenceRules(map[string]interface{}{}, aiSourcesForTarget("repo"))
		if err == nil {
			t.Fatal("applyAIPrecedenceRules() = nil, want an error")
		}
		for _, want := range []string{"PVTR_AI_BASE_URL", "PVTR_AI_API_KEY", "ai_api_key_env"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %v, want it to mention %q", err, want)
			}
		}
	})
}

// A refusal that printed the credential would defeat its own purpose.
func TestAICredentialEndpointPairing_ErrorsDoNotEmitTheCredential(t *testing.T) {
	loadConfig(t, "ai_provider: openai\nai_model: model\nai_api_key: config-pinned-secret\n")
	t.Setenv("PVTR_AI_BASE_URL", "https://attacker.example/v1")
	t.Setenv("PVTR_AI_API_KEY", "")

	err := applyAIPrecedenceRules(map[string]interface{}{}, aiSourcesForTarget("repo"))
	if err == nil {
		t.Fatal("applyAIPrecedenceRules() = nil, want an error")
	}
	if strings.Contains(err.Error(), "config-pinned-secret") {
		t.Fatalf("error = %v, want it to withhold the credential", err)
	}
}
