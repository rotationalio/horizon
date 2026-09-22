package provider

import (
	"errors"
	"testing"

	openaigo "github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
)

// Verifies API errors add their status code prefix.
func TestProviderErrorPrefixesOpenAIStatus(t *testing.T) {
	t.Parallel()

	got := newProviderError(&openaigo.Error{
		StatusCode: 422,
		Message:    "validation error: model is required",
	})

	require.EqualError(t, got, "422: validation error: model is required")
}

// Verifies regular provider errors are returned without additional formatting.
func TestProviderErrorReturnsFullMessage(t *testing.T) {
	t.Parallel()

	message := "provider returned a detailed error message"
	underlying := errors.New(message)
	got := newProviderError(underlying)

	require.EqualError(t, got, message)
	require.ErrorIs(t, got, underlying)
}

// Verifies 401 detection continues through status wrapping.
func TestProviderErrorPreservesUnderlyingErrorForAuthDetection(t *testing.T) {
	t.Parallel()

	underlying := &openaigo.Error{StatusCode: 401, Message: "invalid API key"}
	got := newProviderError(underlying)

	require.True(t, authRequired(got))
	require.Equal(t, "401: invalid API key", got.Error())
}
