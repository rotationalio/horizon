package catalog

import (
	"database/sql/driver"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Capabilities are bitmasks that specify generic model capabilities for easier
// filtering as documented by the model provider.
type ModelCapability uint16

const ZeroModelCapability = ModelCapability(0)

type NullModelCapability struct {
	Capability ModelCapability
	Valid      bool
}

const (
	Tools ModelCapability = 1 << iota
	Reasoning
	WebSearch
)

var capabilities = [3]ModelCapability{
	Tools,
	Reasoning,
	WebSearch,
}

var names = [3]string{
	"Tools",
	"Reasoning",
	"WebSearch",
}

var namesMap = map[string]ModelCapability{
	"tools":     Tools,
	"reasoning": Reasoning,
	"websearch": WebSearch,
}

func Parse(val any) (ModelCapability, error) {
	switch v := val.(type) {
	case string:
		arr := strings.Split(v, "|")
		return parseArray(arr)
	case []any:
		arr := make([]string, len(v))
		for i, v := range v {
			var ok bool
			if arr[i], ok = v.(string); !ok {
				return 0, fmt.Errorf("cannot parse %T into ModelCapability", v)
			}
		}
		return parseArray(arr)
	case []string:
		return parseArray(v)
	case []byte:
		c := ModelCapability(0)
		if err := c.UnmarshalBinary(v); err != nil {
			return 0, err
		}
		return c, nil
	case uint8:
		return ModelCapability(v), nil
	case uint16:
		return ModelCapability(v), nil
	case uint32:
		return ModelCapability(v), nil
	case uint64:
		return ModelCapability(v), nil
	case int8:
		return ModelCapability(v), nil
	case int16:
		return ModelCapability(v), nil
	case int32:
		return ModelCapability(v), nil
	case int64:
		return ModelCapability(v), nil
	case int:
		return ModelCapability(v), nil
	case uint:
		return ModelCapability(v), nil
	default:
		return 0, fmt.Errorf("cannot parse %T into ModelCapability", val)
	}
}

func parseArray(arr []string) (ModelCapability, error) {
	c := ModelCapability(0)
	for _, v := range arr {
		if s := strings.TrimSpace(strings.ToLower(v)); s != "" {
			if n, ok := namesMap[s]; ok {
				c |= n
			} else {
				return ModelCapability(0), fmt.Errorf("invalid capability %q", v)
			}
		}
	}
	return ModelCapability(c), nil
}

func (c ModelCapability) String() string {
	if c == 0 {
		return "None"
	}

	sb := strings.Builder{}
	prev := false

	for i, capability := range capabilities {
		if c&capability != 0 {
			if prev {
				sb.WriteString(" | ")
			}
			sb.WriteString(names[i])
			prev = true
		}
	}

	s := sb.String()
	if s == "" {
		return "Unknown"
	}
	return s
}

func (c ModelCapability) IsTools() bool {
	return c&Tools != 0
}

func (c ModelCapability) IsReasoning() bool {
	return c&Reasoning != 0
}

func (c ModelCapability) IsWebSearch() bool {
	return c&WebSearch != 0
}

func (c ModelCapability) IsZero() bool {
	return c == 0
}

// Return the set of capabilities for easier validation checking.
func (c ModelCapability) Kinds() map[ModelCapability]struct{} {
	set := make(map[ModelCapability]struct{})
	for _, capability := range capabilities {
		if c&capability != 0 {
			set[capability] = struct{}{}
		}
	}
	return set
}

// --- Conversion Helpers ---

func (c ModelCapability) NullCapability() NullModelCapability {
	return NullModelCapability{Capability: c, Valid: c > 0}
}

func (n NullModelCapability) ToCapability() (c ModelCapability, ok bool) {
	return n.Capability, n.Valid
}

//============================================================================
// Database Methods
//============================================================================

// Scan implements [sql.Scanner] for database storage and expects an unsigned integer
// value. Use SMALLINT (16 bit integers) for database storage.
func (c *ModelCapability) Scan(src any) (err error) {
	if src == nil {
		*c = 0
		return nil
	}

	// Database drivers can return integers in various types.
	switch v := src.(type) {
	case []byte:
		var i int
		if i, err = strconv.Atoi(string(v)); err != nil {
			return err
		}
		*c = ModelCapability(i)
	case int16:
		*c = ModelCapability(v)
	case int32:
		*c = ModelCapability(v)
	case int64:
		*c = ModelCapability(v)
	case uint16:
		*c = ModelCapability(v)
	case uint32:
		*c = ModelCapability(v)
	case uint64:
		*c = ModelCapability(v)
	case int:
		*c = ModelCapability(v)
	case uint:
		*c = ModelCapability(v)
	default:
		return fmt.Errorf("cannot scan %T into ModelCapability", src)
	}

	return nil
}

// Driver implements [driver.Valuer] for database storage and returns a signed integer
// value. Use SMALLINT (16 bit integers) for database storage.
func (c ModelCapability) Value() (driver.Value, error) {
	return int64(c), nil
}

// Scan implements [sql.Scanner] for database storage and expects an unsigned integer
// value. Use SMALLINT (16 bit integers) for database storage. This struct is used to
// store nullable capabilities in the database.
func (n *NullModelCapability) Scan(src any) (err error) {
	if src == nil {
		n.Capability, n.Valid = 0, false
		return nil
	}

	if err = n.Capability.Scan(src); err != nil {
		return err
	}

	n.Valid = n.Capability > 0
	return nil
}

// Driver implements [driver.Valuer] for database storage and returns a signed integer
// value. Use SMALLINT (16 bit integers) for database storage. This struct is used to
// store nullable capabilities in the database.
func (n NullModelCapability) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.Capability.Value()
}

//============================================================================
// Serialization and Deserialization
//============================================================================

// MarshalJSON returns the capability as a string array containing the names of the
// capabilities that are set. If the capability is zero valued then an empty array is
// returned. Even if there is only one capability set, an array is returned.
func (c ModelCapability) MarshalJSON() ([]byte, error) {
	val := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		if c&capability != 0 {
			val = append(val, capability.String())
		}
	}
	return json.Marshal(val)
}

// UnmarshalJSON parses string arrays and strings into a capability.
func (c *ModelCapability) UnmarshalJSON(data []byte) (err error) {
	// Parse data into a primitive type e.g. string or string array
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	var n ModelCapability
	if n, err = Parse(v); err != nil {
		return err
	}

	*c = n
	return nil
}

// Returns the varint encoded binary representation of the capability.
func (c ModelCapability) MarshalBinary() ([]byte, error) {
	buf := make([]byte, binary.MaxVarintLen16)
	n := binary.PutUvarint(buf, uint64(c))
	return buf[:n], nil
}

// Decodes the varint encoded binary representation of the capability.
func (c *ModelCapability) UnmarshalBinary(data []byte) error {
	v, n := binary.Uvarint(data)
	if n <= 0 {
		return fmt.Errorf("invalid varint encoding: %d", n)
	}
	*c = ModelCapability(v)
	return nil
}

// Returns the String representation of the capability.
func (c ModelCapability) MarshalText() ([]byte, error) {
	return []byte(c.String()), nil
}

// Parses the String representation of the capability.
func (c *ModelCapability) UnmarshalText(data []byte) (err error) {
	var o ModelCapability
	if o, err = Parse(string(data)); err != nil {
		return err
	}
	*c = o
	return nil
}

// UnmarshalParam implements Gin's BindUnmarshaler so form/query binding accepts
// capability names (e.g. "tools") instead of only numeric bitmask values.
func (m *ModelCapability) UnmarshalParam(param string) error {
	return m.UnmarshalText([]byte(param))
}
