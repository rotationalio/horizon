package config_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider/auth/credtest"
	"go.rtnl.ai/horizon/provider/config"
	"go.rtnl.ai/horizon/provider/types"
	"go.rtnl.ai/ulid"
)

// Verifies the provider config validates properly.
func TestProviderValidate(t *testing.T) {
	t.Run("accepts valid config", func(t *testing.T) {
		require.NoError(t, validProvider().Validate())
	})

	t.Run("requires ID", func(t *testing.T) {
		conf := validProvider()
		conf.ID = ulid.Zero
		require.ErrorIs(t, conf.Validate(), errors.ErrInvalidID)
	})

	t.Run("requires supported API type", func(t *testing.T) {
		conf := validProvider()
		conf.APIType = types.APITypeUnknown
		require.ErrorIs(t, conf.Validate(), errors.ErrUnsupportedAPIType)
	})

	t.Run("requires valid inference endpoint", func(t *testing.T) {
		conf := validProvider()
		conf.InferenceEndpoint = "://bad"
		require.ErrorIs(t, conf.Validate(), errors.ErrInvalidInferenceEndpoint)
	})

	t.Run("requires supported provider type", func(t *testing.T) {
		conf := validProvider()
		conf.ProviderType = types.ProviderTypeUnknown
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
		conf.Credentials = credtest.APIKey("")
		require.ErrorIs(t, conf.Validate(), errors.ErrInvalidCredentials)
	})

	t.Run("requires valid default model", func(t *testing.T) {
		conf := validProvider()
		conf.ProviderType = types.ProviderTypeOpenAICompatible
		conf.DefaultModel = ""
		require.ErrorIs(t, conf.Validate(), errors.ErrInvalidDefaultModel)
	})
}

func validProvider() *config.Provider {
	return &config.Provider{
		ID:                ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV"),
		InferenceEndpoint: "https://openrouter.ai/api/v1",
		CatalogEndpoint:   "https://openrouter.ai/api/v1/models",
		ProviderType:      types.ProviderTypeOpenRouter,
		APIType:           types.APITypeOpenAIChatCompletions,
		DefaultModel:      "default/model",
		Credentials:       credtest.APIKey("secret"),
	}
}
