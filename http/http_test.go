package http_test

import (
	"context"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/http"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/version"
)

// Confirms endpoint suffixes retain the base path and combine query values.
func TestEndpointGetJoinsBasePathAndQuery(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		require.Equal(t, "/api/v1/models/a/b", r.URL.EscapedPath())
		require.Equal(t, "1", r.URL.Query().Get("base"))
		require.Equal(t, "2", r.URL.Query().Get("extra"))
		require.Equal(t, version.UserAgent(), r.Header.Get("User-Agent"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		w.WriteHeader(stdhttp.StatusCreated)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(server.Close)

	endpoint, err := http.NewEndpoint(server.URL+"/api/v1?base=1", nil, auth.NewNone())
	require.NoError(t, err)
	body, err := endpoint.Get(t.Context(), "/models/a%2Fb?extra=2")
	require.NoError(t, err)
	require.JSONEq(t, `{"ok":true}`, string(body))
}

// Confirms endpoint requests apply API key and organization credentials.
func TestEndpointGetCredentials(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		require.Equal(t, "org", r.Header.Get("OpenAI-Organization"))
		require.Equal(t, "project", r.Header.Get("OpenAI-Project"))
		w.WriteHeader(stdhttp.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	endpoint, err := http.NewEndpoint(server.URL, nil, auth.NewOpenAIOrganization("secret", "org", "project"))
	require.NoError(t, err)
	body, err := endpoint.Get(t.Context(), "")
	require.NoError(t, err)
	require.Empty(t, body)
}

// Rejects endpoint URLs that are not valid absolute HTTP(S) addresses.
func TestNewEndpointValidatesURL(t *testing.T) {
	for _, target := range []string{"", "/relative", "ftp://example.com", "http:///missing-host", "https://example.com/#fragment"} {
		t.Run(target, func(t *testing.T) {
			_, err := http.NewEndpoint(target, nil, nil)
			require.Error(t, err)
		})
	}
}

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

// Uses an explicitly supplied HTTP client instead of the package default.
func TestEndpointUsesInjectedClient(t *testing.T) {
	called := false
	client := &stdhttp.Client{Transport: roundTripFunc(func(r *stdhttp.Request) (*stdhttp.Response, error) {
		called = true
		require.Equal(t, "https://example.com/api/child", r.URL.String())
		return &stdhttp.Response{
			StatusCode: stdhttp.StatusOK,
			Status:     "200 OK",
			Header:     make(stdhttp.Header),
			Body:       io.NopCloser(strings.NewReader("injected")),
			Request:    r,
		}, nil
	})}
	endpoint, err := http.NewEndpoint("https://example.com/api", client, nil)
	require.NoError(t, err)
	body, err := endpoint.Get(t.Context(), "child")
	require.NoError(t, err)
	require.Equal(t, "injected", string(body))
	require.True(t, called)
}

// Rejects credential types that cannot be applied to a direct HTTP request.
func TestGetRejectsUnsupportedCredential(t *testing.T) {
	_, err := http.Get(t.Context(), "https://example.com", auth.NewOAuth2Client("client", "secret"), http.GetOptions{})
	require.ErrorIs(t, err, auth.ErrUnsupportedCredentialType)
}

// Sends unauthenticated requests for nil credentials and explicit no-auth credentials.
func TestNilAndNoneCredentialsSendNoAuthorization(t *testing.T) {
	for name, cred := range map[string]auth.RequestCredential{"nil": nil, "none": auth.NewNone()} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
				require.Empty(t, r.Header.Values("Authorization"))
				w.WriteHeader(stdhttp.StatusNoContent)
			}))
			t.Cleanup(server.Close)
			endpoint, err := http.NewEndpoint(server.URL, nil, cred)
			require.NoError(t, err)
			_, err = endpoint.Get(t.Context(), "")
			require.NoError(t, err)
		})
	}
}

type roundTripFunc func(*stdhttp.Request) (*stdhttp.Response, error)

// Sends a request through the test-provided round-trip callback.
func (fn roundTripFunc) RoundTrip(r *stdhttp.Request) (*stdhttp.Response, error) { return fn(r) }
