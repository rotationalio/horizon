package config_test

import (
	"os"
	"testing"
	"time"

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
		require.Equal(t, int64(32), conf.MaxToolTurns)
	})

	t.Run("explicit zero disables tools", func(t *testing.T) {
		require.NoError(t, os.Setenv(key, "0"))

		conf, err := config.New()

		require.NoError(t, err)
		require.Zero(t, conf.MaxToolTurns)
	})
}

// Uses documented defaults when unset and parses explicit byte and duration overrides.
func TestAttachmentDownloadConfigDefaultsAndOverrides(t *testing.T) {
	clearEnv(t, "HORIZON_PROVIDER_CACHE_SIZE")
	clearEnv(t, "HORIZON_ATTACHMENT_MAX_DOWNLOAD_BYTES")
	clearEnv(t, "HORIZON_ATTACHMENT_DOWNLOAD_TIMEOUT")
	clearEnv(t, "HORIZON_EXECUTION_TIMEOUT")
	clearEnv(t, "HORIZON_FINALIZE_TIMEOUT")
	clearEnv(t, "HORIZON_PROVIDER_REQUEST_TIMEOUT")
	clearEnv(t, "HORIZON_HTTP_CLIENT_TIMEOUT")
	clearEnv(t, "HORIZON_BEST_EFFORT_PARSING")

	conf, err := config.New()
	require.NoError(t, err)
	require.Equal(t, 32, conf.ProviderCacheSize)
	require.EqualValues(t, 64<<20, conf.AttachmentMaxDownloadBytes)
	require.Equal(t, 8*time.Second, conf.AttachmentDownloadTimeout)
	require.Zero(t, conf.ExecutionTimeout)
	require.Equal(t, 8*time.Second, conf.FinalizeTimeout)
	require.Equal(t, 128*time.Second, conf.ProviderRequestTimeout)
	require.Equal(t, 768*time.Second, conf.HTTPClientTimeout)
	require.False(t, conf.BestEffortParsing)

	t.Setenv("HORIZON_PROVIDER_CACHE_SIZE", "64")
	t.Setenv("HORIZON_ATTACHMENT_MAX_DOWNLOAD_BYTES", "1048576")
	t.Setenv("HORIZON_ATTACHMENT_DOWNLOAD_TIMEOUT", "500ms")
	t.Setenv("HORIZON_EXECUTION_TIMEOUT", "2m")
	t.Setenv("HORIZON_FINALIZE_TIMEOUT", "3s")
	t.Setenv("HORIZON_PROVIDER_REQUEST_TIMEOUT", "45s")
	t.Setenv("HORIZON_HTTP_CLIENT_TIMEOUT", "5m")
	t.Setenv("HORIZON_BEST_EFFORT_PARSING", "true")
	conf, err = config.New()
	require.NoError(t, err)
	require.Equal(t, 64, conf.ProviderCacheSize)
	require.EqualValues(t, 1048576, conf.AttachmentMaxDownloadBytes)
	require.Equal(t, 500*time.Millisecond, conf.AttachmentDownloadTimeout)
	require.Equal(t, 2*time.Minute, conf.ExecutionTimeout)
	require.Equal(t, 3*time.Second, conf.FinalizeTimeout)
	require.Equal(t, 45*time.Second, conf.ProviderRequestTimeout)
	require.Equal(t, 5*time.Minute, conf.HTTPClientTimeout)
	require.True(t, conf.BestEffortParsing)
}

// Rejects non-positive attachment download limits and timeouts.
func TestAttachmentDownloadConfigRejectsInvalidValues(t *testing.T) {
	for _, tc := range []struct {
		key   string
		value string
	}{
		{key: "HORIZON_PROVIDER_CACHE_SIZE", value: "0"},
		{key: "HORIZON_PROVIDER_CACHE_SIZE", value: "-1"},
		{key: "HORIZON_ATTACHMENT_MAX_DOWNLOAD_BYTES", value: "0"},
		{key: "HORIZON_ATTACHMENT_MAX_DOWNLOAD_BYTES", value: "-1"},
		{key: "HORIZON_ATTACHMENT_DOWNLOAD_TIMEOUT", value: "0s"},
		{key: "HORIZON_ATTACHMENT_DOWNLOAD_TIMEOUT", value: "-1s"},
		{key: "HORIZON_EXECUTION_TIMEOUT", value: "-1s"},
		{key: "HORIZON_FINALIZE_TIMEOUT", value: "0s"},
		{key: "HORIZON_FINALIZE_TIMEOUT", value: "-1s"},
		{key: "HORIZON_PROVIDER_REQUEST_TIMEOUT", value: "0s"},
		{key: "HORIZON_PROVIDER_REQUEST_TIMEOUT", value: "-1s"},
		{key: "HORIZON_HTTP_CLIENT_TIMEOUT", value: "0s"},
		{key: "HORIZON_HTTP_CLIENT_TIMEOUT", value: "-1s"},
	} {
		t.Run(tc.key+"="+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			_, err := config.New()
			require.Error(t, err)
		})
	}
}

// Temporarily clears an environment variable and restores its prior state.
func clearEnv(t *testing.T, key string) {
	t.Helper()
	original, wasSet := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(key, original)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}
