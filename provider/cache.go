package provider

import (
	"crypto/sha256"
	"fmt"
	"sync"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/ulid"
)

// Cache stores configured provider instances for an application.
// Construct caches with [NewCache].
type Cache struct {
	mu        sync.RWMutex
	providers map[ulid.ULID]cachedProvider
}

type cachedProvider struct {
	instance    Provider
	fingerprint [sha256.Size]byte
}

// NewCache creates an empty provider cache.
func NewCache() *Cache {
	return &Cache{providers: make(map[ulid.ULID]cachedProvider)}
}

// Add constructs and caches the provider configured by config. Adding the same
// ID and configuration reuses the existing instance. A changed configuration
// replaces it for future lookups.
func (c *Cache) Add(config Config) error {
	_, err := c.GetOrCreate(config, nil)
	return err
}

// AddInstance caches an existing provider with its configuration, replacing any
// instance under config.ID. IDs must match; the caller is responsible for ensuring
// the configuration's settings and credentials describe the supplied instance.
// Typed-nil providers must not be supplied.
func (c *Cache) AddInstance(config Config, instance Provider) error {
	if instance == nil {
		return errors.ErrProviderRequired
	}
	_, err := c.GetOrCreate(config, instance)
	return err
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

	if construct {
		c.mu.RLock()
		cached, ok := c.providers[config.ID]
		c.mu.RUnlock()
		if ok && cached.fingerprint == fingerprint {
			// Cached instance matches, no need to replace.
			return cached.instance, nil
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if construct {
		if cached, ok := c.providers[config.ID]; ok && cached.fingerprint == fingerprint {
			// Double-checked locking in case we lost a race; cached instance matches, no need to replace.
			return cached.instance, nil
		}
		if instance, err = New(config); err != nil {
			return nil, err
		}
	}

	c.providers[config.ID] = cachedProvider{
		instance:    instance,
		fingerprint: fingerprint,
	}
	return instance, nil
}

// Get returns the provider cached under id.
func (c *Cache) Get(id ulid.ULID) (Provider, error) {
	if id.IsZero() {
		return nil, errors.ErrProviderIDRequired
	}

	c.mu.RLock()
	cached, ok := c.providers[id]
	c.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", errors.ErrProviderNotFound, id)
	}
	return cached.instance, nil
}

// Remove deletes a cached provider and reports whether it existed. Previously
// retrieved instances remain usable; removal does not close the provider.
func (c *Cache) Remove(id ulid.ULID) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, exists := c.providers[id]
	delete(c.providers, id)
	return exists
}
