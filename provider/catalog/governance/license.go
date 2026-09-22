package governance

import (
	"encoding/json"
	"slices"
)

type LicensePolicy struct {
	constraints *LicensePolicyConstraints
}

type LicensePolicyConstraints struct {
	Includes []string `json:"includes"`
	Excludes []string `json:"excludes"`
}

func NewLicensePolicy(policy Policy) (*LicensePolicy, error) {
	constraints := &LicensePolicyConstraints{}
	if err := json.Unmarshal(policy.Constraints, constraints); err != nil {
		return nil, err
	}
	return &LicensePolicy{constraints: constraints}, nil
}

func (p *LicensePolicy) PolicyType() PolicyType {
	return PolicyTypeLicense
}

// TODO: what do we do if the license is not available?
func (p *LicensePolicy) IsRestricted(subject Subject) bool {
	license := subject.LicenseName()

	if len(p.constraints.Includes) > 0 && !slices.Contains(p.constraints.Includes, license) {
		return true
	}

	if slices.Contains(p.constraints.Excludes, license) {
		return true
	}

	return false
}
