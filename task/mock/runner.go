package mock

import (
	"context"

	"go.rtnl.ai/horizon/task"
)

type Runner struct {
	OnPrepare  func(ctx context.Context, task *task.Task) error
	OnFinalize func(ctx context.Context, output *task.Output) error
}

var _ task.Runner = (*Runner)(nil)

func (r *Runner) Prepare(ctx context.Context, task *task.Task) error {
	if r.OnPrepare != nil {
		return r.OnPrepare(ctx, task)
	}
	return nil
}

func (r *Runner) Finalize(ctx context.Context, output *task.Output) error {
	if r.OnFinalize != nil {
		return r.OnFinalize(ctx, output)
	}
	return nil
}
