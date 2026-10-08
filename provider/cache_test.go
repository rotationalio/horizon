package provider_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/mock"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/validation"
)

// Verifies that equivalent configuration reuses the cached provider instance.
func TestProviderCacheAddIsIdempotent(t *testing.T) {
	cache := provider.NewCache()
	config := mockProviderConfig(providerTestID)

	require.NoError(t, cache.Add(config))
	first, err := cache.Get(config.ID)
	require.NoError(t, err)
	require.Equal(t, config.ID, first.ID())

	require.NoError(t, cache.Add(config))
	second, err := cache.Get(config.ID)
	require.NoError(t, err)
	require.Same(t, first, second)

	require.NoError(t, cache.AddInstance(config, first))
	require.NoError(t, cache.Add(config))
	third, err := cache.Get(config.ID)
	require.NoError(t, err)
	require.Same(t, first, third)
}

// Verifies simultaneous additions of equivalent configuration share one instance.
func TestProviderCacheAddConcurrent(t *testing.T) {
	const additions = 16
	cache := provider.NewCache()
	start := make(chan struct{})
	results := make(chan struct {
		instance provider.Provider
		err      error
	}, additions)
	for range additions {
		go func() {
			<-start
			config := mockProviderConfig(providerTestID)
			instance, err := cache.GetOrCreate(config, nil)
			results <- struct {
				instance provider.Provider
				err      error
			}{
				instance: instance,
				err:      err,
			}
		}()
	}
	close(start)
	first := <-results
	require.NoError(t, first.err)
	for range additions - 1 {
		result := <-results
		require.NoError(t, result.err)
		require.Same(t, first.instance, result.instance)
	}
}

// Verifies config-based resolution retains supplied mocks and returns the matching
// instance directly, including after replacement or eviction.
func TestProviderCacheGetOrCreate(t *testing.T) {
	cache := provider.NewCache()
	config := mockProviderConfig(providerTestID)
	instance := mock.New(config.ID)
	instance.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		return &provider.Response{ID: "cached-mock"}, nil
	}
	require.NoError(t, cache.AddInstance(config, instance))
	actual, err := cache.GetOrCreate(config, nil)
	require.NoError(t, err)
	require.Same(t, instance, actual)
	response, err := actual.Generate(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, "cached-mock", response.ID)

	changed := config
	changed.Credentials = auth.NewAPIKey("rotated-secret")
	replacement, err := cache.GetOrCreate(changed, nil)
	require.NoError(t, err)
	require.NotSame(t, instance, replacement)
	again, err := cache.GetOrCreate(changed, nil)
	require.NoError(t, err)
	require.Same(t, replacement, again)

	// The previously acquired instance remains usable after a cache replacement.
	response, err = actual.Generate(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, "cached-mock", response.ID)
	require.True(t, cache.Remove(config.ID))
	recreated, err := cache.GetOrCreate(changed, nil)
	require.NoError(t, err)
	require.NotSame(t, replacement, recreated)

	invalid := changed
	invalid.Credentials = auth.NewAPIKey("")
	failed, err := cache.GetOrCreate(invalid, nil)
	require.Error(t, err)
	require.Nil(t, failed)
	actual, err = cache.Get(config.ID)
	require.NoError(t, err)
	require.Same(t, recreated, actual)
}

// Verifies that changing provider settings or credentials replaces the cached instance.
func TestProviderCacheAddReplacesChangedConfiguration(t *testing.T) {
	cache := provider.NewCache()
	config := mockProviderConfig(providerTestID)
	require.NoError(t, cache.Add(config))
	first, err := cache.Get(config.ID)
	require.NoError(t, err)

	config.DefaultModel = "changed-model"
	require.NoError(t, cache.Add(config))
	second, err := cache.Get(config.ID)
	require.NoError(t, err)
	require.NotSame(t, first, second)

	config.Credentials = auth.NewAPIKey("rotated-secret")
	require.NoError(t, cache.Add(config))
	third, err := cache.Get(config.ID)
	require.NoError(t, err)
	require.NotSame(t, second, third)
}

// Verifies explicit instances always replace existing entries and retain their config hash.
func TestProviderCacheAddProvider(t *testing.T) {
	cache := provider.NewCache()
	config := mockProviderConfig(providerTestID)
	first := mock.New(config.ID)
	require.NoError(t, cache.AddInstance(config, first))

	actual, err := cache.Get(config.ID)
	require.NoError(t, err)
	require.Same(t, first, actual)
	require.NoError(t, cache.Add(config))
	actual, err = cache.Get(config.ID)
	require.NoError(t, err)
	require.Same(t, first, actual)

	second := mock.New(config.ID)
	require.NoError(t, cache.AddInstance(config, second))
	actual, err = cache.Get(config.ID)
	require.NoError(t, err)
	require.Same(t, second, actual)
	require.NoError(t, cache.AddInstance(config, second))
	require.NoError(t, cache.Add(config))
	actual, err = cache.Get(config.ID)
	require.NoError(t, err)
	require.Same(t, second, actual)

	config.Credentials = auth.NewAPIKey("new-secret")
	require.NoError(t, cache.Add(config))
	actual, err = cache.Get(config.ID)
	require.NoError(t, err)
	require.NotSame(t, second, actual)
}

// Verifies invalid configurations, nil providers, and mismatched IDs cannot enter the cache.
func TestProviderCacheRejectsInvalidEntries(t *testing.T) {
	t.Run("invalid config", func(t *testing.T) {
		cache := provider.NewCache()
		config := mockProviderConfig(ulid.ULID{})
		validation.RequireValidation(t, cache.Add(config), validation.Missing("id"))
		validation.RequireValidation(t, cache.AddInstance(config, mock.New(providerTestID)), validation.Missing("id"))
	})

	t.Run("nil provider", func(t *testing.T) {
		cache := provider.NewCache()
		require.ErrorIs(t, cache.AddInstance(mockProviderConfig(providerTestID), nil), errors.ErrProviderRequired)
	})

	t.Run("mismatched provider ID", func(t *testing.T) {
		cache := provider.NewCache()
		config := mockProviderConfig(providerTestID)
		err := cache.AddInstance(config, mock.New(ulid.Make()))
		require.ErrorIs(t, err, errors.ErrProviderIDMismatch)
		_, err = cache.Get(config.ID)
		require.ErrorIs(t, err, errors.ErrProviderNotFound)
	})

	t.Run("zero provider ID", func(t *testing.T) {
		cache := provider.NewCache()
		err := cache.AddInstance(mockProviderConfig(providerTestID), mock.New(ulid.ULID{}))
		require.ErrorIs(t, err, errors.ErrProviderIDMismatch)
	})

	t.Run("failed update preserves existing instance", func(t *testing.T) {
		cache := provider.NewCache()
		config := mockProviderConfig(providerTestID)
		instance := mock.New(config.ID)
		require.NoError(t, cache.AddInstance(config, instance))
		config.Credentials = auth.NewAPIKey("")
		require.Error(t, cache.AddInstance(config, mock.New(config.ID)))
		actual, err := cache.Get(config.ID)
		require.NoError(t, err)
		require.Same(t, instance, actual)
	})

	t.Run("missing entry", func(t *testing.T) {
		cache := provider.NewCache()
		_, err := cache.Get(providerTestID)
		require.ErrorIs(t, err, errors.ErrProviderNotFound)
		_, err = cache.Get(ulid.ULID{})
		require.ErrorIs(t, err, errors.ErrProviderIDRequired)
	})

}

// Verifies removing entries leaves acquired instances usable and allows later re-creation.
func TestProviderCacheRemove(t *testing.T) {
	cache := provider.NewCache()
	config := mockProviderConfig(providerTestID)
	instance := mock.New(config.ID)
	instance.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		return &provider.Response{ID: "still-usable"}, nil
	}
	require.False(t, cache.Remove(config.ID))
	require.False(t, cache.Remove(ulid.ULID{}))
	require.NoError(t, cache.AddInstance(config, instance))
	acquired, err := cache.Get(config.ID)
	require.NoError(t, err)
	require.True(t, cache.Remove(config.ID))
	require.False(t, cache.Remove(config.ID))
	_, err = cache.Get(config.ID)
	require.ErrorIs(t, err, errors.ErrProviderNotFound)
	response, err := acquired.Generate(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, "still-usable", response.ID)
	require.NoError(t, cache.Add(config))
	replacement, err := cache.Get(config.ID)
	require.NoError(t, err)
	require.NotSame(t, instance, replacement)

}

func mockProviderConfig(id ulid.ULID) provider.Config {
	return provider.Config{
		ID:           id,
		APIType:      provider.APITypeMock,
		ProviderType: provider.ProviderTypeMock,
		Credentials:  auth.NewNone(),
	}
}
