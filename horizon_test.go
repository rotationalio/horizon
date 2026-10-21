package horizon_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon"
	"go.rtnl.ai/horizon/config"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/prompts"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	providermock "go.rtnl.ai/horizon/provider/mock"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/horizon/task"
	"go.rtnl.ai/horizon/task/mock"
	"go.rtnl.ai/ulid"
)

// Verifies factory-backed execution resolves task provider configuration and preserves
// setup failures and their causes.
func TestHorizonNewRejectsInvalidConfiguration(t *testing.T) {
	config.Reset()
	t.Cleanup(config.Reset)
	t.Setenv("HORIZON_FINALIZE_TIMEOUT", "0s")

	h, err := horizon.New()
	require.Nil(t, h)
	require.Error(t, err)
}

func TestHorizonRun(t *testing.T) {
	t.Run("registered provider", func(t *testing.T) {
		calls, finalized := 0, 0
		h := newTestHorizon(t, testFactory(func(ctx context.Context) (task.Runner, error) {
			calls++
			runner := selectedRunner()
			runner.OnFinalize = func(_ context.Context, output *task.Output, _ error) error {
				finalized++
				output.Output = "completed"
				return nil
			}
			return runner, nil
		}))
		_, err := h.GetOrCreateProvider(testProviderConfig(), readyProvider())
		require.NoError(t, err)
		tsk := &task.Task{Output: &schema.Output{}}

		for range 2 {
			output, err := h.Run(context.Background(), nil, tsk)
			require.NoError(t, err)
			require.Equal(t, "completed", output.Output)
		}
		require.Equal(t, 2, calls)
		require.Equal(t, 2, finalized)
		require.Nil(t, tsk.Provider, "Prepare must not mutate the original task object")
	})

	t.Run("task config without registration", func(t *testing.T) {
		config := inferenceConfig(t)
		original := config
		h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) {
			return noopRunner(), nil
		}))
		tsk := &task.Task{
			Provider: &config,
			Output:   &schema.Output{},
		}
		for range 2 {
			_, err := h.Run(context.Background(), nil, tsk)
			require.NoError(t, err)
		}
		require.Equal(t, original, config)
		require.True(t, h.RemoveProvider(config.ID))
	})

	t.Run("prepare refreshes task config", func(t *testing.T) {
		config := inferenceConfig(t)
		original := config
		h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) {
			runner := noopRunner()
			runner.OnPrepare = func(_ context.Context, tsk *task.Task) error {
				require.NotSame(t, &config, tsk.Provider)
				tsk.Provider.DefaultModel = "refreshed-model"
				tsk.Provider.Credentials = auth.NewAPIKey("fresh-secret")
				return nil
			}
			return runner, nil
		}))
		_, err := h.Run(context.Background(), nil, &task.Task{
			Provider: &config,
			Output:   &schema.Output{},
		})
		require.NoError(t, err)
		require.Equal(t, original, config)
	})

	t.Run("invalid provider config", func(t *testing.T) {
		h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) {
			return noopRunner(), nil
		}))
		config := testProviderConfig()
		config.Credentials = auth.NewAPIKey("")
		output, err := h.Run(context.Background(), nil, &task.Task{
			Provider: &config,
			Output:   &schema.Output{},
		})
		require.Error(t, err)
		require.NotNil(t, output)
	})

	t.Run("factory receives context", func(t *testing.T) {
		type key struct{}
		ctx := context.WithValue(context.Background(), key{}, "request")
		h := newTestHorizon(t, testFactory(func(got context.Context) (task.Runner, error) {
			require.Equal(t, "request", got.Value(key{}))
			return selectedRunner(), nil
		}))
		_, err := h.GetOrCreateProvider(testProviderConfig(), readyProvider())
		require.NoError(t, err)
		_, err = h.Run(ctx, nil, &task.Task{Output: &schema.Output{}})
		require.NoError(t, err)
	})

	t.Run("factory failure", func(t *testing.T) {
		cause := errors.New("cannot create runner")
		h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) {
			return selectedRunner(), cause
		}))
		output, err := h.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}})
		require.Nil(t, output)
		require.ErrorIs(t, err, cause)
	})

	t.Run("nil runner", func(t *testing.T) {
		h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) {
			return nil, nil
		}))
		_, err := h.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}})
		require.ErrorIs(t, err, errors.ErrRunnerRequired)
	})

	t.Run("provider constructed on demand", func(t *testing.T) {
		h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) {
			return noopRunner(), nil
		}))
		output, err := h.Run(context.Background(), nil, &task.Task{Provider: ptrConfig(inferenceConfig(t)), Output: &schema.Output{}})
		require.NoError(t, err)
		require.NotNil(t, output)
	})

	t.Run("missing provider selection", func(t *testing.T) {
		h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) {
			return noopRunner(), nil
		}))
		_, err := h.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}})
		require.ErrorIs(t, err, errors.ErrProviderRequired)
	})

	t.Run("missing task", func(t *testing.T) {
		h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) {
			return selectedRunner(), nil
		}))
		_, err := h.Run(context.Background(), nil, nil)
		require.ErrorIs(t, err, errors.ErrTaskRequired)
	})

	t.Run("missing output", func(t *testing.T) {
		h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) {
			return selectedRunner(), nil
		}))
		_, err := h.Run(context.Background(), nil, &task.Task{})
		require.ErrorIs(t, err, errors.ErrTaskOutputRequired)
	})

	t.Run("prepare failure", func(t *testing.T) {
		cause := errors.New("preparation failed")
		h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) {
			runner := noopRunner()
			runner.OnPrepare = func(context.Context, *task.Task) error { return cause }
			return runner, nil
		}))
		_, err := h.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}})
		require.ErrorIs(t, err, cause)
	})

	t.Run("finalize failure", func(t *testing.T) {
		cause := errors.New("finalization failed")
		h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) {
			runner := selectedRunner()
			runner.OnFinalize = func(context.Context, *task.Output, error) error { return cause }
			return runner, nil
		}))
		_, err := h.GetOrCreateProvider(testProviderConfig(), readyProvider())
		require.NoError(t, err)
		_, err = h.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}})
		require.ErrorIs(t, err, cause)
	})

	t.Run("cancelled before creation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) {
			require.FailNow(t, "factory must not be called for a cancelled execution")
			return nil, nil
		}))
		_, err := h.Run(ctx, nil, &task.Task{})
		require.ErrorIs(t, err, context.Canceled)
	})

}

// Confirms Horizon applies the configured HTTP timeout to its own provider requests.
func TestHorizonUsesConfiguredHTTPClientTimeout(t *testing.T) {
	conf, err := config.New()
	require.NoError(t, err)
	conf.HTTPClientTimeout = 40 * time.Millisecond
	conf.ProviderRequestTimeout = time.Second
	require.NoError(t, config.Set(*conf))
	t.Cleanup(config.Reset)

	started := make(chan struct{}, 1)
	releaseHandler := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-releaseHandler:
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(releaseHandler) })

	h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) {
		return noopRunner(), nil
	}))
	providerConfig := provider.Config{
		ID:                testProviderID,
		APIType:           provider.APITypeOpenAIChatCompletions,
		ProviderType:      provider.ProviderTypeOpenAICompatible,
		InferenceEndpoint: server.URL + "/v1",
		CatalogEndpoint:   server.URL + "/v1/models",
		DefaultModel:      "test-model",
		Credentials:       auth.NewNone(),
	}
	requestStarted := time.Now()
	_, err = h.Run(context.Background(), nil, &task.Task{
		Provider: &providerConfig,
		Model:    task.Model{Slug: "test-model"},
		Output:   &schema.Output{},
	})
	elapsed := time.Since(requestStarted)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "provider request did not reach the test server")
	}
	require.Less(t, elapsed, 500*time.Millisecond, "configured HTTP timeout did not bound the provider request")
}

// Verifies configured execution timeouts through the handler and both Run APIs.
func TestHorizonExecutionTimeout(t *testing.T) {
	// Setup one configured budget for all three public execution paths, then
	// restore the global config after the test.
	const timeout = 200 * time.Millisecond
	conf, err := config.New()
	require.NoError(t, err)
	conf.ExecutionTimeout = timeout
	require.NoError(t, config.Set(*conf))
	t.Cleanup(config.Reset)

	// Each runner records its deadline, waits for cancellation, and reports
	// the execution error to Finalize.
	type deadlineObservation struct {
		present  bool
		deadline time.Time
	}
	newTimeoutRunner := func() (*mock.Runner, chan deadlineObservation, chan error) {
		prepareDeadline := make(chan deadlineObservation, 1)
		finalized := make(chan error, 1)
		runner := noopRunner()
		runner.OnPrepare = func(ctx context.Context, _ *task.Task) error {
			deadline, present := ctx.Deadline()
			prepareDeadline <- deadlineObservation{present: present, deadline: deadline}
			if !present {
				return errors.New("task context has no execution deadline")
			}
			timer := time.NewTimer(timeout * 2)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return errors.New("task context was not cancelled before the test bound")
			}
		}
		runner.OnFinalize = func(_ context.Context, _ *task.Output, runErr error) error {
			finalized <- runErr
			return nil
		}
		return runner, prepareDeadline, finalized
	}

	// Keep timeout regressions bounded so they fail instead of hanging the suite.
	waitForDeadline := func(t *testing.T, observed <-chan deadlineObservation) deadlineObservation {
		t.Helper()
		select {
		case deadline := <-observed:
			return deadline
		case <-time.After(timeout * 10):
			t.Fatal("execution did not report its context deadline")
			return deadlineObservation{}
		}
	}
	waitForExecutionError := func(t *testing.T, finalized <-chan error) {
		t.Helper()
		select {
		case runErr := <-finalized:
			require.ErrorIs(t, runErr, context.DeadlineExceeded)
		case <-time.After(timeout * 10):
			t.Fatal("runner was not finalized")
		}
	}

	t.Run("handler shares its deadline with factory and execution", func(t *testing.T) {
		// Setup a factory that spends part of the budget before returning
		// the runner that waits for execution cancellation.
		runner, prepareDeadline, finalized := newTimeoutRunner()
		factoryDeadline := make(chan deadlineObservation, 1)
		factory := testFactory(func(ctx context.Context) (task.Runner, error) {
			deadline, present := ctx.Deadline()
			factoryDeadline <- deadlineObservation{present: present, deadline: deadline}
			timer := time.NewTimer(timeout / 4)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-timer.C:
				return runner, nil
			}
		})

		// Run a request through the handler, which starts the timeout before
		// it calls the factory.
		h := newTestHorizon(t, factory)
		handler := h.Handler()
		handler.Router().Insert("/task", &task.Task{Output: &schema.Output{}})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/task", nil))

		// Assert both stages received the same deadline and finalization
		// received the deadline error.
		require.Equal(t, http.StatusInternalServerError, response.Code)
		factoryContext := waitForDeadline(t, factoryDeadline)
		taskContext := waitForDeadline(t, prepareDeadline)
		require.True(t, factoryContext.present, "factory context should have an execution deadline")
		require.True(t, taskContext.present, "task context should have an execution deadline")
		require.True(t, factoryContext.deadline.Equal(taskContext.deadline), "Run must not restart the handler's timeout")
		waitForExecutionError(t, finalized)
	})

	t.Run("Horizon.Run applies the configured timeout", func(t *testing.T) {
		// Run directly with an explicit runner to verify the shared engine applies the budget.
		runner, prepareDeadline, finalized := newTimeoutRunner()
		h := newTestHorizon(t, nil)
		_, err := h.Horizon.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}}, runner)

		// Assert Run returns the timeout cause and the runner observed a deadline.
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.True(t, waitForDeadline(t, prepareDeadline).present)
		waitForExecutionError(t, finalized)
	})

	t.Run("package Run applies the configured timeout", func(t *testing.T) {
		// Exercise the one-off package helper with a fresh runner and the same config.
		runner, prepareDeadline, finalized := newTimeoutRunner()
		_, err := horizon.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}}, runner)

		// Assert this API also applies the budget and finalizes with its expiration cause.
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.True(t, waitForDeadline(t, prepareDeadline).present)
		waitForExecutionError(t, finalized)
	})
}

// Verifies one-off execution constructs the provider selected by the runner.
func TestRun(t *testing.T) {
	t.Run("task provider config", func(t *testing.T) {
		config := inferenceConfig(t)
		finalized := false
		runner := noopRunner()
		runner.OnFinalize = func(context.Context, *task.Output, error) error {
			finalized = true
			return nil
		}
		output, err := horizon.Run(context.Background(), nil, &task.Task{Provider: &config, Output: &schema.Output{}}, runner)
		require.NoError(t, err)
		require.Equal(t, "generated", output.Output)
		require.True(t, finalized)
	})
	t.Run("Prepare selects provider", func(t *testing.T) {
		config := inferenceConfig(t)
		original := &task.Task{Output: &schema.Output{}}
		runner := noopRunner()
		runner.OnPrepare = func(_ context.Context, tsk *task.Task) error {
			tsk.Provider = &config
			return nil
		}
		output, err := horizon.Run(context.Background(), nil, original, runner)
		require.NoError(t, err)
		require.Equal(t, "generated", output.Output)
		require.Nil(t, original.Provider)
	})
	t.Run("missing provider", func(t *testing.T) {
		_, err := horizon.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}}, noopRunner())
		require.ErrorIs(t, err, errors.ErrProviderRequired)
	})
}

// Verifies concurrent executions receive distinct runners and do not mutate a shared task.
func TestHorizonRunConcurrent(t *testing.T) {
	const runs = 12
	var mu sync.Mutex
	runners := make(map[*mock.Runner]bool)
	h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) {
		runner := selectedRunner()
		mu.Lock()
		runners[runner] = true
		mu.Unlock()
		return runner, nil
	}))
	_, err := h.GetOrCreateProvider(testProviderConfig(), readyProvider())
	require.NoError(t, err)
	tsk := &task.Task{Output: &schema.Output{}}
	results := make(chan error, runs)
	for range runs {
		go func() {
			_, err := h.Run(context.Background(), nil, tsk)
			results <- err
		}()
	}
	for range runs {
		require.NoError(t, <-results)
	}
	require.Len(t, runners, runs)
	require.Nil(t, tsk.Provider)
}

func TestHorizonUnregister(t *testing.T) {
	config := inferenceConfig(t)
	h := newTestHorizon(t, testFactory(func(context.Context) (task.Runner, error) { return noopRunner(), nil }))
	_, err := h.GetOrCreateProvider(config, nil)
	require.NoError(t, err)
	require.True(t, h.RemoveProvider(config.ID))
	require.False(t, h.RemoveProvider(config.ID))
	_, err = h.Run(context.Background(), nil, &task.Task{Provider: &config, Output: &schema.Output{}})
	require.NoError(t, err)
	require.True(t, h.RemoveProvider(config.ID), "execution recreates the evicted provider")
}

// newTestHorizon creates a shared engine and attaches the factory used by these tests.
func newTestHorizon(t *testing.T, factory horizon.RunnerFactory) *testHorizon {
	t.Helper()
	engine, err := horizon.New()
	require.NoError(t, err)
	return &testHorizon{Horizon: engine, factory: factory}
}

// testHorizon adapts the explicit-runner engine for tests that exercise a runner factory.
type testHorizon struct {
	*horizon.Horizon
	factory horizon.RunnerFactory
	handler *horizon.Handler
}

// Run creates a test runner and executes the task through the underlying Horizon.
func (h *testHorizon) Run(ctx context.Context, input *task.Input, definition *task.Task) (*task.Output, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	runner, err := h.factory.NewRunner(ctx)
	if err != nil {
		return nil, err
	}
	return h.Horizon.Run(ctx, input, definition, runner)
}

// Handler creates and retains one HTTP handler so test routes persist between requests.
func (h *testHorizon) Handler() *horizon.Handler {
	if h.handler == nil {
		h.handler, _ = h.Horizon.NewHandler(h.factory)
	}
	return h.handler
}

// testFactory adapts a callback to Horizon's runner factory interface.
type testFactory func(context.Context) (task.Runner, error)

// NewRunner calls the test callback to create an execution-local runner.
func (f testFactory) NewRunner(ctx context.Context) (task.Runner, error) {
	return f(ctx)
}

var testProviderID = ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV")

// testProviderConfig returns the shared mock provider configuration for tests.
func testProviderConfig() provider.Config {
	return provider.Config{
		ID:           testProviderID,
		APIType:      provider.APITypeMock,
		ProviderType: provider.ProviderTypeMock,
		Credentials:  auth.NewNone(),
	}
}

// noopRunner returns a runner with no-op preparation and finalization hooks.
func noopRunner() *mock.Runner {
	return &mock.Runner{
		OnPrepare:  func(context.Context, *task.Task) error { return nil },
		OnFinalize: func(context.Context, *task.Output, error) error { return nil },
	}
}

// selectedRunner selects the standard mock provider during task preparation.
func selectedRunner() *mock.Runner {
	runner := noopRunner()
	runner.OnPrepare = func(_ context.Context, t *task.Task) error {
		config := testProviderConfig()
		t.Provider = &config
		return nil
	}
	return runner
}

func readyProvider() *providermock.MockProvider {
	p := providermock.New(testProviderID)
	p.OnGenerate = func(context.Context, *provider.Request) (*provider.Response, error) {
		return &provider.Response{Output: prompts.Prompts{{Role: prompts.RoleAssistant, Content: "generated"}}}, nil
	}
	return p
}

func inferenceConfig(t *testing.T) provider.Config {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"test","object":"chat.completion","model":"test-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"generated"}}]}`))
	}))
	t.Cleanup(server.Close)
	return provider.Config{ID: testProviderID, APIType: provider.APITypeOpenAIChatCompletions, ProviderType: provider.ProviderTypeOpenAICompatible, InferenceEndpoint: server.URL + "/v1", CatalogEndpoint: server.URL + "/v1/models", DefaultModel: "test-model", Credentials: auth.NewNone()}
}

func ptrConfig(config provider.Config) *provider.Config { return &config }
