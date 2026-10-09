package prompts

import (
	"context"
	"log/slog"
	"sort"
	"sync"

	"go.rtnl.ai/horizon/config"
	"go.rtnl.ai/horizon/render"
	"go.rtnl.ai/x/rlog"
)

var (
	rendererCache *render.Cache
	renderInit    sync.Once
	renderErr     error
)

// The default renderer uses a cache to avoid repeated parsing of the same template.
// NOTE: that the data context is passed to every single prompt template.
func Render(templates Templates, ctx Context) (input Prompts, err error) {
	// Ensure the renderer cache is initialized only once.
	renderInit.Do(func() {
		conf, _ := config.Get()
		size := conf.RendererCacheSize

		if rendererCache, renderErr = render.NewCache(size, nil); renderErr != nil {
			rlog.ErrorAttrs(context.Background(), "could not initialize renderer cache", slog.Any("error", renderErr))
		}
	})

	if renderErr != nil {
		return nil, renderErr
	}

	// Render the prompts into LLM message inputs.
	input = make(Prompts, 0, len(templates))
	for _, prompt := range templates {
		var rendered string
		if rendered, err = rendererCache.Render(prompt.Renderer, prompt.Content, ctx); err != nil {
			return nil, err
		}

		input = append(input, &Prompt{
			Index:   prompt.Index,
			Role:    prompt.Role,
			Content: rendered,
		})
	}

	// Ensure the messages are sorted by index.
	sort.Slice(input, func(i, j int) bool {
		return input[i].Index < input[j].Index
	})

	return input, nil
}
