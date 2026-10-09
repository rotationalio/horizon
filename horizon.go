package horizon

import (
	"context"
	"fmt"
	stdhttp "net/http"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.rtnl.ai/horizon/config"
	"go.rtnl.ai/horizon/http"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/ulid"
)

// RunnerFactory creates a fresh runner for each execution. Implementations must
// support concurrent calls and must not share mutable execution state between
// runners.
type RunnerFactory interface {
	NewRunner(context.Context) (task.Runner, error)
}

// Horizon owns shared provider registrations and creates execution-specific
// runners. Create with [New].
type Horizon struct {
	providers  *provider.Cache
	factory    RunnerFactory
	handler    *Handler
	config     config.Config
	httpClient *stdhttp.Client
}

// New creates a Horizon using factory for embedded and HTTP executions.
func New(factory RunnerFactory, options ...Option) (*Horizon, error) {
	conf, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("load Horizon configuration: %w", err)
	}

	resolved := ResolveOptions(options...)
	client := resolved.HTTPClient
	if client == nil {
		client, err = http.New()
		if err != nil {
			return nil, err
		}
	}

	providers, err := provider.NewCache(provider.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create provider cache: %w", err)
	}

	h := &Horizon{
		providers:  providers,
		factory:    factory,
		config:     conf,
		httpClient: client,
	}
	h.handler = &Handler{horizon: h}
	return h, nil
}

// GetProvider returns the cached provider with the given ID, if one exists.
func (h *Horizon) GetProvider(id ulid.ULID) (provider.Provider, error) {
	return h.providers.Get(id)
}

// GetOrCreateProvider returns the cached provider with the given ID, if one exists,
// or creates a new one using the given config and instance. If instance is nil,
// a new provider is constructed from the config.
func (h *Horizon) GetOrCreateProvider(config provider.Config, instance provider.Provider) (provider.Provider, error) {
	return h.providers.GetOrCreate(config, instance)
}

// RemoveProvider removes a cached provider and reports whether it existed.
// In-flight executions retain any provider instance they already resolved.
func (h *Horizon) RemoveProvider(id ulid.ULID) bool {
	return h.providers.Remove(id)
}

// Handler returns Horizon's HTTP handler and its editable task router.
func (h *Horizon) Handler() *Handler {
	return h.handler
}

// Run creates a runner and executes taskDefinition with input.
func (h *Horizon) Run(ctx context.Context, input *task.Input, taskDefinition *task.Task) (output *task.Output, err error) {
	ctx, span := tracer.Start(ctx, "horizon.run")
	defer func() {
		if err != nil {
			span.SetStatus(codes.Error, "horizon run failed")
		}
		span.End()
	}()

	ctx, cancel := withExecutionTimeout(ctx, h.config.ExecutionTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	runner, err := h.factory.NewRunner(ctx)
	if err != nil {
		return nil, fmt.Errorf("create task runner: %w", err)
	}
	proc := newProcess(h, runner, input, taskDefinition, h.config, nil)
	return proc.Run(ctx)
}

// Run executes a one-off task with an explicit runner.
func Run(ctx context.Context, input *task.Input, tsk *task.Task, runner task.Runner) (*task.Output, error) {
	conf, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("load Horizon configuration: %w", err)
	}

	client, err := http.New()
	if err != nil {
		return nil, err
	}

	ctx, cancel := withExecutionTimeout(ctx, conf.ExecutionTimeout)
	defer cancel()

	proc := newProcess(nil, runner, input, tsk, conf, client)
	return proc.Run(ctx)
}

// Sets an execution timeout on the context if one is configured, otherwise
// returns the context as-is and a no-op cancel function.
func withExecutionTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout == 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}
