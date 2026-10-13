package templates

import (
	"context"
	"log/slog"
	"sort"
	"sync"

	"go.rtnl.ai/horizon/config"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/render"
	"go.rtnl.ai/x/rlog"
)

var (
	rendererCache *render.Cache
	renderInit    sync.Once
	renderErr     error
)

// Render uses the default cached renderer to convert templates into prompts.
func Render(templates Templates, ctx prompts.Context) (prompts.Prompts, error) {
	renderInit.Do(func() {
		conf, _ := config.Get()
		if rendererCache, renderErr = render.NewCache(conf.RendererCacheSize, nil); renderErr != nil {
			rlog.ErrorAttrs(context.Background(), "could not initialize renderer cache", slog.Any("error", renderErr))
		}
	})
	if renderErr != nil {
		return nil, renderErr
	}

	input := make(prompts.Prompts, 0, len(templates))
	for _, template := range templates {
		rendered, err := rendererCache.Render(template.Renderer, template.Content, ctx)
		if err != nil {
			return nil, err
		}

		input = append(input, &prompts.Prompt{
			Index:   template.Index,
			Role:    template.Role,
			Type:    prompts.TypeMessage,
			Content: rendered,
		})
	}

	sort.Slice(input, func(i, j int) bool {
		return input[i].Index < input[j].Index
	})
	return input, nil
}
