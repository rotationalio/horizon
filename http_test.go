package horizon_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon"
)

type mockRunner struct {
	OnPrepare  func(ctx context.Context, task *horizon.Task) error
	OnFinalize func(ctx context.Context, output *horizon.Output) error
}

func (r *mockRunner) Prepare(ctx context.Context, task *horizon.Task) error {
	if r.OnPrepare != nil {
		return r.OnPrepare(ctx, task)
	}
	return nil
}

func (r *mockRunner) Finalize(ctx context.Context, output *horizon.Output) error {
	if r.OnFinalize != nil {
		return r.OnFinalize(ctx, output)
	}
	return nil
}

// Test making requests to the HTTP handler
func TestTaskHandler(t *testing.T) {
	task := &horizon.Task{
		Output:       &horizon.TaskOutput{},
		Capabilities: &horizon.Capabilities{},
	}

	t.Run("Get", func(t *testing.T) {
		runner := &mockRunner{
			OnFinalize: func(ctx context.Context, output *horizon.Output) error {
				output.Output = "Hello, world!"
				return nil
			},
		}
		handler := horizon.NewTaskHandler(task, runner)
		request, err := http.NewRequest("GET", "?context=value", nil)
		require.NoError(t, err)
		response := httptest.NewRecorder()

		// Handle the request
		handler.Handle(response, request)
		require.Equal(t, http.StatusOK, response.Code)
		var output horizon.Output
		err = json.NewDecoder(response.Body).Decode(&output)
		require.NoError(t, err)
		require.Equal(t, "Hello, world!", output.Output)
	})
}
