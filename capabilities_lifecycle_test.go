package horizon_test

//=============================================================================
// Tests when the capabilities loop stops and what it retains after cancellation,
// turn limits, or failures, including failures after a tool has already executed.
//=============================================================================

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.rtnl.ai/horizon/attachments"
	"go.rtnl.ai/horizon/capabilities"
	"go.rtnl.ai/horizon/config"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/params"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	mockprovider "go.rtnl.ai/horizon/provider/mock"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/horizon/task/mock"
	"go.rtnl.ai/ulid"
	"go.rtnl.ai/x/mime"
)

// Runs one successful execution with two tool-call rounds and a final answer,
// followed by a simple failure case that checks the outer span's error status.
// Verifies span counts, parentage, lifetimes, and the recorded successful outcome.
// Generation and tool execution belong to the loop, and work inside the provider
// and executor inherits the corresponding operation's context rather than
// becoming a root.
func TestCapabilitiesTracing(t *testing.T) {
	// Setup: record real SDK spans locally. This test cannot run in parallel because
	// Horizon uses the global tracer provider; restore it when the test finishes.
	recorder := tracetest.NewSpanRecorder()
	tracing := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(tracing)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		require.NoError(t, tracing.Shutdown(context.Background()))
	})

	// Check exact counts, not just span presence: neither turns nor calls should
	// share an operation span, and successful execution must not retry anything.
	wantCounts := map[string]int{
		"test.request":             1,
		"horizon.run":              1,
		"task.execute":             1,
		"task.prepare":             1,
		"task.process_input":       1,
		"task.process_context":     1,
		"task.process_attachments": 1,
		"task.render":              1,
		"task.capabilities_loop":   1,
		"capabilities.lookup":      1,
		"guard.input":              3,
		"provider.generate":        3,
		"guard.output":             3,
		"capabilities.execute":     2,
		"test.provider":            3,
		"test.tool":                2,
	}
	wantParents := map[string]string{
		"horizon.run":              "test.request",
		"task.execute":             "horizon.run",
		"task.prepare":             "task.execute",
		"task.process_input":       "task.execute",
		"task.process_context":     "task.execute",
		"task.process_attachments": "task.execute",
		"task.render":              "task.execute",
		"task.capabilities_loop":   "task.execute",
		"capabilities.lookup":      "task.capabilities_loop",
		"guard.input":              "task.capabilities_loop",
		"provider.generate":        "task.capabilities_loop",
		"guard.output":             "task.capabilities_loop",
		"capabilities.execute":     "task.capabilities_loop",
		"test.provider":            "provider.generate",
		"test.tool":                "capabilities.execute",
	}
	t.Run("successful run", func(t *testing.T) {
		// Setup: script two sequential tool requests followed by a final answer.
		// The synthetic child spans prove context reaches host/provider code.
		runner := newCapabilityTestRunner("lookup")
		runner.CapabilityRunner.OnExecuteCapability = func(ctx context.Context, request capabilities.Request) (capabilities.Response, *capabilities.Error) {
			_, span := otel.Tracer("horizon.test").Start(ctx, "test.tool")
			defer span.End()

			return capabilities.NewToolTextContentResponse("result " + request.CallID), nil
		}
		turn := 0
		inference := mockprovider.New(ulid.Make())
		inference.OnGenerate = func(ctx context.Context, _ *provider.Request) (*provider.Response, error) {
			_, span := otel.Tracer("horizon.test").Start(ctx, "test.provider")
			defer span.End()
			turn++

			if turn <= 2 {
				return capabilityTestResponse(prompts.ToolCall{
					CallID: fmt.Sprintf("call-%d", turn), Name: "lookup", Arguments: `{}`,
				}), nil
			}

			return processTestResponse(), nil
		}
		var finalizeContext trace.SpanContext
		runner.Runner.OnFinalize = func(ctx context.Context, _ *task.Output, _ error) error {
			finalizeContext = trace.SpanContextFromContext(ctx)
			return nil
		}

		// Execute under a caller span, then end it so every span is available
		// synchronously in the recorder.
		before := len(recorder.Ended())
		ctx, root := tracing.Tracer("horizon.test").Start(context.Background(), "test.request")
		_, err := runProcessTest(t, ctx, inference, runner, &schema.Output{})
		root.End()

		require.NoError(t, err)

		// Index finished spans by ID so parentage is verified by identity, not
		// just names. Every span must be unique, ended, and free of error status.
		spans := recorder.Ended()[before:]
		counts := make(map[string]int)
		byID := make(map[trace.SpanID]sdktrace.ReadOnlySpan)
		traceID := root.SpanContext().TraceID()
		for _, span := range spans {
			id := span.SpanContext().SpanID()
			require.True(t, span.SpanContext().IsValid(), span.Name())
			require.Equal(t, traceID, span.SpanContext().TraceID(), span.Name())
			require.NotContains(t, byID, id, "span IDs must not be reused")
			byID[id] = span
			counts[span.Name()]++
			require.NotEqual(t, codes.Error, span.Status().Code, span.Name())
			require.False(t, span.EndTime().IsZero(), span.Name())
		}
		require.Equal(t, wantCounts, counts)
		require.Len(t, recorder.Started(), len(recorder.Ended()), "no spans may remain open")

		// Each child must belong to the expected operation and finish within
		// its parent's lifetime. Only the caller span is allowed to be a root.
		for _, span := range spans {
			if span.Name() == "test.request" {
				require.False(t, span.Parent().IsValid())
				continue
			}
			parent, ok := byID[span.Parent().SpanID()]
			require.True(t, ok, "%s must have a recorded parent", span.Name())
			require.Equal(t, wantParents[span.Name()], parent.Name(), span.Name())
			require.False(t, span.StartTime().Before(parent.StartTime()), span.Name())
			require.False(t, span.EndTime().After(parent.EndTime()), span.Name())
			if span.Name() == "capabilities.execute" {
				require.Contains(t, span.Attributes(), attribute.String("capabilities.name", "lookup"))
			}
			if span.Name() == "task.execute" {
				require.Equal(t, span.SpanContext(), finalizeContext, "finalization must retain the task span")

				// The task span must explicitly record a successful outcome,
				// rather than merely omitting an error status.
				finalizingEvents := 0
				for _, event := range span.Events() {
					if event.Name == "task.finalizing" {
						finalizingEvents++
						require.Contains(t, event.Attributes, attribute.String("task.outcome", string(task.OutcomeSucceeded)))
					}
				}
				require.Equal(t, 1, finalizingEvents)
			}
		}
	})

	t.Run("provider failure marks outer span as errored", func(t *testing.T) {
		// Setup: make the first inference fail and record only this run's spans.
		before := len(recorder.Ended())
		providerErr := errors.New("inference failed")
		inference := mockprovider.New(ulid.Make())
		inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
			return nil, providerErr
		}

		// Execute through the capabilities path.
		_, err := runProcessTest(t, context.Background(), inference, newCapabilityTestRunner("lookup"), &schema.Output{})
		require.ErrorIs(t, err, providerErr)

		// Assert the outer span reports the failure.
		for _, span := range recorder.Ended()[before:] {
			if span.Name() == "horizon.run" {
				require.Equal(t, codes.Error, span.Status().Code)
				return
			}
		}
		t.Fatal("horizon.run span was not recorded")
	})
}

// Stops the loop at several different failure points: cancellation, invalid call
// correlation, provider errors, missing output, and the turn limit. The assertions check
// that completed usage and any usable partial answer are retained when execution stops.
func TestCapabilitiesTermination(t *testing.T) {
	providerErr := errors.New("inference failed")
	tests := []struct {
		name                                                                                                               string
		cancelExecutor, executorError, cancelNextInput, repeatID, providerError, nilResponse, finalPartial, invalidPartial bool
		limit                                                                                                              bool
		want, wantAbsent                                                                                                   error
	}{
		{
			name:           "cancelled executor",
			cancelExecutor: true,
			want:           context.Canceled,
		},
		{
			name:           "cancelled executor with error",
			cancelExecutor: true,
			executorError:  true,
			want:           context.Canceled,
			wantAbsent:     providerErr,
		},
		{
			name:            "cancelled next input guard",
			cancelNextInput: true,
			want:            context.Canceled,
		},
		{
			name:     "duplicate ID on later turn",
			repeatID: true,
			want:     capabilities.ErrInvalidToolPrompt,
		},
		{
			name:  "budget exhausted",
			limit: true,
			want:  errors.ErrMaxToolTurns,
		},
		{
			name:          "provider error with calls",
			providerError: true,
			want:          providerErr,
		},
		{
			name:        "nil provider response",
			nilResponse: true,
			want:        errors.ErrNoModelOutput,
		},
		{
			name:          "partial final answer",
			providerError: true,
			finalPartial:  true,
			want:          providerErr,
		},
		{
			name:           "partial invalid protocol",
			providerError:  true,
			invalidPartial: true,
			want:           providerErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: terminate at a controlled boundary, retaining all earlier work.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runner := newCapabilityTestRunner("lookup")
			prepare := runner.Runner.OnPrepare
			runner.Runner.OnPrepare = func(ctx context.Context, definition *task.Task) error {
				if err := prepare(ctx, definition); err != nil {
					return err
				}
				if tt.limit {
					definition.Model.Parameters = params.New(map[string]any{params.MaxToolTurns: 1})
				}
				return nil
			}
			guardTurns := 0
			runner.InputGuard.OnProtectInput = func(*provider.Request) error {
				guardTurns++
				if tt.cancelNextInput && guardTurns == 2 {
					cancel()
				}
				return nil
			}
			runner.CapabilityRunner.OnExecuteCapability = func(_ context.Context, request capabilities.Request) (capabilities.Response, *capabilities.Error) {
				if tt.cancelExecutor {
					cancel()
				}
				if tt.executorError {
					return capabilities.Response{}, &capabilities.Error{Cause: providerErr}
				}
				return capabilities.NewToolTextContentResponse("done"), nil
			}
			turn := 0
			inference := mockprovider.New(ulid.Make())
			inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
				turn++
				if request.Params != nil {
					_, exists := request.Params.Get(params.MaxToolTurns)
					require.False(t, exists)
				}
				if tt.nilResponse {
					return nil, nil
				}
				if tt.finalPartial {
					return processTestResponse(), providerErr
				}
				if tt.invalidPartial {
					response := processTestResponse()
					response.Output = prompts.Prompts{{Role: prompts.RoleAssistant, Type: prompts.TypeToolCall}}
					return response, providerErr
				}
				id := fmt.Sprintf("call-%d", turn)
				if tt.repeatID {
					id = "same"
				}
				response := capabilityTestResponse(prompts.ToolCall{CallID: id, Name: "lookup", Arguments: `{}`})
				if tt.providerError {
					return response, providerErr
				}
				return response, nil
			}
			runner.Runner.OnFinalize = func(ctx context.Context, _ *task.Output, executionErr error) error {
				require.NoError(t, ctx.Err())
				require.ErrorIs(t, executionErr, tt.want)
				return nil
			}

			// Execute with a finite budget or cancellation, never a timing-based sleep.
			output, err := runProcessTest(t, ctx, inference, runner, &schema.Output{})

			// Assert termination causes, usage, and final-only content.
			require.ErrorIs(t, err, tt.want)
			if tt.wantAbsent != nil {
				require.NotErrorIs(t, err, tt.wantAbsent)
			}
			if tt.finalPartial {
				require.Equal(t, "answer", output.Output)
			} else {
				require.Nil(t, output.Output)
			}
			if tt.providerError || tt.nilResponse {
				runner.CapabilityRunner.AssertNotCalled(t, mock.ExecuteCapability)
			} else {
				runner.CapabilityRunner.AssertCalled(t, mock.ExecuteCapability, 1)
			}
			if tt.nilResponse {
				require.Zero(t, output.Usage.TotalTokens)
			} else {
				require.Equal(t, int64(turn*18), output.Usage.TotalTokens)
			}
			runner.Runner.AssertCalled(t, mock.Finalize, 1)
		})
	}
}

// Cancels as the first executor call returns, with both one-call and multi-call batches.
// The loop should stop before dispatching another call or starting a new inference turn.
func TestCapabilitiesCancellationBetweenCalls(t *testing.T) {
	for _, count := range []int{1, 2} {
		t.Run(fmt.Sprintf("batch of %d", count), func(t *testing.T) {
			// Setup: cancellation occurs as a completed result is contributed to history.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runner := newCapabilityTestRunner("lookup")
			runner.CapabilityRunner.OnExecuteCapability = func(_ context.Context, _ capabilities.Request) (capabilities.Response, *capabilities.Error) {
				cancel()
				return capabilities.NewToolTextContentResponse("completed"), nil
			}
			inference := mockprovider.New(ulid.Make())
			inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
				calls := []prompts.ToolCall{{CallID: "first", Name: "lookup", Arguments: `{}`}}
				if count == 2 {
					calls = append(calls, prompts.ToolCall{CallID: "second", Name: "lookup", Arguments: `{}`})
				}
				return capabilityTestResponse(calls...), nil
			}

			// Execute without sleeps; the first executor callback controls cancellation.
			output, err := runProcessTest(t, ctx, inference, runner, &schema.Output{})

			// Assert cancellation stops the batch before the result or another call is added.
			require.ErrorIs(t, err, context.Canceled)
			require.Equal(t, task.OutcomeCancelled, output.Outcome)
			require.Len(t, output.Messages, 1)
			runner.CapabilityRunner.AssertCalled(t, mock.ExecuteCapability, 1)
			inference.AssertCalled(t, mockprovider.Generate, 1)
		})
	}
}

// Completes a capability call and then makes a later inference or response-processing
// step fail. The test checks that the completed call's history, usage, and attachments
// remain available, while its side effect is not repeated and rejected text is not exposed.
func TestCapabilitiesLaterTurnFailures(t *testing.T) {
	providerErr := errors.New("later inference failure")
	guardErr := errors.New("later guard rejection")
	finalizeErr := errors.New("finalization failure")
	tests := []struct {
		name            string
		wantErrors      []error
		wantMessages    int
		wantInvocations uint64
		wantTokens      int64
		wantFiles       int
		wantOutput      any
	}{
		{
			name:            "nil later response",
			wantErrors:      []error{errors.ErrNoModelOutput},
			wantMessages:    2,
			wantInvocations: 2,
			wantTokens:      18,
			wantFiles:       1,
		},
		{
			name:            "provider failure with calls",
			wantErrors:      []error{providerErr},
			wantMessages:    3,
			wantInvocations: 2,
			wantTokens:      36,
			wantFiles:       2,
		},
		{
			name:            "provider failure with partial answer",
			wantErrors:      []error{providerErr},
			wantMessages:    3,
			wantInvocations: 2,
			wantTokens:      36,
			wantFiles:       2,
			wantOutput:      "answer",
		},
		{
			name:            "input guard rejects continuation",
			wantErrors:      []error{guardErr},
			wantMessages:    2,
			wantInvocations: 1,
			wantTokens:      18,
			wantFiles:       1,
		},
		{
			name:            "output guard rejects continuation",
			wantErrors:      []error{guardErr},
			wantMessages:    2,
			wantInvocations: 2,
			wantTokens:      36,
			wantFiles:       1,
		},
		{
			name:            "provider and output guard fail",
			wantErrors:      []error{providerErr, guardErr},
			wantMessages:    2,
			wantInvocations: 2,
			wantTokens:      36,
			wantFiles:       1,
		},
		{
			name:            "cancelled later provider",
			wantErrors:      []error{context.Canceled},
			wantMessages:    2,
			wantInvocations: 2,
			wantTokens:      36,
			wantFiles:       1,
		},
		{
			name:            "later request timeout",
			wantErrors:      []error{context.DeadlineExceeded},
			wantMessages:    2,
			wantInvocations: 2,
			wantTokens:      18,
			wantFiles:       1,
		},
		{
			name:            "commentary is not an answer",
			wantErrors:      []error{errors.ErrNoModelOutput},
			wantMessages:    3,
			wantInvocations: 2,
			wantTokens:      36,
			wantFiles:       1,
		},
		{
			name:            "invalid final JSON and provider error",
			wantErrors:      []error{providerErr, errors.ErrInvalidModelOutput},
			wantMessages:    3,
			wantInvocations: 2,
			wantTokens:      36,
			wantFiles:       2,
			wantOutput:      `{"answer":`,
		},
		{
			name:            "finalize fails after answer",
			wantErrors:      []error{finalizeErr},
			wantMessages:    3,
			wantInvocations: 2,
			wantTokens:      36,
			wantFiles:       2,
			wantOutput:      "answer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: complete one capability before injecting a later-stage failure.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runner := newCapabilityTestRunner("lookup")

			// Guard failures apply only to the continuation, after the first tool
			// has completed. Other cases leave both validation steps successful.
			inputTurns, outputTurns := 0, 0
			runner.InputGuard.OnProtectInput = func(request *provider.Request) error {
				inputTurns++
				if inputTurns == 2 {
					require.Len(t, request.Input, 2)
					require.Equal(t, "first", request.Input[1].ToolResults[0].CallID)
					if tt.name == "input guard rejects continuation" {
						return guardErr
					}
				}
				return nil
			}
			runner.OutputGuard.OnProtectOutput = func(*provider.Response) error {
				outputTurns++
				if outputTurns == 2 && (tt.name == "output guard rejects continuation" || tt.name == "provider and output guard fail") {
					return guardErr
				}
				return nil
			}

			// The first response always requests one tool and includes a file.
			// The second response introduces the selected failure or partial answer.
			inference := mockprovider.New(ulid.Make())
			turn := 0
			inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
				turn++
				if turn == 1 {
					response := capabilityTestResponse(prompts.ToolCall{CallID: "first", Name: "lookup", Arguments: `{}`})
					response.Attachments = attachments.Attachments{{Filename: "first.txt", Data: []byte("completed")}}
					return response, nil
				}
				require.Equal(t, 2, turn, "neither later failure nor finalization may retry completed calls")
				require.Equal(t, "first", request.Input[0].ToolCalls[0].CallID)
				require.Equal(t, "done", request.Input[1].ToolResults[0].Content)
				response := processTestResponse()
				response.Attachments = attachments.Attachments{{Filename: "later.txt", Data: []byte("later")}}
				switch tt.name {
				case "nil later response":
					return nil, nil
				case "later request timeout":
					return nil, context.DeadlineExceeded
				case "provider failure with calls":
					response.Output = capabilityTestResponse(prompts.ToolCall{CallID: "never_execute", Name: "lookup", Arguments: `{}`}).Output
					return response, providerErr
				case "provider failure with partial answer", "provider and output guard fail":
					return response, providerErr
				case "cancelled later provider":
					cancel()
				case "commentary is not an answer":
					response.Output[0].Phase = "commentary"
					response.Attachments = nil
				case "invalid final JSON and provider error":
					response.Output[0].Content = `{"answer":`
					return response, providerErr
				}
				return response, nil
			}

			// Cleanup must see the retained history even after cancellation. One
			// case also fails cleanup itself, after generation has already succeeded.
			runner.Runner.OnFinalize = func(finalizeCtx context.Context, output *task.Output, executionErr error) error {
				require.NoError(t, finalizeCtx.Err(), "cleanup remains usable after cancellation")
				require.Len(t, output.Messages, tt.wantMessages)
				if tt.name == "finalize fails after answer" {
					require.NoError(t, executionErr)
					return finalizeErr
				}
				for _, want := range tt.wantErrors {
					require.ErrorIs(t, executionErr, want)
				}
				return nil
			}
			outputSchema := &schema.Output{}
			if tt.name == "invalid final JSON and provider error" {
				outputSchema.Schema = &schema.Schema{MimeType: mime.ApplicationJSON}
			}

			// Execute through the normal lifecycle; only the first capability may run.
			output, err := runProcessTest(t, ctx, inference, runner, outputSchema)

			// Assert exact retained accounting, transcript, files, and independent causes.
			for _, want := range tt.wantErrors {
				require.ErrorIs(t, err, want)
			}
			require.Equal(t, tt.wantOutput, output.Output)
			require.Equal(t, tt.wantInvocations, output.Usage.Invocations)
			require.Equal(t, uint64(1), output.Usage.ToolCalls)
			require.Equal(t, tt.wantTokens, output.Usage.TotalTokens)
			require.Equal(t, float64(tt.wantTokens/18)*0.25, output.Usage.APICost)
			require.Len(t, output.Messages, tt.wantMessages)
			require.Len(t, output.Attachments, tt.wantFiles)
			require.Equal(t, "first.txt", output.Attachments[0].Filename)
			require.Equal(t, "first", output.Messages[1].ToolResults[0].CallID)
			runner.CapabilityRunner.AssertCalled(t, mock.ExecuteCapability, 1)
			runner.CapabilityRunner.AssertCalled(t, mock.LookupCapabilities, 1)
			runner.Runner.AssertCalled(t, mock.Finalize, 1)
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				require.Equal(t, task.OutcomeCancelled, output.Outcome)
			} else {
				require.Equal(t, task.OutcomeFailed, output.Outcome)
			}
		})
	}
}

// Verifies that the turn budget counts model responses containing calls, not the number
// of calls in each batch. Several calls can run in the last allowed batch, followed by a
// final answer; a task-level setting also takes precedence over the configured default.
func TestCapabilitiesBudgetBoundaries(t *testing.T) {
	original, err := config.Get()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, config.Set(original)) })
	tests := []struct {
		name         string
		configTurns  int64
		override     *int64
		batches      int
		wantCalls    int
		wantMessages int
		wantError    bool
	}{
		{
			name:         "final answer at exact limit",
			configTurns:  1,
			batches:      1,
			wantCalls:    3,
			wantMessages: 5,
		},
		{
			name:         "next batch exceeds default limit",
			configTurns:  1,
			batches:      2,
			wantCalls:    3,
			wantMessages: 5,
			wantError:    true,
		},
		{
			name:         "task increases allowance",
			configTurns:  1,
			override:     capabilityTurns(2),
			batches:      2,
			wantCalls:    6,
			wantMessages: 9,
		},
		{
			name:         "task decreases allowance",
			configTurns:  2,
			override:     capabilityTurns(1),
			batches:      2,
			wantCalls:    3,
			wantMessages: 5,
			wantError:    true,
		},
		{
			name:         "task enables tools over disabled default",
			configTurns:  0,
			override:     capabilityTurns(1),
			batches:      1,
			wantCalls:    3,
			wantMessages: 5,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: a known default budget and three calls in each requested batch.
			conf := original
			conf.MaxToolTurns = tt.configTurns
			require.NoError(t, config.Set(conf))
			runner := newCapabilityTestRunner("lookup")
			prepare := runner.Runner.OnPrepare
			runner.Runner.OnPrepare = func(ctx context.Context, definition *task.Task) error {
				if err := prepare(ctx, definition); err != nil {
					return err
				}
				if tt.override != nil {
					definition.Model.Parameters = params.New(map[string]any{params.MaxToolTurns: *tt.override})
				}
				return nil
			}
			turn := 0
			inference := mockprovider.New(ulid.Make())
			inference.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
				turn++
				if turn > tt.batches {
					return processTestResponse(), nil
				}
				calls := make([]prompts.ToolCall, 3)
				for i := range calls {
					calls[i] = prompts.ToolCall{CallID: fmt.Sprintf("batch-%d-call-%d", turn, i), Name: "lookup", Arguments: `{}`}
				}
				return capabilityTestResponse(calls...), nil
			}

			// Execute; call count must not consume the model-response turn allowance.
			output, err := runProcessTest(t, context.Background(), inference, runner, &schema.Output{})

			// Assert exact-boundary success or zero dispatch from an over-budget batch.
			if tt.wantError {
				require.ErrorIs(t, err, errors.ErrMaxToolTurns)
				require.Nil(t, output.Output)
			} else {
				require.NoError(t, err)
				require.Equal(t, "answer", output.Output)
			}
			require.Equal(t, uint64(tt.wantCalls), output.Usage.ToolCalls)
			require.Len(t, output.Messages, tt.wantMessages)
			runner.CapabilityRunner.AssertCalled(t, mock.ExecuteCapability, tt.wantCalls)
			if tt.override != nil {
				value, exists := output.Model.Parameters.MaxToolTurns()
				require.True(t, exists)
				require.Equal(t, *tt.override, value)
			}
		})
	}
}

// Mixes recoverable and terminal outcomes in a batch of three calls. Recoverable results
// let later calls run, but a terminal second result stops the third call and prevents the
// next inference, while keeping the first call's completed result in history.
func TestCapabilitiesMixedBatchOutcomes(t *testing.T) {
	cause := errors.New("private executor cause")
	tests := []struct {
		name             string
		wantCalls        []string
		wantErrors       []error
		wantAbsentErrors []error
		wantMessages     int
	}{
		{
			name:         "retryable first call",
			wantCalls:    []string{"first", "second", "third"},
			wantMessages: 5,
		},
		{
			name:         "malformed first arguments",
			wantCalls:    []string{"second", "third"},
			wantMessages: 5,
		},
		{
			name:         "terminal second call",
			wantCalls:    []string{"first", "second"},
			wantErrors:   []error{cause},
			wantMessages: 2,
		},
		{
			name:             "cancelled second call",
			wantCalls:        []string{"first", "second"},
			wantErrors:       []error{context.Canceled},
			wantAbsentErrors: []error{cause},
			wantMessages:     2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup: three calls are declared together, with one controlled failure.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runner := newCapabilityTestRunner("lookup")
			var dispatched []string
			runner.CapabilityRunner.OnExecuteCapability = func(_ context.Context, request capabilities.Request) (capabilities.Response, *capabilities.Error) {
				dispatched = append(dispatched, request.CallID)
				if request.CallID == "first" && tt.name == "retryable first call" {
					return capabilities.Response{}, &capabilities.Error{Retryable: true, Message: "correct first", Cause: cause}
				}
				if request.CallID == "second" {
					if tt.name == "cancelled second call" {
						cancel()
					}
					if len(tt.wantErrors) > 0 {
						return capabilities.Response{}, &capabilities.Error{Message: "terminal failure", Cause: cause}
					}
				}
				return capabilities.NewToolTextContentResponse("completed " + request.CallID), nil
			}
			turn := 0
			inference := mockprovider.New(ulid.Make())
			inference.OnGenerate = func(_ context.Context, request *provider.Request) (*provider.Response, error) {
				turn++
				if turn == 1 {
					firstArguments := `{}`
					if tt.name == "malformed first arguments" {
						firstArguments = `[]`
					}
					return capabilityTestResponse(
						prompts.ToolCall{CallID: "first", Name: "lookup", Arguments: firstArguments},
						prompts.ToolCall{CallID: "second", Name: "lookup", Arguments: `{}`},
						prompts.ToolCall{CallID: "third", Name: "lookup", Arguments: `{}`},
					), nil
				}
				require.Empty(t, tt.wantErrors, "terminal batches must not continue")
				require.Equal(t, tt.wantCalls, dispatched, "all calls finish before the next inference")
				require.Len(t, request.Input, 4)
				for i, id := range []string{"first", "second", "third"} {
					require.Equal(t, id, request.Input[i+1].ToolResults[0].CallID)
				}
				require.NotContains(t, request.Input[1].ToolResults[0].Content, cause.Error())
				return processTestResponse(), nil
			}

			// Execute the whole batch without automatically retrying its failed member.
			output, err := runProcessTest(t, ctx, inference, runner, &schema.Output{})

			// Assert sequential attempts and retention of only the completed results.
			if len(tt.wantErrors) == 0 {
				require.NoError(t, err)
				require.Equal(t, "answer", output.Output)
			} else {
				for _, want := range tt.wantErrors {
					require.ErrorIs(t, err, want)
				}
				for _, absent := range tt.wantAbsentErrors {
					require.NotErrorIs(t, err, absent)
				}
				require.Nil(t, output.Output)
				require.Equal(t, 1, turn)
				require.Equal(t, "completed first", output.Messages[1].ToolResults[0].Content)
			}
			require.Equal(t, tt.wantCalls, dispatched)
			require.Equal(t, uint64(len(tt.wantCalls)), output.Usage.ToolCalls)
			require.Len(t, output.Messages, tt.wantMessages)
			runner.Runner.AssertCalled(t, mock.Finalize, 1)
		})
	}
}
