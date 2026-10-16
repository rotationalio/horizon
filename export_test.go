package horizon

import (
	"go.rtnl.ai/horizon/actions"
	"go.rtnl.ai/horizon/task"
)

// NewProcessForTest exposes process execution without changing the public API.
func (h *Horizon) NewProcessForTest(runner task.Runner, input *task.Input, definition *task.Task) *process {
	return newProcess(h, runner, input, definition, h.config, nil)
}

// SetActionsForTest replaces the log after Run initializes the output. Call it
// from runner preparation, before the process records any actions.
func (p *process) SetActionsForTest(log *actions.Actions) {
	p.output.Actions = log
}
