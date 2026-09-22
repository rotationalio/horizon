package provider

import (
	"net/http"
	"testing"

	openaigo "github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/assert"
	"go.rtnl.ai/horizon/errors"
)

// TestAuthRequired verifies the testauthrequired behavior covered by this test.
func TestAuthRequired(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "openai unauthorized",
			err:  &openaigo.Error{StatusCode: http.StatusUnauthorized},
			want: true,
		},
		{
			name: "openai forbidden",
			err:  &openaigo.Error{StatusCode: http.StatusForbidden},
			want: true,
		},
		{
			name: "openai other status",
			err:  &openaigo.Error{StatusCode: http.StatusTooManyRequests},
			want: false,
		},
		{
			name: "catalog http unauthorized",
			err: errors.Join(
				errors.ErrCatalogConnectivity,
				errors.ErrRequestFailed,
				errors.Fmt("%s: invalid api key", http.StatusText(http.StatusUnauthorized)),
			),
			want: false,
		},
		{
			name: "catalog http unauthorized with status line",
			err: errors.Join(
				errors.ErrCatalogConnectivity,
				errors.ErrRequestFailed,
				errors.Fmt("%s: invalid api key", "401 Unauthorized"),
			),
			want: true,
		},
		{
			name: "catalog http forbidden with status line",
			err: errors.Join(
				errors.ErrRequestFailed,
				errors.Fmt("%s: forbidden", "403 Forbidden"),
			),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.Join(errors.ErrCatalogConnectivity, errors.Fmt("connection refused")),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, authRequired(tt.err))
		})
	}
}
