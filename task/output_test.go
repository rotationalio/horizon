package task_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/actions"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/task"
)

// Messages returns the recorded conversation, or nil when no log is available.
func TestOutputMessages(t *testing.T) {
	messages := prompts.Prompts{{
		Role: prompts.RoleUser, Type: prompts.TypeMessage, Content: "question",
	}}
	log := actions.New()
	log.AddMessage(t.Context(), actions.EntityUser, actions.EntityRunner, messages)

	for _, tc := range []struct {
		name   string
		output *task.Output
		want   prompts.Prompts
	}{
		{
			name: "nil output",
		},
		{
			name:   "missing log",
			output: &task.Output{},
		},
		{
			name:   "empty log",
			output: &task.Output{Actions: actions.New()},
		},
		{
			name:   "recorded messages",
			output: &task.Output{Actions: log},
			want:   messages,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.output.Messages())
		})
	}
}
