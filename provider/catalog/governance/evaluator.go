package governance

import (
	"fmt"
)

// Evaluator evaluates whether a catalog model is restricted by a governance policy.
type Evaluator interface {
	PolicyType() PolicyType
	IsRestricted(subject Subject) bool
}

// NewEvaluator parses a policy into an evaluator for the given policy type.
func NewEvaluator(policy Policy) (Evaluator, error) {
	switch policy.PolicyType {
	case PolicyTypeLicense:
		return NewLicensePolicy(policy)
	case PolicyTypeModel:
		return NewModelPolicy(policy)
	default:
		return nil, fmt.Errorf("invalid policy type: %s", policy.PolicyType)
	}
}
