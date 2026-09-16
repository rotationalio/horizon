package render_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/render"
)

func TestValidate(t *testing.T) {
	t.Run("Sprintf", func(t *testing.T) {
		require.NoError(t, render.Validate(render.Sprintf, "hello world %d times!"), "expected valid template to pass validation")
		require.Error(t, render.Validate(render.Sprintf, "hello world"), "expected invalid template to fail validation")
	})

	t.Run("Go", func(t *testing.T) {
		require.NoError(t, render.Validate(render.Go, "hello world {{ .Count }} times!"), "expected valid template to pass validation")
		require.Error(t, render.Validate(render.Go, "hello world {{ if }}"), "expected invalid template to fail validation")
	})

	t.Run("Unknown", func(t *testing.T) {
		require.Error(t, render.Validate(render.Unknown, "hello world"), "expected invalid template to fail validation")
	})

	t.Run("InvalidType", func(t *testing.T) {
		require.Error(t, render.Validate(42, "hello world"), "expected invalid template to fail validation")
	})
}
