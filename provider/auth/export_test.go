package auth

// NewForTest constructs credentials with an explicit auth type (tests only).
func NewForTest(typ Type) *Credentials {
	return &Credentials{wire: credswire{Type: typ}}
}

// NewUnnormalizedForTest constructs credentials with a raw API key for validation tests.
func NewUnnormalizedForTest(typ Type, apiKey string) *Credentials {
	return &Credentials{wire: credswire{Type: typ, APIKey: apiKey}}
}
