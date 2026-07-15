package provider

import (
	"fmt"
	"strings"
	"testing"
)

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
