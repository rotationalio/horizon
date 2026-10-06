package horizon

import (
	"context"
	"fmt"

	"go.rtnl.ai/horizon/errors"
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
	providers *provider.Cache
	factory   RunnerFactory
	handler   *Handler
}

// New creates a Horizon using factory for embedded and HTTP executions.
// The factory must be non-nil.
func New(factory RunnerFactory) *Horizon {
	h := &Horizon{
		providers: provider.NewCache(),
		factory:   factory,
	}
	h.handler = &Handler{horizon: h}
	return h
}

// AddProvider prewarms the cache with config. Executions also populate the cache
// automatically from their task's provider configuration.
func (h *Horizon) AddProvider(config provider.Config) error {
	return h.providers.Add(config)
}

// AddProviderInstance caches an already-constructed provider (non-nil) with its
// configuration while always replacing any existing provider with the same ID.
func (h *Horizon) AddProviderInstance(config provider.Config, instance provider.Provider) error {
	return h.providers.AddInstance(config, instance)
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
func (h *Horizon) Run(ctx context.Context, input *task.Input, taskDefinition *task.Task) (*task.Output, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	runner, err := h.factory.NewRunner(ctx)
	if err != nil {
		return nil, fmt.Errorf("create task runner: %w", err)
	}
	proc := &process{
		horizon: h,
		runner:  runner,
		task:    taskDefinition,
		input:   input,
	}
	return proc.Run(ctx)
}

// Run executes a one-off task with an explicit runner and provider, without a
// [Horizon] or [provider.Cache]. Use [NewProvider] to construct a provider
// from configuration.
func Run(ctx context.Context, input *task.Input, taskDefinition *task.Task, runner task.Runner, provider provider.Provider) (*task.Output, error) {
	if provider == nil {
		return nil, errors.ErrProviderRequired
	}
	proc := &process{
		runner:   runner,
		task:     taskDefinition,
		input:    input,
		provider: provider,
	}
	return proc.Run(ctx)
}
