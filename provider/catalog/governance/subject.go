package governance

// Subject is the catalog model metadata evaluated by governance policies.
type Subject interface {
	LicenseName() string
	ContextSizeValue() (int32, bool)
	InputCostValue() (float64, bool)
}
