package governance

import (
	"encoding/json"
)

type ModelPolicy struct {
	constraints *ModelPolicyConstraints
}

type ModelPolicyConstraints struct {
	Parameters          *Int32Constraint `json:"parameters,omitempty"`
	ContextSize         *Int32Constraint `json:"context_size,omitempty"`
	TokenCostPerMillion *FloatConstraint `json:"token_cost_per_million,omitempty"`
}

func NewModelPolicy(policy Policy) (*ModelPolicy, error) {
	constraints := &ModelPolicyConstraints{}
	if err := json.Unmarshal(policy.Constraints, constraints); err != nil {
		return nil, err
	}
	return &ModelPolicy{constraints: constraints}, nil
}

func (p *ModelPolicy) PolicyType() PolicyType {
	return PolicyTypeModel
}

// TODO: what to do if a parameter we're checking for is not available?
func (p *ModelPolicy) IsRestricted(subject Subject) bool {
	// TODO: Determine parameter size from model info

	if p.constraints.ContextSize != nil {
		if contextSize, ok := subject.ContextSizeValue(); ok && p.constraints.ContextSize.IsRestricted(contextSize) {
			return true
		}
	}

	if p.constraints.TokenCostPerMillion != nil {
		if inputCost, ok := subject.InputCostValue(); ok && p.constraints.TokenCostPerMillion.IsRestricted(inputCost) {
			return true
		}
	}
	return false
}
