package horizon

import (
	"time"

	"go.rtnl.ai/x/mime"
)

type Output struct {
	Usage        Usage
	Model        Model
	Output       any
	MimeType     mime.Type
	Capabilities Capabilities
	Actions      Actions
	Started      time.Time
	Finished     time.Time
}

// Usage is the model and capability usage information for a task execution.
type Usage struct {
	Invocations  uint64  // Number of inference calls in the task execution.
	ToolCalls    uint64  // Number of capability tool calls in the task execution.
	InputTokens  int64   // Total input tokens reported by inference.
	OutputTokens int64   // Total output tokens reported by inference.
	TotalTokens  int64   // Total tokens reported by inference.
	APICost      float64 // Price in US dollars for the API call.
	EnergyCost   float64 // Energy cost in US dollars for the API call.
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
	Name   string // The name of the action (e.g. "tool call", "prompt", etc.)
	Target any    // Can be either a *Model or a *Capability
	Params JSON   // The parameters sent to the target
	Output JSON   // The output response of the action
	Usage  *Usage // Usage information for the action
}
