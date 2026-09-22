package catalog

import (
	"go.rtnl.ai/horizon/provider/catalog/governance"
)

// ApplyPolicies evaluates governance policies against a catalog model and annotates
// restriction state on the model.
func ApplyPolicies(model *Model, policies ...governance.Policy) error {
	for _, policy := range policies {
		evaluator, err := governance.NewEvaluator(policy)
		if err != nil {
			return err
		}
		if evaluator.IsRestricted(model) {
			model.Restricted = true
			model.Policies = append(model.Policies, policy)
		}
	}
	return nil
}

// ApplyAll evaluates governance policies against each catalog model and annotates
// restriction state on the models.
func ApplyAll(models []Model, policies ...governance.Policy) ([]Model, error) {
	if len(policies) == 0 {
		return models, nil
	}

	for i := range models {
		if err := ApplyPolicies(&models[i], policies...); err != nil {
			return nil, err
		}
	}
	return models, nil
}
