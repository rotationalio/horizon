package provider

import (
	"testing"

	"go.rtnl.ai/x/cache"
)

// Clears the provider cache between tests.
func ResetProviderCacheForTest(t *testing.T) {
	t.Helper()
	providerCache = &cache.SafeMap[providerCacheKey, *Provider]{}
}
