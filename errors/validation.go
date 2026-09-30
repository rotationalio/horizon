package errors

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/x/validation"
)

// Asserts err is a ValidationErrors value containing exactly the specified
// field keys.
// TODO: remove and replace calls to this with go.rtnl.ai/x/validation helpers once they are merged in
func RequireValidationFields(t *testing.T, err error, fields ...string) {
	t.Helper()

	require.Error(t, err)
	var verr validation.Errors
	require.ErrorAs(t, err, &verr)
	require.Len(t, verr, len(fields))

	got := verr.Map()
	for _, field := range fields {
		require.Contains(t, got, field)
	}
}

// Asserts err is a ValidationErrors value containing exactly the specified
// field keys.
// TODO: remove and replace calls to this with go.rtnl.ai/x/validation helpers once they are merged in
func RequireValidation(t *testing.T, err error, target *validation.FieldError, msgAndArgs ...interface{}) {
	t.Helper()

	require.Error(t, err, msgAndArgs...)

	var verr validation.Errors
	require.ErrorAs(t, err, &verr, msgAndArgs...)

	found := false
	for _, e := range verr {
		if target.Equal(e) {
			found = true
			break
		}
	}

	if !found {
		require.Fail(t, "expected validation error for field "+target.Field(), msgAndArgs...)
	}
}
