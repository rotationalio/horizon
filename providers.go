package horizon

import (
	"go.rtnl.ai/horizon/provider"

	// Import provider implementations to register them for [provider.New].
	_ "go.rtnl.ai/horizon/provider/mock"
	_ "go.rtnl.ai/horizon/provider/openai"
	_ "go.rtnl.ai/horizon/provider/openrouter"
)

// Performs inference and exposes a model catalog.
type Provider = provider.Provider

// Validates config and constructs the corresponding built-in provider.
// Importing the root Horizon package registers all built-in provider
// implementations automatically.
func NewProvider(config ProviderConfig) (Provider, error) {
	return provider.New(config)
}
