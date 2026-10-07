package horizon_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"go.rtnl.ai/horizon"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	mockprovider "go.rtnl.ai/horizon/provider/mock"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/mime"
)

// Verifies that a successful inference is guarded, accounted, and finalized in order.
func TestProcessRunSuccessfulInference(t *testing.T) {
	inference := mockprovider.New(ulid.Make())
	runner := &processTestRunner{}
	inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
		runner.events = append(runner.events, "generate")
		require.Equal(t, "requested-model", request.Model)
		return processTestResponse(), nil
	}
	runner.protectOutput = func(response *provider.Response) error {
		response.Output[0].Content = "guarded answer"
		return nil
	}

	output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{})

	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, "guarded answer", output.Output)
	require.Equal(t, mime.TextPlain, output.MimeType)
	require.Equal(t, "served-model", output.Model.Slug)
	require.Equal(t, uint64(1), output.Usage.Invocations)
	require.Equal(t, int64(11), output.Usage.InputTokens)
	require.Equal(t, int64(7), output.Usage.OutputTokens)
	require.Equal(t, int64(18), output.Usage.TotalTokens)
	require.Equal(t, 0.25, output.Usage.APICost)
	require.Equal(t, task.OutcomeSucceeded, output.Outcome)
	require.False(t, output.Started.IsZero())
	require.False(t, output.Finished.IsZero())
	require.Equal(t, []string{"prepare", "input guard", "generate", "output guard", "finalize"}, runner.events)
	require.Equal(t, 1, inference.Calls(t, mockprovider.Generate))
}

// A finalization failure updates a successful execution's returned outcome.
func TestProcessRunFinalizeFailureMarksOutputFailed(t *testing.T) {
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		return processTestResponse(), nil
	}
	runner := &processTestRunner{
		finalizeErr: errors.New("finalize failed"),
		finalize: func(_ context.Context, output *task.Output, executionErr error) error {
			require.NoError(t, executionErr)
			require.Equal(t, task.OutcomeSucceeded, output.Outcome)
			return nil
		},
	}

	output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{})

	require.ErrorIs(t, err, runner.finalizeErr)
	require.Equal(t, task.OutcomeFailed, output.Outcome)
}

// Ensures a provider failure retains returned partial output and usage for the host.
func TestProcessRunReturnsPartialOutputOnInferenceFailure(t *testing.T) {
	providerErr := errors.New("provider failed after producing a partial response")
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		return processTestResponse(), providerErr
	}
	runner := &processTestRunner{}

	output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{})

	require.ErrorIs(t, err, providerErr)
	require.NotNil(t, output)
	require.Equal(t, "answer", output.Output)
	require.Equal(t, uint64(1), output.Usage.Invocations)
	require.Equal(t, int64(18), output.Usage.TotalTokens)
	require.Equal(t, task.OutcomeFailed, output.Outcome)
	require.Equal(t, []string{"prepare", "input guard", "output guard", "finalize"}, runner.events)
}

// Confirms that preparation and finalization failures are both returned without
// discarding the execution output.
func TestProcessRunPreservesPreparationAndFinalizationFailures(t *testing.T) {
	prepareErr := errors.New("prepare failed")
	finalizeErr := errors.New("finalize failed")
	runner := &processTestRunner{
		prepareErr:  prepareErr,
		finalizeErr: finalizeErr,
		finalize: func(_ context.Context, output *task.Output, executionErr error) error {
			require.ErrorIs(t, executionErr, prepareErr)
			require.Equal(t, task.OutcomeFailed, output.Outcome)
			return nil
		},
	}
	output, err := runProcessTest(t, context.Background(), mockprovider.New(ulid.Make()), runner, &schema.Output{})

	require.ErrorIs(t, err, prepareErr)
	require.ErrorIs(t, err, finalizeErr)
	require.NotNil(t, output)
	require.False(t, output.Started.IsZero())
	require.False(t, output.Finished.IsZero())
	require.Equal(t, task.OutcomeFailed, output.Outcome)
	require.Equal(t, []string{"prepare", "finalize"}, runner.events)
}

// Ensures a rejected request never reaches the provider and still reaches finalization.
func TestProcessRunStopsWhenInputGuardRejects(t *testing.T) {
	guardErr := errors.New("request rejected")
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		require.FailNow(t, "provider must not be called when input is rejected")
		return nil, nil
	}
	runner := &processTestRunner{
		protectInput: func(*provider.Request) error { return guardErr },
	}

	output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{})

	require.ErrorIs(t, err, guardErr)
	require.NotNil(t, output)
	require.Zero(t, output.Usage.Invocations)
	require.Equal(t, 0, inference.Calls(t, mockprovider.Generate))
	require.Equal(t, []string{"prepare", "input guard", "finalize"}, runner.events)
}

// Ensures output-guard failures block model content while retaining provider usage.
func TestProcessRunStopsWhenOutputGuardRejects(t *testing.T) {
	guardErr := errors.New("response rejected")
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		return processTestResponse(), nil
	}
	runner := &processTestRunner{
		protectOutput: func(*provider.Response) error { return guardErr },
	}

	output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{})

	require.ErrorIs(t, err, guardErr)
	require.NotNil(t, output)
	require.Nil(t, output.Output)
	require.Equal(t, uint64(1), output.Usage.Invocations)
	require.Equal(t, int64(18), output.Usage.TotalTokens)
	require.Equal(t, []string{"prepare", "input guard", "output guard", "finalize"}, runner.events)
}

// Confirms cancellation after provider work preserves usage but discards the response.
func TestProcessRunDiscardsOutputWhenCancelledDuringInference(t *testing.T) {
	type contextKey struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "retained"))
	defer cancel()
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		cancel()
		return processTestResponse(), nil
	}
	runner := &processTestRunner{
		finalize: func(finalizeCtx context.Context, output *task.Output, executionErr error) error {
			require.ErrorIs(t, executionErr, context.Canceled)
			require.NoError(t, finalizeCtx.Err())
			_, ok := finalizeCtx.Deadline()
			require.True(t, ok)
			require.Equal(t, "retained", finalizeCtx.Value(contextKey{}))
			require.Equal(t, task.OutcomeCancelled, output.Outcome)
			return nil
		},
	}

	output, err := runProcessTest(t, ctx, inference, runner, &schema.Output{})

	require.ErrorIs(t, err, context.Canceled)
	require.NotNil(t, output)
	require.Nil(t, output.Output)
	require.Equal(t, uint64(1), output.Usage.Invocations)
	require.Equal(t, int64(18), output.Usage.TotalTokens)
	require.Equal(t, task.OutcomeCancelled, output.Outcome)
	require.Equal(t, []string{"prepare", "input guard", "finalize"}, runner.events)
}

// Selects the provider's final-answer phase instead of returning commentary as task output.
func TestProcessRunUsesFinalAnswerPhase(t *testing.T) {
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		response := processTestResponse()
		response.Output = prompts.Prompts{
			{Role: prompts.RoleAssistant, Phase: "commentary", Content: "commentary"},
			{Role: prompts.RoleAssistant, Phase: "final_answer", Content: "final"},
		}
		return response, nil
	}

	output, err := runProcessTest(t, context.Background(), inference, &processTestRunner{}, &schema.Output{})

	require.NoError(t, err)
	require.Equal(t, "final", output.Output)
}

// Rejects malformed structured output rather than returning invalid JSON as a result.
func TestProcessRunRejectsMalformedJSONOutput(t *testing.T) {
	inference := mockprovider.New(ulid.Make())
	inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		response := processTestResponse()
		response.Output[0].Content = `{"answer":`
		return response, nil
	}
	runner := &processTestRunner{}
	output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{Schema: &schema.Schema{MimeType: mime.ApplicationSchemaJSON}})

	require.ErrorIs(t, err, errors.ErrInvalidModelOutput)
	require.NotNil(t, output)
	require.Nil(t, output.Output)
	require.Equal(t, uint64(1), output.Usage.Invocations)
}

// Validates required execution inputs before invoking runner lifecycle hooks.
func TestProcessRunRequiresTaskRunnerAndOutput(t *testing.T) {
	tests := []struct {
		name string
		task *task.Task
		run  task.Runner
		want error
	}{
		{
			name: "task",
			run:  &processTestRunner{},
			want: errors.ErrTaskRequired,
		},
		{
			name: "runner",
			task: &task.Task{Output: &schema.Output{}},
			want: errors.ErrRunnerRequired,
		},
		{
			name: "output",
			task: &task.Task{},
			run:  &processTestRunner{},
			want: errors.ErrTaskOutputRequired,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := horizon.Run(context.Background(), nil, test.task, test.run)
			require.ErrorIs(t, err, test.want)
		})
	}
}

// Confirms that the task config constructs a provider through the public API.
func TestProcessPrepareBuildsConfiguredProvider(t *testing.T) {
	config := inferenceConfig(t)
	h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) { return &processTestRunner{}, nil }))
	output, err := h.Run(context.Background(), nil, &task.Task{Provider: &config, Output: &schema.Output{}})
	require.NoError(t, err)
	require.Equal(t, "generated", output.Output)
}

// Reports missing provider configuration as a setup error.
func TestProcessPrepareRequiresProviderConfiguration(t *testing.T) {
	_, err := horizon.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}}, &processTestRunner{})
	require.ErrorIs(t, err, errors.ErrProviderRequired)
}

// Concurrent executions keep prepared tasks, requests, guards, and outputs isolated.
func TestProcessConcurrentIsolation(t *testing.T) {
	const runs = 12
	type executionKey struct{}

	config := testProviderConfig()
	inference := mockprovider.New(config.ID)
	inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
		return &provider.Response{
			Model: request.Model,
			Output: prompts.Prompts{{
				Role:    prompts.RoleAssistant,
				Content: request.Model,
			}},
		}, nil
	}

	original := &task.Task{
		Provider: &config,
		Model:    task.Model{Slug: "shared-model"},
		Output:   &schema.Output{},
	}
	h := newTestHorizon(t, testFactory(func(ctx context.Context) (task.Runner, error) {
		executionID, ok := ctx.Value(executionKey{}).(string)
		if !ok {
			return nil, fmt.Errorf("execution ID missing from runner context")
		}
		runner := &processTestRunner{}
		runner.prepare = func(_ context.Context, prepared *task.Task) error {
			if prepared.Provider == original.Provider {
				return fmt.Errorf("execution reused the original provider config")
			}
			prepared.Model.Slug = executionID
			return nil
		}
		runner.protectInput = func(request *provider.Request) error {
			if request.Model != executionID {
				return fmt.Errorf("input guard got model %q, want %q", request.Model, executionID)
			}
			return nil
		}
		runner.protectOutput = func(response *provider.Response) error {
			if response.Output[0].Content != executionID {
				return fmt.Errorf("output guard got response %q, want %q", response.Output[0].Content, executionID)
			}
			return nil
		}
		return runner, nil
	}))
	_, err := h.GetOrCreateProvider(config, inference)
	require.NoError(t, err)

	type result struct {
		id     string
		output *task.Output
		err    error
	}
	results := make(chan result, runs)
	for i := range runs {
		id := fmt.Sprintf("run-%d", i)
		go func() {
			ctx := context.WithValue(context.Background(), executionKey{}, id)
			output, err := h.Run(ctx, &task.Input{}, original)
			results <- result{id: id, output: output, err: err}
		}()
	}

	for range runs {
		got := <-results
		require.NoError(t, got.err)
		require.Equal(t, got.id, got.output.Output)
		require.Equal(t, got.id, got.output.Model.Slug)
		require.Equal(t, task.OutcomeSucceeded, got.output.Outcome)
	}
	require.Equal(t, "shared-model", original.Model.Slug)
	require.Same(t, &config, original.Provider)
}

// Holds configurable host hooks for exercising one execution lifecycle.
type processTestRunner struct {
	events        []string
	prepare       func(context.Context, *task.Task) error
	prepareErr    error
	finalizeErr   error
	protectInput  func(*provider.Request) error
	protectOutput func(*provider.Response) error
	finalize      func(context.Context, *task.Output, error) error
}

func (r *processTestRunner) Prepare(ctx context.Context, prepared *task.Task) error {
	r.events = append(r.events, "prepare")
	if r.prepare != nil {
		if err := r.prepare(ctx, prepared); err != nil {
			return err
		}
	}
	return r.prepareErr
}

func (r *processTestRunner) Finalize(ctx context.Context, output *task.Output, executionErr error) error {
	r.events = append(r.events, "finalize")
	if r.finalize != nil {
		if err := r.finalize(ctx, output, executionErr); err != nil {
			return err
		}
	}
	return r.finalizeErr
}

func (r *processTestRunner) ProtectInput(request *provider.Request) error {
	r.events = append(r.events, "input guard")
	if r.protectInput != nil {
		return r.protectInput(request)
	}
	return nil
}

func (r *processTestRunner) ProtectOutput(response *provider.Response) error {
	r.events = append(r.events, "output guard")
	if r.protectOutput != nil {
		return r.protectOutput(response)
	}
	return nil
}

func runProcessTest(t *testing.T, ctx context.Context, inference provider.Provider, runner *processTestRunner, output *schema.Output) (*task.Output, error) {
	t.Helper()
	config := testProviderConfig()
	config.ID = inference.ID()
	h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) { return runner, nil }))
	_, err := h.GetOrCreateProvider(config, inference)
	require.NoError(t, err)
	return h.Run(ctx, &task.Input{}, &task.Task{
		Provider: &config,
		Model:    task.Model{Slug: "requested-model"},
		Output:   output,
	})
}

func processTestResponse() *provider.Response {
	return &provider.Response{
		Model: "served-model",
		Usage: provider.Usage{
			InputTokens:  11,
			OutputTokens: 7,
			TotalTokens:  18,
			APICost:      0.25,
		},
		Output: prompts.Prompts{{
			Role:    prompts.RoleAssistant,
			Type:    prompts.TypeMessage,
			Content: "answer",
		}},
	}
}
