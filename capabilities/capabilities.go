// Package capabilities defines the host-neutral contracts for model-requested
// capabilities in Horizon.
package capabilities

import (
	"fmt"
	"regexp"

	"go.rtnl.ai/horizon/schema"
)

// Allows 'A-Z', 'a-z', '0-9', '_', and '-' with a length between 1 and 64.
var namePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Name identifies a capability available to a task.
type Name string

// Validate checks that the name contains 1 to 64 ASCII letters, digits,
// underscores, or hyphens.
func (n Name) Validate() error {
	if !namePattern.MatchString(string(n)) {
		return fmt.Errorf("%w: %q", ErrInvalidName, n)
	}
	return nil
}

// Definition describes a capability that can be advertised to an inference
// provider.
type Definition struct {
	Name        Name
	Description string
	InputSchema *schema.Schema
}

// Request is a validated capability request passed to the host executor.
type Request struct {
	CallID    string
	Name      Name
	Arguments map[string]any
}
