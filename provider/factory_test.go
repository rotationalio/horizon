package provider_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/mock"
	"go.rtnl.ai/horizon/provider/openai"
	"go.rtnl.ai/horizon/provider/openrouter"
)

// Verifies registered built-in providers are constructed for valid
// configurations and incompatible API types are rejected.
func TestNew(t *testing.T) {
	t.Run("OpenAI", func(t *testing.T) {
		actual, err := provider.New(provider.Config{
			APIType:           provider.APITypeOpenAIResponses,
			ProviderType:      provider.ProviderTypeOpenAI,
			InferenceEndpoint: "https://api.openai.com/v1",
			CatalogEndpoint:   "https://api.openai.com/v1/models",
			Credentials:       auth.NewAPIKey("secret"),
		})
		require.NoError(t, err)
		require.IsType(t, &openai.Provider{}, actual)
	})

	t.Run("OpenRouter", func(t *testing.T) {
		actual, err := provider.New(provider.Config{
			APIType:           provider.APITypeOpenAIChatCompletions,
			ProviderType:      provider.ProviderTypeOpenRouter,
			InferenceEndpoint: "https://openrouter.ai/api/v1",
			CatalogEndpoint:   "https://openrouter.ai/api/v1/models",
			Credentials:       auth.NewAPIKey("secret"),
		})
		require.NoError(t, err)
		require.IsType(t, &openrouter.Provider{}, actual)
	})

	t.Run("OpenAICompatibleWithoutAuth", func(t *testing.T) {
		actual, err := provider.New(provider.Config{
			APIType:           provider.APITypeOpenAIChatCompletions,
			ProviderType:      provider.ProviderTypeOpenAICompatible,
			InferenceEndpoint: "http://localhost:11434/v1",
			CatalogEndpoint:   "http://localhost:11434/v1/models",
			DefaultModel:      "local/model",
			Credentials:       auth.NewNone(),
		})
		require.NoError(t, err)
		require.IsType(t, &openai.Provider{}, actual)
	})

	t.Run("Mock", func(t *testing.T) {
		actual, err := provider.New(provider.Config{
			APIType:      provider.APITypeMock,
			ProviderType: provider.ProviderTypeMock,
			Credentials:  auth.NewNone(),
		})
		require.NoError(t, err)
		require.IsType(t, &mock.MockProvider{}, actual)
	})

	t.Run("UnsupportedAPI", func(t *testing.T) {
		_, err := provider.New(provider.Config{
			APIType:           provider.APITypeMock,
			ProviderType:      provider.ProviderTypeOpenAI,
			InferenceEndpoint: "https://api.openai.com/v1",
			CatalogEndpoint:   "https://api.openai.com/v1/models",
			Credentials:       auth.NewAPIKey("secret"),
		})
		require.ErrorIs(t, err, errors.ErrUnsupportedAPIType)
	})
}

// Verifies provider registrations expose implementation-owned API and auth
// capabilities without allowing callers to mutate registry state.
func TestRegistrationMetadata(t *testing.T) {
	apiTypes := provider.ProviderTypeOpenAICompatible.SupportedAPITypes()
	require.Equal(t, []provider.APIType{
		provider.APITypeOpenAIChatCompletions,
		provider.APITypeOpenAIResponses,
	}, apiTypes)
	require.Equal(t, []auth.Type{
		auth.TypeAPIKey,
		auth.TypeOpenAIOrganization,
		auth.TypeNone,
	}, provider.ProviderTypeOpenAICompatible.SupportedAuthTypes())

	apiTypes[0] = provider.APITypeUnknown
	require.Equal(t, provider.APITypeOpenAIChatCompletions, provider.ProviderTypeOpenAICompatible.SupportedAPITypes()[0])
}
