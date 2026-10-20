package openrouter

import (
	"context"
	"log/slog"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/x/rlog"

	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/catalog"
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

// Status returns the current status of the provider.
func (p *Provider) Status(ctx context.Context) (status provider.Status, err error) {
	if p == nil {
		return provider.StatusError, errors.ErrCatalogRequired
	}

	// Check catalog connectivity
	var model *catalog.Model
	if model, err = p.RetrieveModel(ctx, ConnectivityModel); err != nil {
		return provider.StatusFromError(err), err
	}

	// Check inference connectivity
	var rep *provider.Response
	if rep, err = p.Generate(ctx, &provider.Request{
		Model:  model.Slug,
		Params: params.New(map[string]any{params.MaxTokens: int64(10)}),
		Input: prompts.Prompts{
			{Role: prompts.RoleSystem, Content: "respond with one word"},
			{Role: prompts.RoleUser, Content: "ping"},
		},
	}); err != nil {
		return provider.StatusFromError(err), err
	}

	if rep == nil {
		rlog.ErrorAttrs(ctx, "provider connectivity inference returned no output",
			slog.String("phase", "inference"),
			slog.String("model", model.Slug),
			slog.Any("error", errors.ErrNoModelOutput),
		)
		return provider.StatusError, errors.ErrNoModelOutput
	}

	return provider.StatusConnected, nil
}
