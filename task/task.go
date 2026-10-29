package task

import (
	"fmt"
	"slices"

	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/metadata"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts/templates"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/semver"
	"go.rtnl.ai/x/validation"
)

//============================================================================
// Detailed Task Data Definitions
//============================================================================

// A collection of [Task]s.
type Tasks []Task

// A single, independent unit of work that can be executed, including all of the
// necessary inputs, outputs, clients, capabilities, and other configuration.
type Task struct {
	ID           ulid.ULID           `json:"id,omitempty" yaml:"id,omitempty" msg:"id,omitempty"`                               // The ID of the task
	Slug         string              `json:"slug" yaml:"slug" msg:"slug"`                                                       // The slug of the task
	Name         string              `json:"name" yaml:"name" msg:"name"`                                                       // The name of the task
	Description  string              `json:"description,omitempty" yaml:"description,omitempty" msg:"description,omitempty"`    // The description of the task
	Version      semver.Version      `json:"version" yaml:"version" msg:"version"`                                              // The version of the task
	Input        *schema.Input       `json:"input,omitempty" yaml:"input,omitempty" msg:"input,omitempty"`                      // The input schema of the task
	Output       *schema.Output      `json:"output" yaml:"output" msg:"output"`                                                 // The output type of the task
	Model        Model               `json:"model" yaml:"model" msg:"model"`                                                    // The Model used for the task
	Prompts      templates.Templates `json:"prompts" yaml:"prompts" msg:"prompts"`                                              // The prompts used for the task
	Capabilities []capabilities.Name `json:"capabilities,omitempty" yaml:"capabilities,omitempty" msg:"capabilities,omitempty"` // The capabilities enabled for the task
	Metadata     metadata.Metadata   `json:"metadata,omitempty" yaml:"metadata,omitempty" msg:"metadata,omitempty"`             // Application-specific metadata for the task

	// Task Execution Configuration
	Provider *provider.Config `json:"provider,omitempty" yaml:"provider,omitempty" msg:"provider,omitempty"` // The inference provider used for the task
}

// Defines the model used for a task by its slug and model parameters. The model is
// converted into the correct identifying LLM or VLM on provider request.
type Model struct {
	Slug       string         `json:"slug" yaml:"slug" msg:"slug"`
	Parameters *params.Params `json:"parameters,omitempty" yaml:"parameters,omitempty" msg:"parameters,omitempty"`
}

// Performs static validation of the task.
func (t *Task) Validate() (err error) {
	if t.Provider == nil {
		return errors.ErrProviderRequired
	}

	if t.Input != nil {
		if missing := t.Input.Modality &^ t.Provider.APIType.InputModality(); missing != 0 {
			err = validation.Error(err, validation.Incorrect("input.modality", fmt.Sprintf("input modality %s is not supported by the provider", missing.String())))
		}
	}

	if t.Output != nil {
		if missing := t.Output.Modality &^ t.Provider.APIType.OutputModality(); missing != 0 {
			err = validation.Error(err, validation.Incorrect("output.modality", fmt.Sprintf("output modality %s is not supported by the provider", missing.String())))
		}
	}

	return err
}

// Clone copies a task's mutable configuration for an execution.
func (t *Task) Clone() *Task {
	if t == nil {
		return nil
	}
	cloned := *t
	cloned.Capabilities = slices.Clone(t.Capabilities)
	cloned.Metadata = t.Metadata.Clone()
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
		cloned.Prompts = make(templates.Templates, len(t.Prompts))
		for i, template := range t.Prompts {
			if template != nil {
				copy := *template
				cloned.Prompts[i] = &copy
			}
		}
	}
	return &cloned
}
