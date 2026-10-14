package task

import (
	"time"

	"go.rtnl.ai/horizon/actions"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/x/mime"
)

// Outcome is the terminal result of a task execution.
type Outcome string

const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
	OutcomeCancelled Outcome = "cancelled"
)

type Output struct {
	Usage        Usage               `json:"usage"`
	Model        Model               `json:"model"`
	Output       any                 `json:"output,omitempty"` // Final or partial model output; Run may return it with an error.
	MimeType     mime.Type           `json:"mime_type"`
	Capabilities []capabilities.Name `json:"capabilities"`
	Actions      actions.Actions     `json:"actions,omitempty"` // Log of important actions.
	Started      time.Time           `json:"started"`
	Finished     time.Time           `json:"finished"`
	Outcome      Outcome             `json:"outcome"`
	Attachments  attachments.Attachments
}

// Messages returns the conversation view of the execution's action log.
func (o *Output) Messages() prompts.Prompts {
	// TODO: apply configured transcript filters once action recording is implemented.
	return o.Actions.Messages()
}

// Usage is the model and capability usage information for a task execution.
type Usage struct {
	Invocations  uint64  `json:"invocations"`   // Number of inference calls in the task execution.
	ToolCalls    uint64  `json:"tool_calls"`    // Number of capability tool calls in the task execution.
	InputTokens  int64   `json:"input_tokens"`  // Total input tokens reported by inference.
	OutputTokens int64   `json:"output_tokens"` // Total output tokens reported by inference.
	TotalTokens  int64   `json:"total_tokens"`  // Total tokens reported by inference.
	APICost      float64 `json:"api_cost"`      // Price in US dollars for the API call.
	EnergyCost   float64 `json:"energy_cost"`   // Energy cost in US dollars for the API call.
}

// Returns the duration of the task execution.
func (o *Output) Latency() time.Duration {
	if o.Started.IsZero() || o.Finished.IsZero() {
		return 0
	}
	return o.Finished.Sub(o.Started)
}
