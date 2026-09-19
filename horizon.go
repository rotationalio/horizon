package horizon

import (
	"context"
	"time"

	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/provider"
)

// var tracer = otel.Tracer("go.rtnl.ai/horizon")

// Executes the horizon task using the provided runner and input context.
func Run(ctx context.Context, input *Input, task *Task, runner Runner) (output *Output, err error) {
	proc := &process{
		runner: runner,
		task:   task,
		input:  input,
	}
	return proc.Run(ctx)
}

// A horizon process executes a horizon task using a runner.
type process struct {
	runner   Runner
	task     *Task
	input    *Input
	request  *provider.Request
	output   *Output
	response *provider.Response
}

func (p *process) Run(ctx context.Context) (output *Output, err error) {
	// Step 0: Prepare the runner for the task.
	// At the end of the prepare step, the task should have a valid provider.
	if err = p.Prepare(ctx); err != nil {
		return nil, err
	}

	// Create the output object to start executing the task.
	p.output = &Output{
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
	p.request = p.task.request()

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

		return nil
	})
}

func (p *process) ProcessInput(ctx context.Context) error {
	return cancel(ctx, func(ctx context.Context) (err error) {
		if p.runner == nil {
			return nil
		}

		if preprocessor, ok := p.runner.(InputProcessor); ok {
			if p.input, err = preprocessor.ProcessInput(p.input); err != nil {
				return err
			}
		}
		return nil
	})
}

func (p *process) ProcessContext(ctx context.Context) error {
	return cancel(ctx, func(ctx context.Context) (err error) {
		if p.runner == nil {
			return nil
		}

		if preprocessor, ok := p.runner.(ContextProcessor); ok {
			if p.input.Context, err = preprocessor.ProcessContext(p.input.Context); err != nil {
				return err
			}
		}
		return nil
	})
}

func (p *process) ProcessAttachments(ctx context.Context) error {
	return cancel(ctx, func(ctx context.Context) (err error) {
		if p.runner != nil {
			if preprocessor, ok := p.runner.(AttachmentProcessor); ok {
				p.request.Attachments = make(attachments.Attachments, len(p.input.Attachments))
				for i, attachment := range p.input.Attachments {
					if p.request.Attachments[i], err = preprocessor.ProcessAttachment(attachment); err != nil {
						return err
					}
				}
				return nil
			}
		}

		// Otherwise just use the attachments as defined in the input.
		p.request.Attachments = p.input.Attachments
		return nil
	})
}

func (p *process) Render(ctx context.Context) error {
	return cancel(ctx, func(ctx context.Context) (err error) {
		if p.runner != nil {
			if renderer, ok := p.runner.(Renderer); ok {
				if p.request.Input, err = renderer.Render(p.task.Prompts, p.input.Context); err != nil {
					return err
				}
				return nil
			}
		}

		// Use the default renderer to render the prompts.
		if p.request.Input, err = Render(p.task.Prompts, p.input.Context); err != nil {
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
		if p.runner == nil {
			return nil
		}

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
