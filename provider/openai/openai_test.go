package openai_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/provider"
	"go.rtnl.ai/horizon/provider/auth"
	"go.rtnl.ai/horizon/provider/openai"
)

// Verifies no-auth clients suppress ambient OpenAI credentials and send no
// Authorization header.
func TestNewWithoutAuthDoesNotInheritAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "ambient-secret")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Values("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"object":"list","data":[]}`))
		require.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	client, err := openai.New(provider.Config{
		InferenceEndpoint: server.URL + "/v1",
		Credentials:       auth.NewNone(),
	})
	require.NoError(t, err)

	_, err = client.Models.List(t.Context())
	require.NoError(t, err)
}
