package task

import (
	"slices"

	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/semver"
)

//============================================================================
// Detailed Task Data Definitions
//============================================================================

// A collection of [Task]s.
type Tasks []Task

// A single, independent unit of work that can be executed, including all of the
// necessary inputs, outputs, clients, capabilities, and other configuration.
type Task struct {
	// Task Definition
	ID           ulid.ULID           `json:"id,omitempty" yaml:"id,omitempty" msg:"id,omitempty"`                               // The ID of the task
	Slug         string              `json:"slug" yaml:"slug" msg:"slug"`                                                       // The slug of the task
	Name         string              `json:"name" yaml:"name" msg:"name"`                                                       // The name of the task
	Description  string              `json:"description,omitempty" yaml:"description,omitempty" msg:"description,omitempty"`    // The description of the task
	Version      semver.Version      `json:"version" yaml:"version" msg:"version"`                                              // The version of the task
	Input        *schema.Input       `json:"input,omitempty" yaml:"input,omitempty" msg:"input,omitempty"`                      // The input schema of the task
	Output       *schema.Output      `json:"output" yaml:"output" msg:"output"`                                                 // The output type of the task
	Model        Model               `json:"model" yaml:"model" msg:"model"`                                                    // The Model used for the task
	Prompts      prompts.Templates   `json:"prompts" yaml:"prompts" msg:"prompts"`                                              // The prompts used for the task
	Capabilities []capabilities.Name `json:"capabilities,omitempty" yaml:"capabilities,omitempty" msg:"capabilities,omitempty"` // The capabilities enabled for the task

	// Task Execution Configuration
	Provider *provider.Config `json:"provider,omitempty" yaml:"provider,omitempty" msg:"provider,omitempty"` // The inference provider used for the task
}

// Defines the model used for a task by its slug and model parameters. The model is
// converted into the correct identifying LLM or VLM on provider request.
type Model struct {
	Slug       string         `json:"slug" yaml:"slug" msg:"slug"`
	Parameters *params.Params `json:"parameters,omitempty" yaml:"parameters,omitempty" msg:"parameters,omitempty"`
}

// Clone copies a task's mutable configuration for an execution.
func (t *Task) Clone() *Task {
	if t == nil {
		return nil
	}
	cloned := *t
	cloned.Capabilities = slices.Clone(t.Capabilities)
	cloned.Model.Parameters = t.Model.Parameters.Clone()
	if t.Provider != nil {
		config := *t.Provider
		cloned.Provider = &config
	}
	if t.Input != nil {
		input := *t.Input
		cloned.Input = &input
		if t.Input.Schema != nil {
			schema := *t.Input.Schema
			input.Schema = &schema
		}
	}
	if t.Output != nil {
		output := *t.Output
		cloned.Output = &output
		if t.Output.Schema != nil {
			schema := *t.Output.Schema
			output.Schema = &schema
		}
	}
	if t.Prompts != nil {
		cloned.Prompts = make(prompts.Templates, len(t.Prompts))
		for i, template := range t.Prompts {
			if template != nil {
				copy := *template
				cloned.Prompts[i] = &copy
			}
		}
	}
	return &cloned
}
