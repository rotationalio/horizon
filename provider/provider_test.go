package provider_test

import (
	"context"
	"math/rand"

	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/internal/testenv"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth/credtest"
	"go.rtnl.ai/horizon/provider/config"
	"go.rtnl.ai/horizon/provider/types"
	"go.rtnl.ai/ulid"
)

// Ensures that [provider.New] caches by [config.Provider].
func TestNewProvider_CachesByConfig(t *testing.T) {
	t.Cleanup(func() { provider.ResetProviderCacheForTest(t) })

	conf := mockProviderConfig(ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV"))

	first, err := provider.New(conf)
	require.NoError(t, err)

	second, err := provider.New(conf)
	require.NoError(t, err)

	assert.Same(t, first, second)
}

// Ensures that [provider.New] returns different clients for different
// [config.Provider]s.
func TestNewProvider_DifferentConfigs(t *testing.T) {
	t.Cleanup(func() { provider.ResetProviderCacheForTest(t) })

	first, err := provider.New(mockProviderConfig(ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FA0")))
	require.NoError(t, err)

	second, err := provider.New(mockProviderConfig(ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FB1")))
	require.NoError(t, err)

	assert.NotSame(t, first, second)
}

// Ensures that [provider.New] returns the same client for the same
// [config.Provider] in concurrent calls.
func TestNewProvider_ConcurrentSameConfig(t *testing.T) {
	t.Cleanup(func() { provider.ResetProviderCacheForTest(t) })

	conf1 := mockProviderConfig(ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FA0"))
	conf2 := mockProviderConfig(ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FB1"))
	conf3 := mockProviderConfig(ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FC2"))
	conf4 := mockProviderConfig(ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FD3"))

	const numGoroutines = 100
	providers := make([]*provider.Provider, numGoroutines)
	errs := make([]error, numGoroutines)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	for i := range numGoroutines {
		var conf config.Provider
		switch i % 4 {
		case 0:
			conf = conf1
		case 1:
			conf = conf2
		case 2:
			conf = conf3
		case 3:
			conf = conf4
		}
		go func(i int) {
			defer wg.Done()
			time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
			providers[i], errs[i] = provider.New(conf)
		}(i)
	}
	wg.Wait()

	for i := range numGoroutines {
		require.NoError(t, errs[i])
		assert.Same(t, providers[i%4], providers[i])
	}
}

// Ensures that [provider.New] returns an error for an invalid
// [config.Provider].
func TestNewProvider_InvalidConfigNotCached(t *testing.T) {
	t.Cleanup(func() { provider.ResetProviderCacheForTest(t) })

	conf := mockProviderConfig(ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV"))
	conf.APIType = types.APITypeUnknown

	_, err := provider.New(conf)
	require.ErrorIs(t, err, errors.ErrUnsupportedAPIType)

	_, err = provider.New(conf)
	require.ErrorIs(t, err, errors.ErrUnsupportedAPIType)

	conf.APIType = types.APITypeMock
	p, err := provider.New(conf)
	require.NoError(t, err)
	require.NotNil(t, p)
}

// TestCheckConnectivity_OpenRouter runs a live catalog and inference connectivity
// check against OpenRouter with the required OpenRouter environment configuration.
func TestCheckConnectivity_OpenRouter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live OpenRouter connectivity test in short mode")
	}

	apiKey := testenv.OpenRouterAPIKey(t)
	endpoint := testenv.OpenRouterEndpointURL(t)

	t.Cleanup(func() { provider.ResetProviderCacheForTest(t) })

	conf := config.Provider{
		ID:                ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FE4"),
		APIType:           types.APITypeOpenAIChatCompletions,
		ProviderType:      types.ProviderTypeOpenRouter,
		InferenceEndpoint: endpoint,
		CatalogEndpoint:   "https://openrouter.ai/api/v1/models",
		Credentials:       credtest.APIKey(apiKey),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Connected

	client, err := provider.New(conf)
	require.NoError(t, err)

	status, err := client.CheckConnectivity(ctx)
	require.NoError(t, err)
	require.Equal(t, types.StatusConnected, status)

	// Unauthorized

	conf.Credentials = credtest.APIKey("invalid")
	client, err = provider.New(conf)
	require.NoError(t, err)

	status, err = client.CheckConnectivity(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "generate error: 401:")
	require.Equal(t, types.StatusAuthRequired, status)
}

//=============================================================================
// Helpers
//=============================================================================

// Returns a mock [config.Provider] with the given ID, seeding the ID into the
// inference and catalog endpoints and credentials.
func mockProviderConfig(id ulid.ULID) config.Provider {
	return config.Provider{
		ID:                id,
		APIType:           types.APITypeMock,
		InferenceEndpoint: "https://mock.example/inference/" + id.String(),
		CatalogEndpoint:   "https://mock.example/catalog/" + id.String(),
		ProviderType:      types.ProviderTypeMock,
		Credentials:       credtest.APIKey("secret" + id.String()),
	}
}
