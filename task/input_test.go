package task_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/task"
)

// Verifies execution copies isolate input context entries and attachment bytes.
func TestInputClone(t *testing.T) {
	t.Run("mutable input", func(t *testing.T) {
		original := &task.Input{
			Context:     prompts.Context{"context": "original"},
			Attachments: attachments.Attachments{nil, {Filename: "original.txt", Data: []byte("original")}},
		}
		cloned := original.Clone()
		require.Equal(t, original, cloned)
		cloned.Context["context"] = "changed"
		cloned.Attachments[1].Filename = "changed.txt"
		cloned.Attachments[1].Data[0] = 'X'
		require.Equal(t, "original", original.Context["context"])
		require.Equal(t, "original.txt", original.Attachments[1].Filename)
		require.Equal(t, []byte("original"), original.Attachments[1].Data)
	})

	t.Run("nil input", func(t *testing.T) {
		var input *task.Input
		require.Equal(t, &task.Input{}, input.Clone())
	})

	t.Run("empty input", func(t *testing.T) {
		input := &task.Input{}
		require.Equal(t, input, input.Clone())
		require.NotSame(t, input, input.Clone())
	})
}
