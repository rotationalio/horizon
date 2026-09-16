package render

import (
	"sync"

	"github.com/cespare/xxhash/v2"
	"go.rtnl.ai/x/cache"
)

// A factory function for creating a [Renderer] from a renderer [Type] and a
// template string.
type RendererFactory func(Type, string) (Renderer, error)

// An LRU to manage templated rendering in order to avoid repeated parsing of the same
// template. Caches are well used particularly in an experiment context where the
// template is used multiple times with different context data.
type Cache struct {
	mu      sync.RWMutex
	factory RendererFactory
	cache   *cache.LRU[uint64, Renderer]
}

// Creates a new [Cache] with the given size and factory function.
func NewCache(size int, factory RendererFactory) (c *Cache, err error) {
	// If factory is nil, use the default factory.
	if factory == nil {
		factory = func(t Type, template string) (Renderer, error) {
			return New(t, template)
		}
	}

	c = &Cache{
		factory: factory,
	}

	if c.cache, err = cache.NewLRU[uint64, Renderer](size, nil); err != nil {
		return nil, err
	}
	return c, nil
}

// Renders a template with the given data using the renderer type and template
// string.
func (c *Cache) Render(t Type, template string, data map[string]any) (out string, err error) {
	// Create a hash of the template data for use as a fast key.
	var renderer Renderer
	key := xxhash.Sum64String(template)

	// Attempt a read lock to see if the renderer is already in the cache.
	c.mu.RLock()
	var ok bool
	if renderer, ok = c.cache.Get(key); ok {
		// Keep the read lock until after the renderer is utilized.
		defer c.mu.RUnlock()
		return renderer.Render(data)
	}

	// Release the read lock to proceed with double-checked locking.
	c.mu.RUnlock()

	// The renderer is not in the cache, so we need to create and add it to the cache.
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double check that the renderer is not in the cache.
	if renderer, ok = c.cache.Get(key); ok {
		return renderer.Render(data)
	}

	// The renderer is still not in the cache, parse the template.
	if renderer, err = c.factory(t, template); err != nil {
		return "", err
	}

	// Add the renderer to the cache and return the rendered output.
	c.cache.Put(key, renderer)
	return renderer.Render(data)
}
