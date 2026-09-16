package config_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/config"
)

// The default is used when max tool turns is omitted. Zero disables tools.
func TestMaxToolTurnsConfiguration(t *testing.T) {
	const key = "HORIZON_MAX_TOOL_TURNS"
	original, wasSet := os.LookupEnv(key)
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(key, original)
		} else {
			_ = os.Unsetenv(key)
		}
	})

	t.Run("omitted uses default", func(t *testing.T) {
		require.NoError(t, os.Unsetenv(key))

		conf, err := config.New()

		require.NoError(t, err)
		require.Equal(t, int64(config.DefaultMaxToolTurns), conf.MaxToolTurns)
	})

	t.Run("explicit zero disables tools", func(t *testing.T) {
		require.NoError(t, os.Setenv(key, "0"))

		conf, err := config.New()

		require.NoError(t, err)
		require.Zero(t, conf.MaxToolTurns)
	})
}
