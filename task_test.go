package horizon

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/x/mime"
)

// TestTaskRequest verifies that the root task model is translated into the
// provider request contract without losing model parameters or output schema
// metadata.
func TestTaskRequest(t *testing.T) {
	task := &Task{
		Model: Model{
			Slug:       "test-model",
			Parameters: params.New(map[string]any{"temperature": 0.2}),
		},
		Output: &TaskOutput{
			Schema: &schema.Schema{
				Name:        "result",
				Description: "the result",
				MimeType:    mime.ApplicationSchemaJSON,
				Data:        `{"type":"object"}`,
			},
		},
	}

	request := task.request()
	require.Equal(t, "test-model", request.Model)
	require.Same(t, task.Model.Parameters, request.Params)
	require.NotNil(t, request.OutputSchema)
	require.Equal(t, "result", request.OutputSchema.Name)
	require.Equal(t, "the result", request.OutputSchema.Description)
	require.Equal(t, `{"type":"object"}`, request.OutputSchema.Data)
}
