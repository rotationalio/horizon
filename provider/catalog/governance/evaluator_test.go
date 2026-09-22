package governance_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.rtnl.ai/horizon/provider/catalog"
	"go.rtnl.ai/horizon/provider/catalog/governance"
)

// TestNewEvaluator verifies the testnewevaluator behavior covered by this test.
func TestNewEvaluator(t *testing.T) {
	t.Run("InvalidPolicyType", func(t *testing.T) {
		_, err := governance.NewEvaluator(governance.Policy{
			PolicyType: governance.PolicyTypeUnknown,
		})
		require.Error(t, err)
	})

	t.Run("LicensePolicy", func(t *testing.T) {
		evaluator, err := governance.NewEvaluator(governance.Policy{
			PolicyType:  governance.PolicyTypeLicense,
			Constraints: []byte(`{"includes": ["MIT"]}`),
		})
		require.NoError(t, err)
		require.IsType(t, &governance.LicensePolicy{}, evaluator)
		require.False(t, evaluator.IsRestricted(&catalog.Model{License: "MIT"}))
		require.True(t, evaluator.IsRestricted(&catalog.Model{License: "commercial"}))
	})

	t.Run("ModelPolicy", func(t *testing.T) {
		evaluator, err := governance.NewEvaluator(governance.Policy{
			PolicyType:  governance.PolicyTypeModel,
			Constraints: []byte(`{"context_size": {"max": 1000}}`),
		})
		require.NoError(t, err)
		require.IsType(t, &governance.ModelPolicy{}, evaluator)
		contextSize512 := int32(512)
		require.False(t, evaluator.IsRestricted(&catalog.Model{ContextSize: &contextSize512}))
		contextSize1024 := int32(1024)
		require.True(t, evaluator.IsRestricted(&catalog.Model{ContextSize: &contextSize1024}))
	})
}
