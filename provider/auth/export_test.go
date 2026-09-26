package auth

// NewForTest constructs credentials with an explicit auth type (tests only).
func NewForTest(typ Type) *Credentials {
	return &Credentials{wire: credswire{Type: typ}}
}
