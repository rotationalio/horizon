package prompts

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

const DefaultContextKey = "context"

//============================================================================
// Context Object
//============================================================================

// A context is a map of key-value pairs that are used to render a prompt template.
type Context map[string]any

// Returns true if the context is a single string value with the default key.
func (c Context) IsText() bool {
	if len(c) == 1 {
		if val, ok := c[DefaultContextKey]; ok {
			if _, ok := val.(string); ok {
				return true
			}
		}
	}
	return false
}

// Returns the string value if the context is a single string value with the default key.
// Otherwise returns the context as an indented JSON string.
func (c Context) String() (_ string, err error) {
	if c.IsText() {
		return c[DefaultContextKey].(string), nil
	}

	var data []byte
	if data, err = json.MarshalIndent(c, "", "  "); err != nil {
		return "", err
	}
	return string(data), nil
}

func (c Context) MarshalJSON() ([]byte, error) {
	if c.IsText() {
		return json.Marshal(c[DefaultContextKey])
	}
	return json.Marshal(map[string]any(c))
}

func (c *Context) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	switch v := v.(type) {
	case string:
		*c = Context{DefaultContextKey: v}
	case map[string]any:
		*c = Context(v)
	default:
		return fmt.Errorf("cannot unmarshal %T into context", v)
	}

	return nil
}

func (c *Context) Scan(src interface{}) (err error) {
	switch v := src.(type) {
	case nil:
		return nil
	case []byte:
		return json.Unmarshal(v, &c)
	case string:
		return json.Unmarshal([]byte(v), &c)
	}

	return fmt.Errorf("cannot scan %T into context", src)
}

func (c Context) Value() (driver.Value, error) {
	return json.Marshal(c)
}
