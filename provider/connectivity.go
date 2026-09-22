package provider

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	openaigo "github.com/openai/openai-go/v3"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/provider/api"
	"go.rtnl.ai/horizon/provider/catalog"
	"go.rtnl.ai/horizon/provider/openai"
	"go.rtnl.ai/horizon/provider/openrouter"
	"go.rtnl.ai/horizon/provider/types"
	"go.rtnl.ai/x/rlog"
)

// CheckConnectivity runs basic inference and catalog connectivity checks.
func (p *Provider) CheckConnectivity(ctx context.Context) (types.Status, error) {
	if p == nil {
		return types.StatusError, errors.ErrInferenceRequired
	}

	// Catalog connectivity check.
	model, err := connectivityTestModel(p, ctx)
	if err != nil {
		err = fmt.Errorf("model fetch error: %w", err)
		if authRequired(err) {
			return types.StatusAuthRequired, err
		}
		return types.StatusError, err
	}

	// Inference connectivity check.
	rep, err := p.Generate(ctx, &api.Request{
		Model:  model.Slug,
		Params: params.New(map[string]any{params.MaxTokens: int64(10)}),
		Input: []api.Message{
			{Role: api.RoleSystem, Content: "respond with one word"},
			{Role: api.RoleUser, Content: "ping"},
		},
	})
	if err != nil {
		err = fmt.Errorf("generate error: %w", err)
		if authRequired(err) {
			return types.StatusAuthRequired, err
		}
		return types.StatusError, err
	}
	if rep == nil {
		rlog.ErrorAttrs(ctx, "provider connectivity inference returned no output",
			slog.String("phase", "inference"),
			slog.String("model", model.Slug),
			slog.Any("error", errors.ErrNoModelOutput),
		)
		return types.StatusError, errors.ErrNoModelOutput
	}

	return types.StatusConnected, nil
}

// Returns the model to use for connectivity testing.
func connectivityTestModel(p *Provider, ctx context.Context) (*catalog.Model, error) {
	switch p.conf.ProviderType {
	case types.ProviderTypeOpenRouter:
		return retrieveConnectivityModel(p, ctx, openrouter.ConnectivityModel)
	case types.ProviderTypeOpenAI:
		return retrieveConnectivityModel(p, ctx, openai.ConnectivityModel)
	case types.ProviderTypeOpenAICompatible:
		return retrieveConnectivityModel(p, ctx, p.conf.DefaultModel)
	default:
		return nil, errors.ErrUnsupportedProviderType
	}
}

// Retrieves a model from the provider catalog by model ID.
func retrieveConnectivityModel(p *Provider, ctx context.Context, modelID string) (*catalog.Model, error) {
	model, err := p.RetrieveModel(ctx, modelID)
	if err != nil {
		return nil, err
	}
	if model == nil || model.Slug == "" {
		return nil, errors.ErrModelSlugRequired
	}
	return model, nil
}

// authRequired reports whether err (or its unwrap chain) indicates missing or
// invalid credentials.
func authRequired(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *openaigo.Error
	if errors.As(err, &apiErr) {
		return isHTTPAuthStatus(apiErr.StatusCode)
	}

	return errorChainContainsHTTPAuthStatus(err)
}

// errorChainContainsHTTPAuthStatus checks if the error chain contains an HTTP
// unauthorized or forbidden status code.
func errorChainContainsHTTPAuthStatus(err error) bool {
	unauthorized := fmt.Sprintf("%d %s", http.StatusUnauthorized, http.StatusText(http.StatusUnauthorized))
	forbidden := fmt.Sprintf("%d %s", http.StatusForbidden, http.StatusText(http.StatusForbidden))

	var walk func(error) bool
	walk = func(e error) bool {
		if e == nil {
			return false
		}
		var apiErr *openaigo.Error
		if errors.As(e, &apiErr) {
			return isHTTPAuthStatus(apiErr.StatusCode)
		}
		msg := e.Error()
		if strings.Contains(msg, unauthorized) || strings.Contains(msg, forbidden) {
			return true
		}
		type unwrapperMulti interface{ Unwrap() []error }
		if mw, ok := e.(unwrapperMulti); ok {
			if slices.ContainsFunc(mw.Unwrap(), walk) {
				return true
			}
		}
		return walk(errors.Unwrap(e))
	}
	return walk(err)
}

// isHTTPAuthStatus reports whether code indicates missing or invalid credentials.
func isHTTPAuthStatus(code int) bool {
	return code == http.StatusUnauthorized || code == http.StatusForbidden
}

// StatusFromError maps a provider client error to a connection status.
func StatusFromError(err error) types.Status {
	if authRequired(err) {
		return types.StatusAuthRequired
	}
	return types.StatusError
}
