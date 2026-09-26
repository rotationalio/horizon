package horizon_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
)

// Verifies the root factory registers and constructs built-in providers without
// requiring callers to import implementation packages.
func TestNewProviderRegistersBuiltins(t *testing.T) {
	tests := []struct {
		name   string
		config provider.Config
	}{
		{
			name: "Mock",
			config: provider.Config{
				APIType:      provider.APITypeMock,
				ProviderType: provider.ProviderTypeMock,
				Credentials:  auth.NewNone(),
			},
		},
		{
			name: "OpenAI",
			config: provider.Config{
				APIType:           provider.APITypeOpenAIResponses,
				ProviderType:      provider.ProviderTypeOpenAI,
				InferenceEndpoint: "https://api.openai.com/v1",
				CatalogEndpoint:   "https://api.openai.com/v1/models",
				Credentials:       auth.NewAPIKey("secret"),
			},
		},
		{
			name: "OpenAICompatible",
			config: provider.Config{
				APIType:           provider.APITypeOpenAIChatCompletions,
				ProviderType:      provider.ProviderTypeOpenAICompatible,
				InferenceEndpoint: "http://localhost:11434/v1",
				CatalogEndpoint:   "http://localhost:11434/v1/models",
				DefaultModel:      "local/model",
				Credentials:       auth.NewNone(),
			},
		},
		{
			name: "OpenRouter",
			config: provider.Config{
				APIType:           provider.APITypeOpenAIChatCompletions,
				ProviderType:      provider.ProviderTypeOpenRouter,
				InferenceEndpoint: "https://openrouter.ai/api/v1",
				CatalogEndpoint:   "https://openrouter.ai/api/v1/models",
				Credentials:       auth.NewAPIKey("secret"),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := horizon.NewProvider(tc.config)
			require.NoError(t, err)
			require.NotNil(t, actual)
		})
	}
}
