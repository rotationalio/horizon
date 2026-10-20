package provider_test

import (
	"net/http"
	"testing"

	"github.com/openai/openai-go/v3"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/provider"
)

func TestStatusFromError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want provider.Status
	}{
		{name: "nil", err: nil, want: provider.StatusError},
		{name: "unauthorized", err: &openai.Error{StatusCode: http.StatusUnauthorized}, want: provider.StatusAuthRequired},
		{name: "forbidden", err: &openai.Error{StatusCode: http.StatusForbidden}, want: provider.StatusAuthRequired},
		{name: "error", err: errors.New("test"), want: provider.StatusError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := provider.StatusFromError(test.err)
			if got != test.want {
				t.Errorf("StatusFromError(%v) = %v, want %v", test.err, got, test.want)
			}
		})
	}
}
