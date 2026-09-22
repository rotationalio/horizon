package provider_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth/credtest"
	"go.rtnl.ai/horizon/provider/config"
	"go.rtnl.ai/horizon/provider/types"
	"go.rtnl.ai/ulid"
)

// TestNewProvider_CachesByCredentialValue verifies that clients with equivalent
// credentials but different pointers share the same cached provider provider.
func TestNewProvider_CachesByCredentialValue(t *testing.T) {
	t.Cleanup(func() { provider.ResetProviderCacheForTest(t) })

	id := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV")
	base := config.Provider{
		ID:                id,
		APIType:           types.APITypeMock,
		InferenceEndpoint: "https://mock.example/inference",
		CatalogEndpoint:   "https://mock.example/catalog",
		ProviderType:      types.ProviderTypeMock,
		Credentials:       credtest.APIKey("secret"),
	}

	first, err := provider.New(base)
	require.NoError(t, err)

	secondConf := base
	secondConf.Credentials = credtest.APIKey("secret")
	second, err := provider.New(secondConf)
	require.NoError(t, err)

	require.Same(t, first, second)
}

// TestNewProvider_DifferentCredentialValues verifies that different credential
// values produce distinct cached provider clients.
func TestNewProvider_DifferentCredentialValues(t *testing.T) {
	t.Cleanup(func() { provider.ResetProviderCacheForTest(t) })

	id := ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV")
	conf := func(apiKey string) config.Provider {
		return config.Provider{
			ID:                id,
			APIType:           types.APITypeMock,
			InferenceEndpoint: "https://mock.example/inference",
			CatalogEndpoint:   "https://mock.example/catalog",
			ProviderType:      types.ProviderTypeMock,
			Credentials:       credtest.APIKey(apiKey),
		}
	}

	first, err := provider.New(conf("one"))
	require.NoError(t, err)

	second, err := provider.New(conf("two"))
	require.NoError(t, err)

	require.NotSame(t, first, second)
}
