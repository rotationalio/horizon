package provider_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/config"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/mock"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/validation"
)

// Verifies unchanged configuration is reused and changed configuration replaces
// the cached provider.
func TestProviderCacheGetOrCreate(t *testing.T) {
	cache := newProviderCache(t)
	config := mockProviderConfig(providerTestID)

	first, err := cache.GetOrCreate(config, nil)
	require.NoError(t, err)
	require.Equal(t, config.ID, first.ID())

	reused, err := cache.GetOrCreate(config, nil)
	require.NoError(t, err)
	require.Same(t, first, reused)

	changedSettings := config
	changedSettings.DefaultModel = "changed-model"
	second, err := cache.GetOrCreate(changedSettings, nil)
	require.NoError(t, err)
	require.NotSame(t, first, second)

	changedCredentials := changedSettings
	changedCredentials.Credentials = auth.NewAPIKey("rotated-secret")
	third, err := cache.GetOrCreate(changedCredentials, nil)
	require.NoError(t, err)
	require.NotSame(t, second, third)

	reused, err = cache.GetOrCreate(changedCredentials, nil)
	require.NoError(t, err)
	require.Same(t, third, reused)
}

// Verifies supplied instances are cached, reused for nil-instance lookups, and
// unconditionally replace a different cached instance under the same ID.
func TestProviderCacheGetOrCreateWithInstance(t *testing.T) {
	cache := newProviderCache(t)
	config := mockProviderConfig(providerTestID)
	first := mock.New(config.ID)

	actual, err := cache.GetOrCreate(config, first)
	require.NoError(t, err)
	require.Same(t, first, actual)

	actual, err = cache.GetOrCreate(config, nil)
	require.NoError(t, err)
	require.Same(t, first, actual)

	second := mock.New(config.ID)
	actual, err = cache.GetOrCreate(config, second)
	require.NoError(t, err)
	require.Same(t, second, actual)

	actual, err = cache.GetOrCreate(config, nil)
	require.NoError(t, err)
	require.Same(t, second, actual)
}

// Verifies simultaneous lookups for equivalent configuration share one instance.
func TestProviderCacheGetOrCreateConcurrent(t *testing.T) {
	const lookups = 16
	cache := newProviderCache(t)
	start := make(chan struct{})
	results := make(chan struct {
		instance provider.Provider
		err      error
	}, lookups)

	for range lookups {
		go func() {
			<-start
			instance, err := cache.GetOrCreate(mockProviderConfig(providerTestID), nil)
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
	for range lookups - 1 {
		result := <-results
		require.NoError(t, result.err)
		require.Same(t, first.instance, result.instance)
	}
}

// Verifies invalid configurations and instances are rejected without changing
// the current cached provider.
func TestProviderCacheRejectsInvalidEntries(t *testing.T) {
	t.Run("invalid config is validated before the supplied instance", func(t *testing.T) {
		cache := newProviderCache(t)
		config := mockProviderConfig(ulid.ULID{})

		_, err := cache.GetOrCreate(config, nil)
		validation.RequireValidation(t, err, validation.Missing("id"))
		_, err = cache.GetOrCreate(config, mock.New(providerTestID))
		validation.RequireValidation(t, err, validation.Missing("id"))
	})

	t.Run("provider ID must match config ID", func(t *testing.T) {
		cache := newProviderCache(t)
		config := mockProviderConfig(providerTestID)

		_, err := cache.GetOrCreate(config, mock.New(ulid.Make()))
		require.ErrorIs(t, err, errors.ErrProviderIDMismatch)
		_, err = cache.Get(config.ID)
		require.ErrorIs(t, err, errors.ErrProviderNotFound)

		_, err = cache.GetOrCreate(config, mock.New(ulid.ULID{}))
		require.ErrorIs(t, err, errors.ErrProviderIDMismatch)
	})

	t.Run("failed update preserves existing provider", func(t *testing.T) {
		cache := newProviderCache(t)
		config := mockProviderConfig(providerTestID)
		instance := mock.New(config.ID)
		_, err := cache.GetOrCreate(config, instance)
		require.NoError(t, err)

		invalid := config
		invalid.Credentials = auth.NewAPIKey("")
		failed, err := cache.GetOrCreate(invalid, mock.New(config.ID))
		require.Error(t, err)
		require.Nil(t, failed)

		actual, err := cache.Get(config.ID)
		require.NoError(t, err)
		require.Same(t, instance, actual)
	})

	t.Run("zero lookup ID is rejected", func(t *testing.T) {
		_, err := newProviderCache(t).Get(ulid.ULID{})
		require.ErrorIs(t, err, errors.ErrProviderIDRequired)
	})
}

// Verifies removing an entry leaves acquired instances usable and allows later
// re-creation.
func TestProviderCacheEvictsLeastRecentlyUsed(t *testing.T) {
	conf, err := config.Get()
	require.NoError(t, err)
	conf.ProviderCacheSize = 2
	require.NoError(t, config.Set(conf))
	t.Cleanup(config.Reset)
	cache := newProviderCache(t)

	firstConfig := mockProviderConfig(ulid.Make())
	secondConfig := mockProviderConfig(ulid.Make())
	thirdConfig := mockProviderConfig(ulid.Make())
	first, err := cache.GetOrCreate(firstConfig, nil)
	require.NoError(t, err)
	_, err = cache.GetOrCreate(secondConfig, nil)
	require.NoError(t, err)

	// Refresh the first entry so inserting the third evicts the second.
	got, err := cache.Get(firstConfig.ID)
	require.NoError(t, err)
	require.Same(t, first, got)
	_, err = cache.GetOrCreate(thirdConfig, nil)
	require.NoError(t, err)

	_, err = cache.Get(firstConfig.ID)
	require.NoError(t, err)
	_, err = cache.Get(secondConfig.ID)
	require.ErrorIs(t, err, errors.ErrProviderNotFound)
	_, err = cache.Get(thirdConfig.ID)
	require.NoError(t, err)
}

func TestProviderCacheRemove(t *testing.T) {
	cache := newProviderCache(t)
	config := mockProviderConfig(providerTestID)
	instance := mock.New(config.ID)
	instance.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		return &provider.Response{ID: "still-usable"}, nil
	}

	require.False(t, cache.Remove(config.ID))
	require.False(t, cache.Remove(ulid.ULID{}))
	acquired, err := cache.GetOrCreate(config, instance)
	require.NoError(t, err)
	require.True(t, cache.Remove(config.ID))
	require.False(t, cache.Remove(config.ID))

	_, err = cache.Get(config.ID)
	require.ErrorIs(t, err, errors.ErrProviderNotFound)
	response, err := acquired.Generate(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, "still-usable", response.ID)

	recreated, err := cache.GetOrCreate(config, nil)
	require.NoError(t, err)
	require.NotSame(t, instance, recreated)
}

func newProviderCache(t *testing.T) *provider.Cache {
	t.Helper()
	cache, err := provider.NewCache()
	require.NoError(t, err)
	return cache
}

func mockProviderConfig(id ulid.ULID) provider.Config {
	return provider.Config{
		ID:           id,
		APIType:      provider.APITypeMock,
		ProviderType: provider.ProviderTypeMock,
		Credentials:  auth.NewNone(),
	}
}
