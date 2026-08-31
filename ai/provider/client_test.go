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
