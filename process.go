package horizon

import (
	"context"

	"time"

	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/task"
)

// var tracer = otel.Tracer("go.rtnl.ai/horizon")

// A horizon process executes a horizon task using a runner.
type process struct {
	horizon  *Horizon
	runner   task.Runner
	task     *task.Task
	input    *task.Input
	provider provider.Provider
	request  *provider.Request
	output   *task.Output
}

func (p *process) Run(ctx context.Context) (output *task.Output, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p.task == nil {
		return nil, errors.ErrTaskRequired
	}
	if p.runner == nil {
		return nil, errors.ErrRunnerRequired
	}
	if p.task.Output == nil {
		return nil, errors.ErrTaskOutputRequired
	}

	p.task = p.task.Clone()
	p.input = p.input.Clone()

	// Step 0: Prepare the runner for the task.
	// At the end of the prepare step, the task should have a valid provider.
	if err = p.Prepare(ctx); err != nil {
		return nil, err
	}

	// Create the output object to start executing the task.
	p.output = &task.Output{
		Started: time.Now(),
	}

	// Step 1a: Input Pre-Processing Before Rendering
	if err = p.ProcessInput(ctx); err != nil {
		return nil, err
	}

	// Step 1b: Context Pre-Processing Before Rendering
	if err = p.ProcessContext(ctx); err != nil {
		return nil, err
	}

	// Create the request object to start the generation/tool calling loop.
	if p.task.Output == nil {
		return nil, errors.ErrTaskOutputRequired
	}
	p.request = &provider.Request{
		Model:        p.task.Model.Slug,
		Params:       p.task.Model.Parameters,
		OutputSchema: p.task.Output.Schema,
	}

	// Step 2: Prepare attachments before creating an LLM request.
	if err = p.ProcessAttachments(ctx); err != nil {
		return nil, err
	}

	// Step 3: Render the prompts
	if err = p.Render(ctx); err != nil {
		return nil, err
	}

	// Step 4: Execute the Generate/Tool Calling Loop
	if err = p.Generate(ctx); err != nil {
		return nil, err
	}

	// Step 6: Finalize the runner for the task.
	p.output.Finished = time.Now()
	if err = p.Finalize(ctx); err != nil {
		return nil, err
	}

	return p.output, nil
}

func (p *process) Prepare(ctx context.Context) error {
	return cancel(ctx, func(ctx context.Context) (err error) {
		if err := p.runner.Prepare(ctx, p.task); err != nil {
			return err
		}

		// TODO: System Prepare
		// TODO: Validate that the provider can handle the task.
		// One-off executions already have an explicitly supplied provider.
		if p.provider != nil {
			return nil
		}
		if p.task.Provider == nil {
			return errors.ErrProviderRequired
		}
		if p.provider, err = p.horizon.providers.GetOrCreate(*p.task.Provider, nil); err != nil {
			return err
		}

		return nil
	})
}

func (p *process) ProcessInput(ctx context.Context) error {
	return cancel(ctx, func(ctx context.Context) (err error) {
		if preprocessor, ok := p.runner.(task.InputProcessor); ok {
			if p.input, err = preprocessor.ProcessInput(p.input); err != nil {
				return err
			}
		}
		return nil
	})
}

func (p *process) ProcessContext(ctx context.Context) error {
	return cancel(ctx, func(ctx context.Context) (err error) {
		if preprocessor, ok := p.runner.(task.ContextProcessor); ok {
			if p.input.Context, err = preprocessor.ProcessContext(p.input.Context); err != nil {
				return err
			}
		}
		return nil
	})
}

func (p *process) ProcessAttachments(ctx context.Context) error {
	return cancel(ctx, func(ctx context.Context) (err error) {
		if preprocessor, ok := p.runner.(task.AttachmentProcessor); ok {
			p.request.Attachments = make(attachments.Attachments, len(p.input.Attachments))
			for i, attachment := range p.input.Attachments {
				if p.request.Attachments[i], err = preprocessor.ProcessAttachment(attachment); err != nil {
					return err
				}
			}
			return nil
		}

		// Otherwise just use the attachments as defined in the input.
		p.request.Attachments = p.input.Attachments
		return nil
	})
}

func (p *process) Render(ctx context.Context) error {
	return cancel(ctx, func(ctx context.Context) (err error) {
		if renderer, ok := p.runner.(task.Renderer); ok {
			if p.request.Input, err = renderer.Render(p.task.Prompts, p.input.Context); err != nil {
				return err
			}
			return nil
		}

		// Use the default renderer to render the prompts.
		if p.request.Input, err = prompts.Render(p.task.Prompts, p.input.Context); err != nil {
			return err
		}
		return nil
	})
}

func (p *process) Generate(ctx context.Context) error {
	// TODO: Implement the Generate/Tool Calling Loop
	return cancel(ctx, func(ctx context.Context) (err error) {
		return nil
	})
}

func (p *process) Finalize(ctx context.Context) error {
	return cancel(ctx, func(ctx context.Context) (err error) {
		if err = p.runner.Finalize(ctx, p.output); err != nil {
			return err
		}
		return nil
	})
}

// Decorator to check if the context is done after the function call.
func cancel(ctx context.Context, f func(context.Context) error) (err error) {
	if err = f(ctx); err != nil {
		return err
	}

	// This is a long running operation, check if the context is done.
	if err = ctx.Err(); err != nil {
		return err
	}
	return nil
}
