package provider

import (
	"context"

	"go.rtnl.ai/horizon/errors"

	"go.rtnl.ai/horizon/provider/api"
	"go.rtnl.ai/horizon/provider/catalog"
	"go.rtnl.ai/horizon/provider/catalog/governance"
	"go.rtnl.ai/horizon/provider/config"
	"go.rtnl.ai/horizon/provider/mock"
	"go.rtnl.ai/horizon/provider/openai"
	"go.rtnl.ai/horizon/provider/openrouter"
	"go.rtnl.ai/horizon/provider/types"
	"go.rtnl.ai/x/cache"
)

//===============================================
// Interfaces
//===============================================

// Inference is the inference surface for a provider client.
type Inference interface {
	Generate(ctx context.Context, req *api.Request) (*api.Response, error)
}

// Catalog is the catalog surface for a provider client.
type Catalog interface {
	// Fetch returns a list of models from the provider catalog, restricted by
	// the included governance policies.
	Fetch(ctx context.Context, policies ...governance.Policy) ([]catalog.Model, error)

	// Retrieve returns a model from the provider catalog by model ID (ID may
	// differ from provider to provider), restricted by the included governance
	// policies.
	Retrieve(ctx context.Context, modelID string, policies ...governance.Policy) (*catalog.Model, error)
}

//===============================================
// Provider
//===============================================

// Provider is a Horizon provider client composed of inference and catalog backends.
type Provider struct {
	conf      config.Provider
	inference Inference
	catalog   Catalog
}

func (p *Provider) Config() config.Provider {
	return p.conf
}

func (p *Provider) Inference() Inference {
	return p.inference
}

func (p *Provider) Catalog() Catalog {
	return p.catalog
}

//===============================================
// Constructor
//===============================================

// A cache of [Provider] clients by stable provider configuration identity.
var providerCache = &cache.SafeMap[providerCacheKey, *Provider]{}

// Returns a [Provider] client for the given configuration. Clients are cached
// by provider ID, endpoints, and credential values to avoid creating multiple
// clients for the same configuration, unless [config.Provider.NoCache] is true.
func New(conf config.Provider) (*Provider, error) {
	if conf.NoCache {
		return new(conf)
	}

	key, err := providerCacheKeyFrom(conf)
	if err != nil {
		return nil, err
	}

	return providerCache.GetOrCreate(key, func(providerCacheKey) (*Provider, error) {
		return new(conf)
	})
}

// Returns a new [Provider] client for the given configuration, without caching.
func new(conf config.Provider) (p *Provider, err error) {
	if err = conf.Validate(); err != nil {
		return nil, err
	}

	p = &Provider{conf: conf}

	if p.inference, err = newInference(conf); err != nil {
		return nil, err
	}

	if p.catalog, err = newCatalog(conf); err != nil {
		return nil, err
	}

	return p, nil
}

//=============================================================================
// API Methods
//=============================================================================

// Generate executes an inference request.
func (p *Provider) Generate(ctx context.Context, req *api.Request) (*api.Response, error) {
	if p == nil || p.inference == nil {
		return nil, errors.ErrInferenceRequired
	}
	rep, err := p.inference.Generate(ctx, req)
	if err != nil {
		err = newProviderError(err)
	}
	return rep, err
}

// FetchCatalog returns models from the provider catalog.
func (p *Provider) FetchCatalog(ctx context.Context, policies ...governance.Policy) ([]catalog.Model, error) {
	if p == nil || p.catalog == nil {
		return nil, errors.ErrCatalogRequired
	}
	models, err := p.catalog.Fetch(ctx, policies...)
	if err != nil {
		err = newProviderError(err)
	}
	return models, err
}

// RetrieveModel retrieves a model from the provider catalog.
func (p *Provider) RetrieveModel(ctx context.Context, modelID string, policies ...governance.Policy) (*catalog.Model, error) {
	if p == nil || p.catalog == nil {
		return nil, errors.ErrCatalogRequired
	}
	model, err := p.catalog.Retrieve(ctx, modelID, policies...)
	if err != nil {
		err = newProviderError(err)
	}
	return model, err
}

//===============================================
// Factories
//===============================================

// Creates a new inference client for the given configuration.
func newInference(conf config.Provider) (Inference, error) {
	switch conf.APIType {
	case types.APITypeMock:
		return mock.NewInference(), nil
	case types.APITypeOpenAIResponses:
		return openai.NewResponses(conf)
	case types.APITypeOpenAIChatCompletions:
		return openai.NewChatCompletions(conf)
	default:
		return nil, errors.ErrUnsupportedAPIType
	}
}

// Creates a new catalog client for the given configuration.
func newCatalog(conf config.Provider) (Catalog, error) {
	switch conf.ProviderType {
	case types.ProviderTypeMock:
		return mock.NewCatalog(), nil
	case types.ProviderTypeOpenRouter:
		return openrouter.NewCatalog(conf)
	case types.ProviderTypeOpenAI, types.ProviderTypeOpenAICompatible:
		return openai.NewCatalog(conf)
	default:
		return nil, errors.ErrUnsupportedProviderType
	}
}
