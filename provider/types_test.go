package provider_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/provider"
)

type stringAlias string

// Verifies non-empty strings, defined string types, and string pointers are
// normalized while empty and non-string values are omitted.
func TestMetaPutString(t *testing.T) {
	meta := make(provider.Meta)
	alias := stringAlias("alias")

	meta.PutString("string", "value")
	meta.PutString("alias", alias)
	meta.PutString("pointer", &alias)
	meta.PutString("empty", "")
	meta.PutString("integer", 42)
	meta.PutString("nil", nil)

	require.Equal(t, provider.Meta{
		"string":  "value",
		"alias":   "alias",
		"pointer": "alias",
	}, meta)
}
