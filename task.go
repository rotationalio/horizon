package horizon

import (
	"context"

	"go.rtnl.ai/horizon/modality"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/mime"
	"go.rtnl.ai/x/semver"
)

// A JSON object is a map of string keys to any values.
type JSON map[string]any

//============================================================================
// Detailed Task Data Definitions
//============================================================================

// A collection of [Task]s.
type Tasks []Task

// A single, independent unit of work that can be executed, including all of the
// necessary inputs, outputs, clients, capabilities, and other configuration.
type Task struct {
	// Task Definition
	ID           ulid.ULID         `json:"id,omitempty" yaml:"id,omitempty" msg:"id,omitempty"`                               // The ID of the task
	Slug         string            `json:"slug" yaml:"slug" msg:"slug"`                                                       // The slug of the task
	Name         string            `json:"name" yaml:"name" msg:"name"`                                                       // The name of the task
	Description  string            `json:"description,omitempty" yaml:"description,omitempty" msg:"description,omitempty"`    // The description of the task
	Version      semver.Version    `json:"version" yaml:"version" msg:"version"`                                              // The version of the task
	Input        *TaskInput        `json:"input,omitempty" yaml:"input,omitempty" msg:"input,omitempty"`                      // The input schema of the task
	Output       *TaskOutput       `json:"output" yaml:"output" msg:"output"`                                                 // The output type of the task
	Model        Model             `json:"model" yaml:"model" msg:"model"`                                                    // The Model used for the task
	Prompts      prompts.Templates `json:"prompts" yaml:"prompts" msg:"prompts"`                                              // The prompts used for the task
	Capabilities *Capabilities     `json:"capabilities,omitempty" yaml:"capabilities,omitempty" msg:"capabilities,omitempty"` // The capabilities enabled for the task

	// Task Execution Configuration
	// Tools *config.Tools
	Provider *provider.Config `json:"provider,omitempty" yaml:"provider,omitempty" msg:"provider,omitempty"` // The inference provider used for the task
}

// Defines the input for a task including the modalities, context type, and any
// schemas used to validate the input context.
type TaskInput struct {
	Modality modality.Modality `json:"modality" yaml:"modality" msg:"modality"`
	Context  mime.Type         `json:"context" yaml:"context" msg:"context"`
	Schema   *schema.Schema    `json:"schema,omitempty" yaml:"schema,omitempty" msg:"schema,omitempty"`
}

// Defines the output for a task including the modalities, and any schemas used to
// validate or construct the output (such as JSON schemas being passed to the LLM).
type TaskOutput struct {
	Modality modality.Modality `json:"modality" yaml:"modality" msg:"modality"`
	Schema   *schema.Schema    `json:"schema,omitempty" yaml:"schema,omitempty" msg:"schema,omitempty"`
}

// Defines the model used for a task by its slug and model parameters. The model is
// converted into the correct identifying LLM or VLM on provider request.
type Model struct {
	Slug       string         `json:"slug" yaml:"slug" msg:"slug"`
	Parameters *params.Params `json:"parameters,omitempty" yaml:"parameters,omitempty" msg:"parameters,omitempty"`
}

// Capabilities that can be used by the LLM during a task execution.
type Capabilities struct {
	Tools     []string `json:"tools,omitempty" yaml:"tools,omitempty" msg:"tools,omitempty"`             // The tools enabled for the task
	Resources []string `json:"resources,omitempty" yaml:"resources,omitempty" msg:"resources,omitempty"` // The resources enabled for the task
	Prompts   []string `json:"prompts,omitempty" yaml:"prompts,omitempty" msg:"prompts,omitempty"`       // The prompts enabled for the task
}

// Executes the task with the given input and runner, returning the result.
func (t *Task) Run(ctx context.Context, input *Input, runner Runner) (output *Output, err error) {
	return Run(ctx, input, t, runner)
}

//============================================================================
// Internal Task Helper Functions
//============================================================================

// Creates a basic provider request from the task definition.
func (t *Task) request() *provider.Request {
	// TODO: Handle the tools definition.
	return &provider.Request{
		Model:        t.Model.Slug,
		Params:       t.Model.Parameters,
		OutputSchema: t.Output.Schema,
		Tools:        t.Capabilities.Tools,
	}
}
