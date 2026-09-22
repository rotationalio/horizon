package governance

import "fmt"

// PolicyType identifies the kind of catalog governance policy.
type PolicyType uint8

const (
	PolicyTypeUnknown PolicyType = iota
	PolicyTypeLicense
	PolicyTypeModel
)

func (p PolicyType) String() string {
	switch p {
	case PolicyTypeLicense:
		return "license"
	case PolicyTypeModel:
		return "model"
	default:
		return fmt.Sprintf("unknown(%d)", p)
	}
}

type Int32Constraint struct {
	Min *int32 `json:"min,omitempty"`
	Max *int32 `json:"max,omitempty"`
}

func (c *Int32Constraint) IsRestricted(value int32) bool {
	return c != nil && ((c.Min != nil && value < *c.Min) || (c.Max != nil && value > *c.Max))
}

type FloatConstraint struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

func (c *FloatConstraint) IsRestricted(value float64) bool {
	return c != nil && ((c.Min != nil && value < *c.Min) || (c.Max != nil && value > *c.Max))
}
