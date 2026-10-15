package horizon

import (
	"context"
	"fmt"
	"log/slog"
	stdhttp "net/http"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.rtnl.ai/horizon/actions"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/config"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/schema"

	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/task"

	"go.rtnl.ai/x/rlog"
)

var tracer = otel.Tracer("go.rtnl.ai/horizon")

// A horizon process executes a horizon task using a runner.
type process struct {
	runner  task.Runner
	task    *task.Task
	input   *task.Input
	request *provider.Request
	// NOTE: Do not append to messages, call process.appendMessages so that
	// the transcript is kept in sync with output actions log.
	messages   prompts.Prompts
	provider   provider.Provider
	horizon    *Horizon
	output     *task.Output
	parser     schema.Parser
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
		parser:     schema.NewParser(conf.BestEffortParsing),
	}
}

func (p *process) Run(ctx context.Context) (output *task.Output, err error) {
	ctx, span := tracer.Start(ctx, "task.execute")
	if p.task != nil {
		span.SetAttributes(attribute.String("task.slug", p.task.Slug))
	}
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
	p.output = &task.Output{Started: time.Now(), Actions: actions.New()}

	defer func() {
		// Step 6: Finalize the runner after the execution path completes.
		p.output.Finished = time.Now()
		for _, message := range p.messages {
			if message == nil {
				continue
			}
			if !message.Type.IsKnown() {
				rlog.WarnAttrs(ctx, "message has unknown prompt type", slog.Int("type", int(message.Type)))
			}
			if !message.Role.IsKnown() {
				rlog.WarnAttrs(ctx, "message has unknown prompt role", slog.Int("role", int(message.Role)))
			}
		}

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
		Params:       p.task.Model.Parameters.ProviderOnly(),
		OutputSchema: p.task.Output.Schema,
	}

	// Step 2: Prepare attachments for the inference request.
	if err = cancelAndTrace(ctx, "task.process_attachments", p.ProcessAttachments); err != nil {
		return p.output, fmt.Errorf("process task attachments: %w", err)
	}

	// Step 3: Render task prompts into the provider request, ensure we have at
	// least one user message for attachments, and append initial messages to
	// the actions log.
	err = cancelAndTrace(ctx, "task.render", p.Render)
	p.messages = p.request.Input
	if err != nil {
		return p.output, fmt.Errorf("render task prompts: %w", err)
	}
	p.ensureAttachmentMessage()
	p.output.Actions.AddMessage(ctx, actions.EntityUser, actions.EntityRunner, p.messages, actions.WithAttachments(p.request.Attachments...))

	// Step 4: Generate once, or execute capabilities until a final answer arrives.
	generate := func(ctx context.Context) error {
		response, err := p.Generate(ctx)
		return p.captureResponse(response, err)
	}
	stage := "task.generate"
	if len(p.task.Capabilities) > 0 {
		generate = p.CapabilitiesLoop
		stage = "task.capabilities_loop"
	}
	if err = cancelAndTrace(ctx, stage, generate); err != nil {
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
		p.provider, err = provider.New(*p.task.Provider, provider.WithHTTPClient(p.httpClient))
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

// ProcessAttachments applies host preprocessing to each attachment and downloads
// and caches attachment data once for the lifetime of the process.
func (p *process) ProcessAttachments(ctx context.Context) error {
	if p.input.Attachments == nil {
		return nil
	}

	preprocessor, hasAP := p.runner.(task.AttachmentProcessor)
	p.request.Attachments = make(attachments.Attachments, 0, len(p.input.Attachments))
	for i, attachment := range p.input.Attachments {
		if err := ctx.Err(); err != nil {
			return err
		}

		if attachment == nil {
			return errors.ErrNilAttachment
		}

		if hasAP {
			var err error
			if attachment, err = preprocessor.ProcessAttachment(attachment); err != nil {
				return fmt.Errorf("process attachment %d: %w", i, err)
			}
		}

		if attachment == nil {
			return errors.ErrNilAttachment
		}

		// Only download the attachment data if it's not already cached.
		if err := attachment.Download(ctx); err != nil {
			return fmt.Errorf("download attachment %d: %w", i, err)
		}
		p.request.Attachments = append(p.request.Attachments, attachment.Clone())
	}
	return nil
}

func (p *process) Render(_ context.Context) (err error) {
	if renderer, ok := p.runner.(task.Renderer); ok {
		p.request.Input, err = renderer.Render(p.task.Prompts, p.input.Context)
	} else {
		// Use the default renderer to render the prompts.
		p.request.Input, err = p.task.Prompts.Render(p.input.Context)
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

// appendMessages keeps the live transcript and action-log message entries in sync.
func (p *process) appendMessages(ctx context.Context, sender, receiver actions.Entity, messages prompts.Prompts, options ...actions.Option) {
	p.output.Actions.AddMessage(ctx, sender, receiver, messages, options...)
	p.messages = append(p.messages, messages...)
}

// Checks for an existing user message; if none exists, supplies a default one
// for attachment-only input.
func (p *process) ensureAttachmentMessage() {
	if len(p.request.Attachments) == 0 {
		return
	}

	for _, message := range p.messages {
		if message != nil && message.Role == prompts.RoleUser {
			return
		}
	}

	p.messages = append(p.messages, &prompts.Prompt{
		Role:    prompts.RoleUser,
		Type:    prompts.TypeMessage,
		Content: "This message contains only attachments.",
	})
}
