package governance_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.rtnl.ai/horizon/provider/catalog"
	"go.rtnl.ai/horizon/provider/catalog/governance"
)

// TestLicensePolicyRestricted verifies the testlicensepolicyrestricted behavior covered by this test.
func TestLicensePolicyRestricted(t *testing.T) {
	t.Run("RestrictedIncludes", func(t *testing.T) {
		policy, err := governance.NewLicensePolicy(governance.Policy{
			Constraints: []byte(`{"includes": ["MIT", "Apache-2.0", "GPL-3.0"]}`),
		})
		require.NoError(t, err)
		require.True(t, policy.IsRestricted(&catalog.Model{License: "commercial"}))
	})
	t.Run("RestrictedExcludes", func(t *testing.T) {
		policy, err := governance.NewLicensePolicy(governance.Policy{
			Constraints: []byte(`{"excludes": ["MIT", "Apache-2.0", "GPL-3.0"]}`),
		})
		require.NoError(t, err)
		require.True(t, policy.IsRestricted(&catalog.Model{License: "MIT"}))
	})
	t.Run("Unrestricted", func(t *testing.T) {
		policy, err := governance.NewLicensePolicy(governance.Policy{
			Constraints: []byte(`{"includes": ["MIT", "Apache-2.0", "GPL-3.0"]}`),
		})
		require.NoError(t, err)
		require.False(t, policy.IsRestricted(&catalog.Model{License: "MIT"}))
	})
}
