package openai

import (
	"context"
	"log/slog"

	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/catalog"
	"go.rtnl.ai/x/rlog"
)

func (p *Provider) Status(ctx context.Context) (status provider.Status, err error) {
	if p == nil {
		return provider.StatusError, errors.ErrInferenceRequired
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
