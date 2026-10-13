package horizon

/* IMPORTANT: Importing the root Horizon package registers all built-in provider
implementations automatically. NewProvider is aliased below instead of in
alias.go in order to import the provider implementations to ensure they are
registered for provider.New, otherwise calling it will cause a panic. */

import (
	"go.rtnl.ai/horizon/provider"

	// ! Import provider implementations to register them for [provider.New].
	_ "go.rtnl.ai/horizon/provider/mock"
	_ "go.rtnl.ai/horizon/provider/openai"
	_ "go.rtnl.ai/horizon/provider/openrouter"
)

// Validates config and constructs the corresponding built-in provider.
func NewProvider(config ProviderConfig) (Provider, error) {
	return provider.New(config)
}
