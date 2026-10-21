// Package metadata defines application-specific metadata values.
package metadata

import "slices"

// The maximum depth to clone nested [Metadata] containers.
const MaxCloneDepth = 16

// Cloner provides custom cloning for a metadata value. Implementations should
// return a clone with the same concrete type as the receiver.
type Cloner interface {
	Clone() any
}

// Metadata is a user-defined set of key-value pairs.
type Metadata map[string]any

// Clone recursively copies common JSON-like metadata containers (slices, maps)
// while retaining other types as-is. Copies up to MaxCloneDepth levels deep,
// then returns the original value for any nested containers.
func (m Metadata) Clone() Metadata {
	if m == nil {
		return nil
	}
	return cloneValue(m, 0).(Metadata)
}

// cloneValue copies a value, recursively copying any nested maps or slices.
func cloneValue(value any, depth int) any {
	if depth > MaxCloneDepth {
		return value
	}

	switch value := value.(type) {
	case Metadata:
		if value == nil {
			return value
		}
		return Metadata(cloneValue(map[string]any(value), depth).(map[string]any))
	case map[string]any:
		if value == nil {
			return value
		}
		cloned := make(map[string]any, len(value))
		for key, nested := range value {
			cloned[key] = cloneValue(nested, depth+1)
		}
		return cloned
	case []any:
		cloned := make([]any, len(value))
		for i, nested := range value {
			cloned[i] = cloneValue(nested, depth+1)
		}
		return cloned
	case []string:
		return slices.Clone(value)
	case []byte:
		return slices.Clone(value)
	default:
		if cloner, ok := value.(Cloner); ok {
			return cloner.Clone()
		}
		return value
	}
}
