package horizon

import (
	"time"

	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/x/mime"
)

type Output struct {
	Usage        Usage        `json:"usage"`
	Model        Model        `json:"model"`
	Output       any          `json:"output,omitempty"`
	MimeType     mime.Type    `json:"mime_type"`
	Capabilities Capabilities `json:"capabilities"`
	Actions      Actions      `json:"actions,omitempty"`
	Started      time.Time    `json:"started"`
	Finished     time.Time    `json:"finished"`
	Attachments  attachments.Attachments
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

//============================================================================
// Actions Log Definition
//============================================================================

// A collection of [Action]s taken during a task execution.
type Actions []*Action

// An action is a single action taken during a task execution, such as a message
// prompt or a capability (tool/resource) call.
type Action struct {
	Name   string `json:"name"`            // The name of the action (e.g. "tool call", "prompt", etc.)
	Target any    `json:"target"`          // Can be either a *Model or a *Capability
	Params JSON   `json:"params"`          // The parameters sent to the target
	Output JSON   `json:"output"`          // The output response of the action
	Usage  *Usage `json:"usage,omitempty"` // Usage information for the action
}
