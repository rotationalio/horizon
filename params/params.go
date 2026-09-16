package params

import (
	"encoding/json"
	"fmt"
	"strconv"

	"go.rtnl.ai/x/typecase"
)

// Params is a map of key-value pairs that can be used to infer parameters
// that should be passed to the LLM via the provider's API. In order to make searching
// case and form insensitive, the keys are normalized before lookup and the values set
// to their largest native type. Key normalization ensures that all keys are in a lower,
// snake_case format. Normalization requires that input keys are in either kebab-case,
// snake_case, or camelCase format. Any spaces in a key are replaced with underscores.
//
// When creating a new params map, the keys and values are normalized, but any
// modifications to the map should use the Set and Get methods to ensure the params
// can be used correctly by the LLM client.
type Params struct {
	params map[string]any
}

// New creates a new params map from a map of key-value pairs, normalizing all keys to
// facilitate future lookups (ensuring case and form insensitive matching).
//
// NOTE: if the input map contains multiple keys that normalize to the same value,
// the last value will be stored in the params map.
func New(obj map[string]any) *Params {
	params := &Params{
		params: make(map[string]any, len(obj)),
	}

	for key, value := range obj {
		params.Set(key, value)
	}

	return params
}

// Set a key-value pair in the params map, normalizing the key before storing it.
func (p *Params) Set(key string, value any) {
	p.params[normalize(key)] = value
}

// Get a value from the params map, normalizing the key before looking it up.
func (p *Params) Get(key string) (val any, ok bool) {
	val, ok = p.params[normalize(key)]
	return
}

// Del a key-value pair from the params map, normalizing the key before deleting it.
func (p *Params) Del(key string) {
	delete(p.params, normalize(key))
}

// Len returns the number of key-value pairs in the params map.
func (p *Params) Len() int {
	return len(p.params)
}

// Bool returns the boolean value of the key, normalizing the key before looking it up.
// If the key is not found or the bool cannot be parsed, ok will be false.
func (p *Params) Bool(key string) (_ bool, ok bool) {
	var val any
	if val, ok = p.Get(key); !ok {
		return false, false
	}

	switch v := val.(type) {
	case bool:
		return v, true
	case string:
		if parsed, err := strconv.ParseBool(v); err == nil {
			return parsed, true
		}
		return false, false
	default:
		return false, false
	}
}

// String returns the string value of the key, normalizing the key before looking it up.
// If the key is not found or the string cannot be parsed, ok will be false. If the
// value is not a string or a fmt.Stringer, ok will be false.
func (p *Params) String(key string) (_ string, ok bool) {
	var val any
	if val, ok = p.Get(key); !ok {
		return "", false
	}

	switch v := val.(type) {
	case string:
		return v, true
	case fmt.Stringer:
		return v.String(), true
	default:
		return "", false
	}
}

// Int returns the int64 value of the key, normalizing the key before looking it up.
// If the key is not found or the int64 cannot be parsed, ok will be false.
//
// NOTE: uint64 values may be converted to negative int64 values if they are too large.
// NOTE: float values are truncated to int64 values.
func (p *Params) Int(key string) (_ int64, ok bool) {
	var val any
	if val, ok = p.Get(key); !ok {
		return 0, false
	}

	switch v := val.(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case uint:
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		return int64(v), true
	case json.Number:
		if parsed, err := v.Int64(); err == nil {
			return parsed, true
		}
		return 0, false
	case string:
		if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
			return parsed, true
		}
		return 0, false
	case float32:
		return int64(v), true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

// Float returns the float64 value of the key, normalizing the key before looking it up.
// If the key is not found or the float64 cannot be parsed, ok will be false.
func (p *Params) Float(key string) (_ float64, ok bool) {
	var val any
	if val, ok = p.Get(key); !ok {
		return 0, false
	}

	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case json.Number:
		if parsed, err := v.Float64(); err == nil {
			return parsed, true
		}
		return 0, false
	case string:
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			return parsed, true
		}
		return 0, false
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	default:
		return 0, false
	}
}

// Normalize keys ensuring that they are in a lower, snake_case format.
func normalize(key string) string {
	return typecase.Snake(key)
}

//============================================================================
// Serialization
//============================================================================

func (p *Params) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.params)
}

func (p *Params) UnmarshalJSON(data []byte) error {
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}

	for key, value := range obj {
		p.Set(key, value)
	}

	return nil
}
