package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// The endpoint and the credential must not arrive from opposite sides of the
// configuration/environment boundary. Whoever picks the host a credential is
// sent to should be the same party that supplied the credential, because
// otherwise one of them is spending a secret the other never agreed to share.
//
// Both directions are covered here:
//
//   - The environment redirects an endpoint while the file pins the key, which
//     happens when a PVTR_AI_BASE_URL is left exported in a shell or CI job.
//   - The file picks an endpoint while the environment supplies the key, which
//     happens when a config.yml in the working directory is picked up ahead of
//     the one in ~/.privateer and borrows an exported PVTR_AI_API_KEY.
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
		name:      "configured endpoint refuses to capture the process credential",
		document:  enabled + "ai_base_url: https://attacker.example/v1\n",
		envAPIKey: "victim-ai-key",
		wantErr:   "ai_base_url is set in configuration",
	}, {
		name:      "configured endpoint may use a variable the file names",
		document:  enabled + "ai_base_url: https://proxy.example/v1\nai_api_key_env: PVTR_AI_API_KEY\n",
		envAPIKey: "victim-ai-key",
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

	t.Run("configured endpoint names the variable and the alternatives", func(t *testing.T) {
		loadConfig(t, "ai_provider: openai\nai_model: model\nai_base_url: https://attacker.example/v1\n")
		t.Setenv("PVTR_AI_BASE_URL", "")
		t.Setenv("PVTR_AI_API_KEY", "victim-ai-key")

		err := applyAIPrecedenceRules(map[string]interface{}{}, aiSourcesForTarget("repo"))
		if err == nil {
			t.Fatal("applyAIPrecedenceRules() = nil, want an error")
		}
		for _, want := range []string{"PVTR_AI_API_KEY", "ai_api_key_env", "PVTR_AI_BASE_URL"} {
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

// The pairing rule must hold on the plain-Viper path as well. A caller that
// loads configuration with viper.ReadInConfig rather than config.ReadInConfig
// leaves no captured file copy, and the rule asks only whether the file chose
// the endpoint, so a missing copy must not silence it. Before this was fixed,
// a root-level ai_base_url paired with an exported PVTR_AI_API_KEY resolved
// without error on this path.
func TestAICredentialEndpointPairing_WithoutCapturedFileCopy(t *testing.T) {
	const enabled = "ai_provider: openai\nai_model: model\n"

	tests := []struct {
		name     string
		document string
		wantErr  bool
	}{{
		name:     "configured endpoint still refuses to capture the process credential",
		document: enabled + "ai_base_url: https://attacker.example/v1\n",
		wantErr:  true,
	}, {
		name:     "a named variable is still read as consent",
		document: enabled + "ai_base_url: https://proxy.example/v1\nai_api_key_env: PVTR_AI_API_KEY\n",
	}, {
		name:     "no configured endpoint is still left alone",
		document: enabled,
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loadConfigWithoutCapture(t, tt.document)
			t.Setenv("PVTR_AI_API_KEY", "victim-ai-key")
			t.Setenv("PVTR_AI_BASE_URL", "")

			sources := aiSettingSources{file: rawFileSettings()}
			if sources.file != nil {
				t.Fatal("rawFileSettings() captured a copy, so this path is not under test")
			}

			err := applyAIPrecedenceRules(map[string]interface{}{}, sources)
			if tt.wantErr && err == nil {
				t.Fatal("applyAIPrecedenceRules() = nil, want an error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("applyAIPrecedenceRules() = %v, want nil", err)
			}
		})
	}
}

// loadConfigWithoutCapture loads a document through Viper alone, leaving the
// package with no captured file copy, as an SDK caller that never routes
// through config.ReadConfig would.
func loadConfigWithoutCapture(t *testing.T, document string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetConfigType("yaml")
	if err := viper.ReadConfig(strings.NewReader(document)); err != nil {
		t.Fatal(err)
	}
}
