package http_test

import (
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
