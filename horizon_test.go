package horizon_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon"
	"go.rtnl.ai/horizon/errors"
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
func TestHorizonRun(t *testing.T) {
	t.Run("registered provider", func(t *testing.T) {
		calls, finalized := 0, 0
		h := horizon.New(testFactory(func(ctx context.Context) (task.Runner, error) {
			calls++
			runner := selectedRunner()
			runner.OnFinalize = func(_ context.Context, output *task.Output) error {
				finalized++
				output.Output = "completed"
				return nil
			}
			return runner, nil
		}))
		require.NoError(t, h.AddProviderInstance(testProviderConfig(), providermock.New(testProviderID)))
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
		config := testProviderConfig()
		h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
			return &mock.Runner{}, nil
		}))
		tsk := &task.Task{
			Provider: &config,
			Output:   &schema.Output{},
		}
		for range 2 {
			_, err := h.Run(context.Background(), nil, tsk)
			require.NoError(t, err)
		}
		require.Equal(t, testProviderConfig(), config)
		require.True(t, h.RemoveProvider(config.ID))
	})

	t.Run("prepare refreshes task config", func(t *testing.T) {
		config := testProviderConfig()
		h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
			return &mock.Runner{OnPrepare: func(_ context.Context, tsk *task.Task) error {
				require.NotSame(t, &config, tsk.Provider)
				tsk.Provider.DefaultModel = "refreshed-model"
				tsk.Provider.Credentials = auth.NewAPIKey("fresh-secret")
				return nil
			}}, nil
		}))
		_, err := h.Run(context.Background(), nil, &task.Task{
			Provider: &config,
			Output:   &schema.Output{},
		})
		require.NoError(t, err)
		require.Equal(t, testProviderConfig(), config)
	})

	t.Run("invalid provider config", func(t *testing.T) {
		h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
			return &mock.Runner{}, nil
		}))
		config := testProviderConfig()
		config.Credentials = auth.NewAPIKey("")
		output, err := h.Run(context.Background(), nil, &task.Task{
			Provider: &config,
			Output:   &schema.Output{},
		})
		require.Error(t, err)
		require.Nil(t, output)
	})

	t.Run("factory receives context", func(t *testing.T) {
		type key struct{}
		ctx := context.WithValue(context.Background(), key{}, "request")
		h := horizon.New(testFactory(func(got context.Context) (task.Runner, error) {
			require.Equal(t, "request", got.Value(key{}))
			return selectedRunner(), nil
		}))
		require.NoError(t, h.AddProviderInstance(testProviderConfig(), providermock.New(testProviderID)))
		_, err := h.Run(ctx, nil, &task.Task{Output: &schema.Output{}})
		require.NoError(t, err)
	})

	t.Run("factory failure", func(t *testing.T) {
		cause := errors.New("cannot create runner")
		h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
			return selectedRunner(), cause
		}))
		output, err := h.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}})
		require.Nil(t, output)
		require.ErrorIs(t, err, cause)
	})

	t.Run("nil runner", func(t *testing.T) {
		h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
			return nil, nil
		}))
		_, err := h.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}})
		require.ErrorIs(t, err, errors.ErrRunnerRequired)
	})

	t.Run("provider constructed on demand", func(t *testing.T) {
		h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
			return selectedRunner(), nil
		}))
		output, err := h.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}})
		require.NoError(t, err)
		require.NotNil(t, output)
	})

	t.Run("missing provider selection", func(t *testing.T) {
		h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
			return &mock.Runner{}, nil
		}))
		require.NoError(t, h.AddProviderInstance(testProviderConfig(), providermock.New(testProviderID)))
		_, err := h.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}})
		require.ErrorIs(t, err, errors.ErrProviderRequired)
	})

	t.Run("missing task", func(t *testing.T) {
		h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
			return selectedRunner(), nil
		}))
		_, err := h.Run(context.Background(), nil, nil)
		require.ErrorIs(t, err, errors.ErrTaskRequired)
	})

	t.Run("missing output", func(t *testing.T) {
		h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
			return selectedRunner(), nil
		}))
		_, err := h.Run(context.Background(), nil, &task.Task{})
		require.ErrorIs(t, err, errors.ErrTaskOutputRequired)
	})

	t.Run("prepare failure", func(t *testing.T) {
		cause := errors.New("preparation failed")
		h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
			return &mock.Runner{OnPrepare: func(context.Context, *task.Task) error {
				return cause
			}}, nil
		}))
		_, err := h.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}})
		require.ErrorIs(t, err, cause)
	})

	t.Run("finalize failure", func(t *testing.T) {
		cause := errors.New("finalization failed")
		h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
			runner := selectedRunner()
			runner.OnFinalize = func(context.Context, *task.Output) error { return cause }
			return runner, nil
		}))
		require.NoError(t, h.AddProviderInstance(testProviderConfig(), providermock.New(testProviderID)))
		_, err := h.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}})
		require.ErrorIs(t, err, cause)
	})

	t.Run("cancelled before creation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
			require.FailNow(t, "factory must not be called for a cancelled execution")
			return nil, nil
		}))
		_, err := h.Run(ctx, nil, &task.Task{})
		require.ErrorIs(t, err, context.Canceled)
	})

}

// Verifies one-off execution accepts a provider directly without any registration.
func TestRun(t *testing.T) {
	t.Run("explicit provider", func(t *testing.T) {
		finalized := false
		runner := &mock.Runner{OnFinalize: func(context.Context, *task.Output) error {
			finalized = true
			return nil
		}}
		output, err := horizon.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}}, runner, providermock.New(testProviderID))
		require.NoError(t, err)
		require.NotNil(t, output)
		require.True(t, finalized)
	})

	t.Run("provider from config", func(t *testing.T) {
		instance, err := horizon.NewProvider(provider.Config{
			ID:           testProviderID,
			APIType:      provider.APITypeMock,
			ProviderType: provider.ProviderTypeMock,
			Credentials:  auth.NewNone(),
		})
		require.NoError(t, err)
		_, err = horizon.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}}, &mock.Runner{}, instance)
		require.NoError(t, err)
	})

	t.Run("missing provider", func(t *testing.T) {
		_, err := horizon.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}}, &mock.Runner{}, nil)
		require.ErrorIs(t, err, errors.ErrProviderRequired)
	})
}

// Verifies concurrent executions receive distinct runners and do not mutate a shared task.
func TestHorizonRunConcurrent(t *testing.T) {
	const runs = 12
	var mu sync.Mutex
	runners := make(map[*mock.Runner]bool)
	h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
		runner := selectedRunner()
		mu.Lock()
		runners[runner] = true
		mu.Unlock()
		return runner, nil
	}))
	require.NoError(t, h.AddProviderInstance(testProviderConfig(), providermock.New(testProviderID)))
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

// Verifies provider registration works through Horizon without exposing its registry.
func TestHorizonRegister(t *testing.T) {
	h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
		return selectedRunner(), nil
	}))
	config := provider.Config{
		ID:           testProviderID,
		APIType:      provider.APITypeMock,
		ProviderType: provider.ProviderTypeMock,
		Credentials:  auth.NewNone(),
	}
	require.NoError(t, h.AddProvider(config))
	require.NoError(t, h.AddProvider(config))
	_, err := h.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}})
	require.NoError(t, err)
	require.Same(t, h.Handler(), h.Handler())
}

// Verifies removing a provider evicts its client and later executions can recreate it.
func TestHorizonUnregister(t *testing.T) {
	h := horizon.New(testFactory(func(context.Context) (task.Runner, error) {
		return selectedRunner(), nil
	}))
	config := testProviderConfig()
	require.NoError(t, h.AddProviderInstance(config, providermock.New(config.ID)))
	require.True(t, h.RemoveProvider(config.ID))
	require.False(t, h.RemoveProvider(config.ID))
	_, err := h.Run(context.Background(), nil, &task.Task{Output: &schema.Output{}})
	require.NoError(t, err)
	require.True(t, h.RemoveProvider(config.ID), "execution recreates the evicted provider")
}

type testFactory func(context.Context) (task.Runner, error)

func (f testFactory) NewRunner(ctx context.Context) (task.Runner, error) {
	return f(ctx)
}

var testProviderID = ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV")

func testProviderConfig() provider.Config {
	return provider.Config{
		ID:           testProviderID,
		APIType:      provider.APITypeMock,
		ProviderType: provider.ProviderTypeMock,
		Credentials:  auth.NewNone(),
	}
}

func selectedRunner() *mock.Runner {
	return &mock.Runner{
		OnPrepare: func(_ context.Context, t *task.Task) error {
			config := testProviderConfig()
			t.Provider = &config
			return nil
		},
	}
}
