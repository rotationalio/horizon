package provider

import (
	"crypto/sha256"
	"fmt"
	stdhttp "net/http"
	"sync"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/ulid"
)

// Cache stores configured provider instances for an application.
// Construct caches with [NewCache].
type Cache struct {
	mu         sync.RWMutex
	providers  map[ulid.ULID]cachedProvider
	httpClient *stdhttp.Client
}

type cachedProvider struct {
	instance    Provider
	fingerprint [sha256.Size]byte
}

// NewCache creates an empty provider cache that uses provider implementations' default HTTP clients.
func NewCache() *Cache {
	return NewCacheWithHTTPClient(nil)
}

// NewCacheWithHTTPClient creates an empty provider cache that passes client to
// constructed providers. A nil client lets each provider use its default client.
func NewCacheWithHTTPClient(client *stdhttp.Client) *Cache {
	return &Cache{
		providers:  make(map[ulid.ULID]cachedProvider),
		httpClient: client,
	}
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
		if instance, err = NewWithHTTPClient(config, c.httpClient); err != nil {
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
