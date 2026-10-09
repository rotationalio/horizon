package provider

import (
	"crypto/sha256"
	"fmt"

	lru "github.com/hashicorp/golang-lru/v2"
	"go.rtnl.ai/horizon/config"
	"go.rtnl.ai/horizon/errors"

	"go.rtnl.ai/ulid"
	"golang.org/x/sync/singleflight"
)

// Cache stores configured provider instances for an application.
// Construct caches with [NewCache].
type Cache struct {
	providers *lru.Cache[ulid.ULID, cachedProvider]
	options   Options
	creating  singleflight.Group
}

type cachedProvider struct {
	instance    Provider
	fingerprint [sha256.Size]byte
}

// NewCache creates an empty provider cache using the configured cache capacity
// and provider options.
func NewCache(options ...Option) (*Cache, error) {
	conf, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("load provider cache configuration: %w", err)
	}

	providers, err := lru.New[ulid.ULID, cachedProvider](conf.ProviderCacheSize)
	if err != nil {
		return nil, fmt.Errorf("create provider cache: %w", err)
	}
	return &Cache{providers: providers, options: ResolveOptions(options...)}, nil
}

// GetOrCreate returns the instance matching config, constructing and caching it
// if instance is nil. Changed settings or credentials replace the cached instance.
// A supplied instance always replaces the entry and must have the same config ID.
func (c *Cache) GetOrCreate(config Config, instance Provider) (Provider, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	construct := instance == nil
	if !construct && instance.ID() != config.ID {
		return nil, errors.ErrProviderIDMismatch
	}

	fingerprint, err := config.Hash()
	if err != nil {
		return nil, fmt.Errorf("hash provider configuration: %w", err)
	}

	if !construct {
		c.providers.Add(config.ID, cachedProvider{
			instance:    instance,
			fingerprint: fingerprint,
		})
		return instance, nil
	}

	// Return a cached instance without entering singleflight coordination.
	if cached, ok := c.providers.Get(config.ID); ok && cached.fingerprint == fingerprint {
		return cached.instance, nil
	}

	// Otherwise, we need to construct a new instance and cache it. The
	// singleflight.Group ensures that only one instance is constructed at a
	// time and all callers receive the same instance.
	key := config.ID.String() + string(fingerprint[:])
	value, err, _ := c.creating.Do(key, func() (any, error) {
		// Check again in case we lost a race before the Do was called, though
		// unlikely.
		if cached, ok := c.providers.Get(config.ID); ok && cached.fingerprint == fingerprint {
			return cached.instance, nil
		}

		instance, err := New(config, WithHTTPClient(c.options.HTTPClient))
		if err != nil {
			return nil, err
		}
		c.providers.Add(config.ID, cachedProvider{
			instance:    instance,
			fingerprint: fingerprint,
		})
		return instance, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(Provider), nil
}

// Get returns the provider cached under id.
func (c *Cache) Get(id ulid.ULID) (Provider, error) {
	if id.IsZero() {
		return nil, errors.ErrProviderIDRequired
	}

	cached, ok := c.providers.Get(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errors.ErrProviderNotFound, id)
	}
	return cached.instance, nil
}

// Remove deletes a cached provider and reports whether it existed. Previously
// retrieved instances remain usable; removal does not close the provider.
func (c *Cache) Remove(id ulid.ULID) bool {
	return c.providers.Remove(id)
}
