package catalog_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.rtnl.ai/horizon/provider/catalog"
	"go.rtnl.ai/horizon/provider/catalog/governance"
	"go.rtnl.ai/ulid"
)

// TestApplyPolicies verifies the testapplypolicies behavior covered by this test.
func TestApplyPolicies(t *testing.T) {
	policy := governance.Policy{
		ID:          ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV"),
		Name:        "exclude MIT",
		PolicyType:  governance.PolicyTypeLicense,
		Constraints: []byte(`{"excludes": ["MIT"]}`),
	}

	model := catalog.Model{License: "MIT"}
	require.NoError(t, catalog.ApplyPolicies(&model, policy))
	require.True(t, model.Restricted)
	require.Equal(t, []governance.Policy{policy}, model.Policies)
}

// TestApplyAll verifies the testapplyall behavior covered by this test.
func TestApplyAll(t *testing.T) {
	policy := governance.Policy{
		ID:          ulid.MustParse("01ARZ3NDEKTSV4RRFFQ69G5FAV"),
		Name:        "MIT only",
		PolicyType:  governance.PolicyTypeLicense,
		Constraints: []byte(`{"includes": ["MIT"]}`),
	}

	models := []catalog.Model{
		{Slug: "allowed", License: "MIT"},
		{Slug: "restricted", License: "commercial"},
	}

	out, err := catalog.ApplyAll(models, policy)
	require.NoError(t, err)
	require.False(t, out[0].Restricted)
	require.Empty(t, out[0].Policies)
	require.True(t, out[1].Restricted)
	require.Equal(t, []governance.Policy{policy}, out[1].Policies)
}
