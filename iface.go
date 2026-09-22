package horizon

import (
	"context"

	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider/api"
)

// A horizon runner defines the methods required to execute a task. Horizon execution
// uses the runner to manage the execution process and to provide a consistent interface
// to the results and the audit logging.
type Runner interface {
	// The prepare method will always be called first allowing the runner to get access
	// to the task configuration and to perform any necessary setup. Task validation
	// should occur at this step as in general, the runner should error as early as
	// possible in the task flow.
	Prepare(context.Context, *Task) error

	// Execute an LLM generation or model inference request to the runner backend.
	// Generate(context.Context, *Request) (*Response, error)

	// The finalize method will always be called last allowing the runner to perform any
	// necessary cleanup or modify the results of the task execution.
	Finalize(context.Context, *Output) error
}

// If the runner implements this interface, it will be used to pre-process the input
// before it is rendered into prompts. This is useful for adding additional context
// or metadata to the input or managing any attachments with the input.
type InputProcessor interface {
	ProcessInput(*Input) (*Input, error)
}

// If the runner implements this interface, it will be used to pre-process the context
// before it is rendered into prompts. Generally speaking this is done to convert the
// context into a renderable format or to validate the context matches some schema, or
// otherwise preparing the context for rendering.
type ContextProcessor interface {
	ProcessContext(prompts.Context) (prompts.Context, error)
}

// If the runner implements this interface, it will be called for each attachment in
// the input before the request to the LLM is created. Generally speaking this is done
// to load attachments into memory from disk or S3 or to prepare URLs for the LLM.
type AttachmentProcessor interface {
	ProcessAttachment(*attachments.Attachment) (*attachments.Attachment, error)
}

// Runners can optionally implement the render interface to prepare the input being
// sent to the LLM. If a runner does not implement this interface, the prompts will be
// rendered using the default renderer.
type Renderer interface {
	Render(prompts.Templates, prompts.Context) (prompts.Prompts, error)
}

// InputGuards are used to validate the input before it is sent to the LLM, particularly
// to detect security or privacy issues that should be prevented from being sent to the
// the LLM. The InputGuard interface is used to validate every request, even tool calls
// before being sent to the LLM.
type InputGuard interface {
	ProtectInput(*api.Request) error
}

// OutputGuards are used to validate LLM output responses to ensure that they are not
// leaking sensitive information or other data that should not be returned to the user.
// The OutputGuard is applied to every LLM response, even ones that are requesting tool
// calls or other intermediate responses.
type OutputGuard interface {
	ProtectOutput(*api.Response) error
}
