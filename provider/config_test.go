package provider_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
)

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

	t.Run("requires valid default model", func(t *testing.T) {
		conf := validProvider()
		conf.ProviderType = provider.ProviderTypeOpenAICompatible
		conf.DefaultModel = ""
		require.ErrorIs(t, conf.Validate(), errors.ErrInvalidDefaultModel)
	})
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
