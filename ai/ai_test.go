package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/privateerproj/privateer-sdk/ai/openai"
	sdkconfig "github.com/privateerproj/privateer-sdk/config"
	"github.com/spf13/viper"
)

// stubProviderClient is a minimal Client for exercising the factory registry.
type stubProviderClient struct{}

func (stubProviderClient) Analyze(context.Context, string, string, *Schema) (*AnalyzeResponse, error) {
	return nil, nil
}

func TestNewClientWithAIConfig_OpenAI(t *testing.T) {
	client, err := NewClientWithAIConfig(Config{
		Provider: ProviderOpenAI,
		APIKey:   "test-key",
		Model:    "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := client.(*openai.Client); !ok {
		t.Fatalf("expected *openai.Client, got %T", client)
	}
}

func TestNewClientWithAIConfig_UsesFactoryRegistry(t *testing.T) {
	const testProvider Provider = "test-provider"

	originalFactory, hadOriginal := clientFactories[testProvider]
	defer func() {
		if hadOriginal {
			clientFactories[testProvider] = originalFactory
		} else {
			delete(clientFactories, testProvider)
		}
	}()

	stub := stubProviderClient{}
	clientFactories[testProvider] = func(config Config) Client {
		return stub
	}

	client, err := NewClientWithAIConfig(Config{
		Provider: testProvider,
		APIKey:   "test-key",
		Model:    "claude",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client != stub {
		t.Fatalf("expected factory-constructed client, got %T", client)
	}
}

func TestNewClientWithAIConfig_UnknownProvider(t *testing.T) {
	_, err := NewClientWithAIConfig(Config{
		Provider: "no-such-provider",
		APIKey:   "test-key",
		Model:    "gpt-4o-mini",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported ai provider") {
		t.Fatalf("expected unsupported provider error, got %v", err)
	}
}

func TestNewClientWithAIConfig_Validate(t *testing.T) {
	_, err := NewClientWithAIConfig(Config{Provider: ProviderOpenAI, Model: "gpt-4o-mini"})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestNewClientWithAIConfig_NormalizesBeforeDispatch(t *testing.T) {
	client, err := NewClientWithAIConfig(Config{
		Provider: Provider(" OpenAI "),
		APIKey:   "test-key",
		Model:    "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	openAIClient, ok := client.(*openai.Client)
	if !ok {
		t.Fatalf("expected *openai.Client, got %T", client)
	}
	if openAIClient.Config.MaxTokens == 0 {
		t.Fatal("expected adapter to receive normalized config with default max tokens")
	}
}

func TestNewClient(t *testing.T) {
	client, err := NewClient(sdkconfig.Config{Vars: map[string]interface{}{
		"ai_provider": "openai",
		"ai_model":    "gpt-4o-mini",
		"ai_api_key":  "test-key",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected configured client, got nil")
	}
	if _, ok := client.(*openai.Client); !ok {
		t.Fatalf("expected *openai.Client, got %T", client)
	}
}

func TestNewClient_Unconfigured(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	client, err := NewClient(sdkconfig.Config{Vars: map[string]interface{}{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client != nil {
		t.Fatalf("expected nil client when AI is unconfigured, got %T", client)
	}
}

func TestNewClient_PartialLiveConfigErrors(t *testing.T) {
	tests := []struct {
		name        string
		vars        map[string]interface{}
		wantErrText string
	}{
		{
			name: "provider only",
			vars: map[string]interface{}{
				"ai_provider": "openai",
			},
			wantErrText: "ai model is required",
		},
		{
			name: "model only",
			vars: map[string]interface{}{
				"ai_model": "gpt-4o-mini",
			},
			wantErrText: "ai provider is required",
		},
		{
			name: "provider and model without api key",
			vars: map[string]interface{}{
				"ai_provider": "openai",
				"ai_model":    "gpt-4o-mini",
			},
			wantErrText: "ai api key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)

			client, err := NewClient(sdkconfig.Config{Vars: tt.vars})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrText) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErrText)
			}
			if client != nil {
				t.Fatalf("expected nil client, got %T", client)
			}
		})
	}
}
