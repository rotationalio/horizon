package governance_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.rtnl.ai/horizon/provider/catalog"
	"go.rtnl.ai/horizon/provider/catalog/governance"
)

// TestModelPolicyRestricted verifies the testmodelpolicyrestricted behavior covered by this test.
func TestModelPolicyRestricted(t *testing.T) {
	t.Run("RestrictedContextSize", func(t *testing.T) {
		policy, err := governance.NewModelPolicy(governance.Policy{
			Constraints: []byte(`{"context_size": {"max": 1000}}`),
		})
		require.NoError(t, err)
		contextSize := int32(1024)
		require.True(t, policy.IsRestricted(&catalog.Model{ContextSize: &contextSize}))
	})

	t.Run("RestrictedTokenCostPerMillion", func(t *testing.T) {
		policy, err := governance.NewModelPolicy(governance.Policy{
			Constraints: []byte(`{"token_cost_per_million": {"max": 0.5}}`),
		})
		require.NoError(t, err)
		inputCost := 1.0
		require.True(t, policy.IsRestricted(&catalog.Model{InputCost: &inputCost}))
	})

	t.Run("Unrestricted", func(t *testing.T) {
		policy, err := governance.NewModelPolicy(governance.Policy{
			Constraints: []byte(`{"parameters": {"max": 1000}}`),
		})
		require.NoError(t, err)
		contextSize := int32(512)
		require.False(t, policy.IsRestricted(&catalog.Model{ContextSize: &contextSize}))
	})
}
