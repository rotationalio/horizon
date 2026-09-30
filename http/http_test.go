package http_test

import (
	"context"
	"io"
	stdhttp "net/http"
	"net/http/httptest"

	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/http"
	"go.rtnl.ai/horizon/provider/auth"
)

// Accepts all successful 2xx response codes with and without explicit options.
func TestGetAcceptsAnySuccessful2xx(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		w.WriteHeader(stdhttp.StatusCreated)
		_, _ = io.WriteString(w, "created")
	}))
	t.Cleanup(server.Close)

	body, err := http.Get(t.Context(), server.URL, nil)
	require.NoError(t, err)
	require.Equal(t, []byte("created"), body)
	body, err = http.Get(t.Context(), server.URL, nil, http.GetOptions{Accept: "text/plain"})
	require.NoError(t, err)
	require.Equal(t, []byte("created"), body)
}

// Sends a binary-friendly Accept value and enforces limits for sized and chunked responses.
func TestGetBinaryAcceptAndBodyLimits(t *testing.T) {
	for _, tc := range []struct {
		name       string
		contentLen bool
	}{
		{name: "content length"},
		{name: "unknown length", contentLen: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
				require.Equal(t, "*/*", r.Header.Get("Accept"))
				if tc.contentLen {
					w.(stdhttp.Flusher).Flush()
				}
				_, _ = io.WriteString(w, "binary-data")
			}))
			t.Cleanup(server.Close)

			_, err := http.Get(t.Context(), server.URL, nil, http.GetOptions{
				Accept:       "*/*",
				MaxBodyBytes: 4,
			})
			require.ErrorContains(t, err, "exceeds maximum size")
		})
	}
}

// Returns useful status and body information for unsuccessful responses.
func TestGetRejectsErrorStatus(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		stdhttp.Error(w, "not found", stdhttp.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	_, err := http.Get(t.Context(), server.URL, nil, http.GetOptions{})
	require.ErrorIs(t, err, errors.ErrRequestFailed)
	require.ErrorContains(t, err, "not found")
}

// Applies the configured body limit to error responses as well as successful ones.
func TestGetBodyLimitAppliesToErrorResponse(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		stdhttp.Error(w, "this response is too large", stdhttp.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	_, err := http.Get(t.Context(), server.URL, nil, http.GetOptions{MaxBodyBytes: 4})
	require.ErrorContains(t, err, "response body exceeds maximum size of 4 bytes")
}

// Propagates caller cancellation through the shared GET helper.
func TestGetPropagatesCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := http.Get(ctx, server.URL, nil, http.GetOptions{})
		result <- err
	}()
	<-started
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
}

// Rejects credential types that cannot be applied to a direct HTTP request.
func TestGetRejectsUnsupportedCredential(t *testing.T) {
	_, err := http.Get(t.Context(), "https://example.com", auth.NewOAuth2Client("client", "secret"), http.GetOptions{})
	require.ErrorIs(t, err, auth.ErrUnsupportedCredentialType)
}
