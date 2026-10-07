package provider

import (
	"fmt"
	stdhttp "net/http"
	"slices"
	"sync"

	"go.rtnl.ai/horizon/provider/auth"
)

// Factory constructs a provider from validated configuration and an optional HTTP client.
type Factory func(Config, *stdhttp.Client) (Provider, error)

// Describes a provider implementation and the configurations it supports.
type Registration struct {
	Factory   Factory     // Constructor function for the provider
	APITypes  []APIType   // [APIType]s supported by the provider
	AuthTypes []auth.Type // [auth.Type]s supported by the provider
}

var registry = struct {
	sync.RWMutex
	providers map[ProviderType]Registration
}{
	providers: make(map[ProviderType]Registration),
}

// Associates a provider type with its constructor and supported configuration.
// Provider implementation packages register from init; duplicate registrations
// and invalid arguments panic because they indicate a programming error.
func Register(providerType ProviderType, registration Registration) {
	if providerType == ProviderTypeUnknown {
		panic("cannot register unknown provider type")
	}
	if registration.Factory == nil {
		panic("cannot register nil provider factory")
	}
	if len(registration.APITypes) == 0 {
		panic("cannot register provider without API types")
	}
	if len(registration.AuthTypes) == 0 {
		panic("cannot register provider without auth types")
	}

	registration.APITypes = slices.Clone(registration.APITypes)
	registration.AuthTypes = slices.Clone(registration.AuthTypes)

	registry.Lock()
	defer registry.Unlock()
	if _, exists := registry.providers[providerType]; exists {
		panic(fmt.Sprintf("provider type %s is already registered", providerType))
	}
	registry.providers[providerType] = registration
}

// New validates config and constructs its registered provider using the default HTTP client.
//
// NOTE: see the documentation on the provider package for more details on how
// to import provider implementation packages if you use this function
// directly, otherwise the use of [horizon.NewProvider] is preferred.
func New(config Config) (Provider, error) {
	return NewWithHTTPClient(config, nil)
}

// NewWithHTTPClient validates config and constructs its registered provider using client.
// A nil client lets the provider implementation use its default client.
func NewWithHTTPClient(config Config, client *stdhttp.Client) (Provider, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	registration, _ := LookupRegistration(config.ProviderType)
	return registration.Factory(config, client)
}

// Returns the [Registration] for the given provider type, if it is registered.
func LookupRegistration(providerType ProviderType) (Registration, bool) {
	registry.RLock()
	registration, exists := registry.providers[providerType]
	registry.RUnlock()
	if !exists {
		return Registration{}, false
	}

	registration.APITypes = slices.Clone(registration.APITypes)
	registration.AuthTypes = slices.Clone(registration.AuthTypes)
	return registration, true
}
