package provider_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
)

// Verifies provider configuration validation accepts supported combinations and
// rejects invalid endpoints, credentials, models, and provider capabilities.
func TestProviderValidate(t *testing.T) {
	t.Run("accepts valid config", func(t *testing.T) {
		require.NoError(t, validProvider().Validate())
	})

	t.Run("requires supported API type", func(t *testing.T) {
		conf := validProvider()
		conf.APIType = provider.APITypeUnknown
		require.ErrorIs(t, conf.Validate(), errors.ErrUnsupportedAPIType)
	})

	t.Run("requires valid inference endpoint", func(t *testing.T) {
		conf := validProvider()
		conf.InferenceEndpoint = "://bad"
		require.ErrorIs(t, conf.Validate(), errors.ErrInvalidInferenceEndpoint)
	})

	t.Run("requires absolute inference endpoint", func(t *testing.T) {
		conf := validProvider()
		conf.InferenceEndpoint = "/v1"
		require.ErrorIs(t, conf.Validate(), errors.ErrInvalidInferenceEndpoint)
	})

	t.Run("requires supported provider type", func(t *testing.T) {
		conf := validProvider()
		conf.ProviderType = provider.ProviderTypeUnknown
		require.ErrorIs(t, conf.Validate(), errors.ErrUnsupportedProviderType)
	})

	t.Run("requires valid catalog endpoint", func(t *testing.T) {
		conf := validProvider()
		conf.CatalogEndpoint = "://bad"
		require.ErrorIs(t, conf.Validate(), errors.ErrInvalidCatalogEndpoint)
	})

	t.Run("requires absolute catalog endpoint", func(t *testing.T) {
		conf := validProvider()
		conf.CatalogEndpoint = "/models"
		require.ErrorIs(t, conf.Validate(), errors.ErrInvalidCatalogEndpoint)
	})

	t.Run("requires provider API support", func(t *testing.T) {
		conf := validProvider()
		conf.APIType = provider.APITypeMock
		require.ErrorIs(t, conf.Validate(), errors.ErrUnsupportedAPIType)
	})

	t.Run("requires credentials", func(t *testing.T) {
		conf := validProvider()
		conf.Credentials = nil
		require.ErrorIs(t, conf.Validate(), errors.ErrInvalidCredentials)
	})

	t.Run("requires valid credentials", func(t *testing.T) {
		conf := validProvider()
		conf.Credentials = auth.NewAPIKey("")
		require.ErrorIs(t, conf.Validate(), errors.ErrInvalidCredentials)
	})

	t.Run("requires provider credential support", func(t *testing.T) {
		conf := validProvider()
		conf.Credentials = auth.NewBasic("username", "password")
		require.ErrorIs(t, conf.Validate(), errors.ErrInvalidCredentials)
	})

	t.Run("requires valid default model", func(t *testing.T) {
		conf := validProvider()
		conf.ProviderType = provider.ProviderTypeOpenAICompatible
		conf.DefaultModel = ""
		require.ErrorIs(t, conf.Validate(), errors.ErrInvalidDefaultModel)
	})

	t.Run("mock requires no endpoints", func(t *testing.T) {
		conf := &provider.Config{
			APIType:      provider.APITypeMock,
			ProviderType: provider.ProviderTypeMock,
			Credentials:  auth.NewNone(),
		}
		require.NoError(t, conf.Validate())
	})

	t.Run("OpenAI compatible supports no authentication", func(t *testing.T) {
		conf := validProvider()
		conf.ProviderType = provider.ProviderTypeOpenAICompatible
		conf.Credentials = auth.NewNone()
		require.NoError(t, conf.Validate())
	})
}

// Verifies provider configuration equality is nil-safe, ignores credential
// secrets, and rejects semantically different configurations.
func TestProviderEquals(t *testing.T) {
	var nilConfig *provider.Config
	require.True(t, nilConfig.Equals(nil))
	require.False(t, nilConfig.Equals(validProvider()))
	require.False(t, validProvider().Equals(nil))

	left := validProvider()
	right := validProvider()
	right.Credentials = auth.NewAPIKey("different-secret")
	require.True(t, left.Equals(right), "credential values are intentionally excluded")

	right.APIType = provider.APITypeMock
	require.False(t, left.Equals(right))
}

func validProvider() *provider.Config {
	return &provider.Config{
		InferenceEndpoint: "https://openrouter.ai/api/v1",
		CatalogEndpoint:   "https://openrouter.ai/api/v1/models",
		ProviderType:      provider.ProviderTypeOpenRouter,
		APIType:           provider.APITypeOpenAIChatCompletions,
		DefaultModel:      "default/model",
		Credentials:       auth.NewAPIKey("secret"),
	}
}
