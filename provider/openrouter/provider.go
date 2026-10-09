package openrouter

import (
	"go.rtnl.ai/horizon/errors"

	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/openai"
	"go.rtnl.ai/ulid"
)

// Implements the Horizon provider interface using OpenRouter's catalog and
// OpenAI-compatible inference APIs.
type Provider struct {
	id ulid.ULID
	provider.Generator
	*CatalogClient
}

var _ provider.Provider = (*Provider)(nil)

// ID returns the provider configuration ID.
func (p *Provider) ID() ulid.ULID {
	return p.id
}

func init() {
	provider.Register(provider.ProviderTypeOpenRouter, provider.Registration{
		Factory:   NewProvider,
		APITypes:  []provider.APIType{provider.APITypeOpenAIResponses, provider.APITypeOpenAIChatCompletions},
		AuthTypes: []auth.Type{auth.TypeAPIKey},
	})
}

// NewProvider constructs an OpenRouter provider using the supplied options.
func NewProvider(conf provider.Config, options provider.Options) (p provider.Provider, err error) {
	client := &Provider{id: conf.ID}
	switch conf.APIType {
	case provider.APITypeOpenAIResponses:
		if client.Generator, err = openai.NewResponses(conf, options); err != nil {
			return nil, err
		}
	case provider.APITypeOpenAIChatCompletions:
		if client.Generator, err = openai.NewChatCompletions(conf, options); err != nil {
			return nil, err
		}
	default:
		return nil, errors.ErrUnsupportedAPIType
	}

	if client.CatalogClient, err = NewCatalog(conf, options); err != nil {
		return nil, err
	}
	return client, nil
}
