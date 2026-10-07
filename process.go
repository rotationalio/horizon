package horizon

import (
	"context"

	"fmt"
	stdhttp "net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/config"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/task"
)

var tracer = otel.Tracer("go.rtnl.ai/horizon")

// A horizon process executes a horizon task using a runner.
type process struct {
	runner     task.Runner
	task       *task.Task
	input      *task.Input
	request    *provider.Request
	provider   provider.Provider
	horizon    *Horizon
	output     *task.Output
	config     config.Config
	httpClient *stdhttp.Client
}

// Constructs a new process, cloning the input and task which may be modified.
func newProcess(horizon *Horizon, runner task.Runner, input *task.Input, taskDefinition *task.Task, conf config.Config, client *stdhttp.Client) *process {
	return &process{
		horizon:    horizon,
		runner:     runner,
		task:       taskDefinition.Clone(),
		input:      input.Clone(),
		config:     conf,
		httpClient: client,
	}
}

func (p *process) Run(ctx context.Context) (output *task.Output, err error) {

	ctx, span := tracer.Start(ctx, "task.execute")
	defer func() {
		if err != nil {
			span.SetStatus(codes.Error, "task execution failed")
		}
		span.End()
	}()

	// Check to see if the context is already cancelled.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Ensure the process was setup correctly.
	if p.task == nil {
		return nil, errors.ErrTaskRequired
	}
	if p.runner == nil {
		return nil, errors.ErrRunnerRequired
	}
	if p.task.Output == nil {
		return nil, errors.ErrTaskOutputRequired
	}

	// Record the start time.
	p.output = &task.Output{Started: time.Now()}

	defer func() {
		// Step 6: Finalize the runner after the execution path completes.
		p.output.Finished = time.Now()
		executionErr := err
		executionOutcome := outcomeFor(executionErr)
		p.output.Outcome = executionOutcome
		span.AddEvent("task.finalizing", trace.WithAttributes(attribute.String("task.outcome", string(executionOutcome))))

		finalizeErr := p.Finalize(ctx, executionErr)
		if finalizeErr != nil {
			err = errors.Join(err, fmt.Errorf("finalize task: %w", finalizeErr))
			if executionOutcome == task.OutcomeSucceeded {
				executionOutcome = task.OutcomeFailed
			}
		}
		p.output.Outcome = executionOutcome
		output = p.output
	}()

	// Step 0: Prepare the runner and resolve the provider for this task.
	if err = cancelAndTrace(ctx, "task.prepare", p.Prepare); err != nil {
		return p.output, fmt.Errorf("prepare task: %w", err)
	}

	// Step 1a: Preprocess input before rendering.
	if err = cancelAndTrace(ctx, "task.process_input", p.ProcessInput); err != nil {
		return p.output, fmt.Errorf("process task input: %w", err)
	}

	// Step 1b: Preprocess template context before rendering.
	if err = cancelAndTrace(ctx, "task.process_context", p.ProcessContext); err != nil {
		return p.output, fmt.Errorf("process task context: %w", err)
	}

	// Prepare the request object for generation.
	p.request = &provider.Request{
		Model:        p.task.Model.Slug,
		Params:       p.task.Model.Parameters,
		OutputSchema: p.task.Output.Schema,
	}

	// Step 2: Prepare attachments for the inference request.
	if err = cancelAndTrace(ctx, "task.process_attachments", p.ProcessAttachments); err != nil {
		return p.output, fmt.Errorf("process task attachments: %w", err)
	}

	// Step 3: Render task prompts into the provider request.
	if err = cancelAndTrace(ctx, "task.render", p.Render); err != nil {
		return p.output, fmt.Errorf("render task prompts: %w", err)
	}

	// Step 4: Perform one provider inference request.
	if err = cancelAndTrace(ctx, "task.generate", p.Generate); err != nil {
		return p.output, fmt.Errorf("generate task output: %w", err)
	}

	return p.output, nil
}

func (p *process) Prepare(ctx context.Context) (err error) {
	if err = p.runner.Prepare(ctx, p.task); err != nil {
		return fmt.Errorf("prepare execution: %w", err)
	}
	if p.task.Provider == nil {
		return errors.ErrProviderRequired
	}

	// TODO: Validate that the provider can handle the task.

	if p.horizon == nil {
		p.provider, err = provider.NewWithHTTPClient(*p.task.Provider, p.httpClient)
	} else {
		p.provider, err = p.horizon.providers.GetOrCreate(*p.task.Provider, nil)
	}
	if err != nil {
		return fmt.Errorf("configure inference provider: %w", err)
	}
	return nil
}

func (p *process) ProcessInput(_ context.Context) (err error) {
	if preprocessor, ok := p.runner.(task.InputProcessor); ok {
		if p.input, err = preprocessor.ProcessInput(p.input); err != nil {
			return fmt.Errorf("process input: %w", err)
		}
	}
	return nil
}

func (p *process) ProcessContext(_ context.Context) (err error) {
	if preprocessor, ok := p.runner.(task.ContextProcessor); ok {
		if p.input.Context, err = preprocessor.ProcessContext(p.input.Context); err != nil {
			return fmt.Errorf("process context: %w", err)
		}
	}
	return nil
}

func (p *process) ProcessAttachments(_ context.Context) (err error) {
	if preprocessor, ok := p.runner.(task.AttachmentProcessor); ok {
		p.request.Attachments = make(attachments.Attachments, len(p.input.Attachments))
		for i, attachment := range p.input.Attachments {
			if p.request.Attachments[i], err = preprocessor.ProcessAttachment(attachment); err != nil {
				return fmt.Errorf("process attachment %d: %w", i, err)
			}
		}
		return nil
	}

	// Otherwise just use the attachments as defined in the input.
	p.request.Attachments = p.input.Attachments
	return nil
}

func (p *process) Render(_ context.Context) (err error) {
	if renderer, ok := p.runner.(task.Renderer); ok {
		p.request.Input, err = renderer.Render(p.task.Prompts, p.input.Context)
	} else {
		// Use the default renderer to render the prompts.
		p.request.Input, err = prompts.Render(p.task.Prompts, p.input.Context)
	}
	return err
}

// Finalize the task with a short timeout separate from the caller context.
func (p *process) Finalize(ctx context.Context, executionErr error) (err error) {
	finalizeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), p.config.FinalizeTimeout)
	err = p.runner.Finalize(finalizeCtx, p.output, executionErr)
	cancel()
	return err
}

// Returns a task outcome based on the given error.
func outcomeFor(err error) task.Outcome {
	if err == nil {
		return task.OutcomeSucceeded
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return task.OutcomeCancelled
	}
	return task.OutcomeFailed
}

// Checks cancellation before and after a stage while tracing the stage execution.
func cancelAndTrace(ctx context.Context, name string, fn func(context.Context) error) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}

	// The span only wraps the stage execution, not the context cancellation.
	ctx, span := tracer.Start(ctx, name)
	if err = fn(ctx); err != nil {
		span.SetStatus(codes.Error, "stage failed")
	}
	span.End()

	return errors.Join(err, ctx.Err())
}
