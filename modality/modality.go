package modality

import (
	"database/sql/driver"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Modalities are bitmasks that allow us to specify multimodal types using a single
// type and binary operations. For example inputModality := Text | Image gives you a
// multimodal input modality of text and image.
//
// For future proofing purposes, we allow up to 16 different modalities using a uint16
// bitmask. Right now there are only 9 modalities, leaving room for an additional 7.
//
// In databases, store modalities as a SMALLINT (16 bits) for maximum space storage.
// This package handles the conversion to and from the int16 type used in database
// storage to the uint16 type used in the codebase.
type Modality uint16

const ZeroModality = Modality(0)

type NullModality struct {
	Modality Modality
	Valid    bool
}

const (
	Text Modality = 1 << iota
	Image
	Audio
	Video
	Document
	Spatial
	TimeSeries
	Robotic
	Genomic
)

var modes = [9]Modality{
	Text,
	Image,
	Audio,
	Video,
	Document,
	Spatial,
	TimeSeries,
	Robotic,
	Genomic,
}

var names = [9]string{
	"Text",
	"Image",
	"Audio",
	"Video",
	"Document",
	"Spatial",
	"TimeSeries",
	"Robotic",
	"Genomic",
}

var namesMap = map[string]Modality{
	"text":       Text,
	"image":      Image,
	"audio":      Audio,
	"video":      Video,
	"document":   Document,
	"spatial":    Spatial,
	"timeseries": TimeSeries,
	"robotic":    Robotic,
	"genomic":    Genomic,
}

func Parse(val any) (Modality, error) {
	switch v := val.(type) {
	case string:
		arr := strings.Split(v, "|")
		return parseArray(arr)
	case []any:
		arr := make([]string, len(v))
		for i, v := range v {
			var ok bool
			if arr[i], ok = v.(string); !ok {
				return 0, fmt.Errorf("cannot parse %T into Modality", v)
			}
		}
		return parseArray(arr)
	case []string:
		return parseArray(v)
	case []byte:
		m := Modality(0)
		if err := m.UnmarshalBinary(v); err != nil {
			return 0, err
		}
		return m, nil
	case uint8:
		return Modality(v), nil
	case uint16:
		return Modality(v), nil
	case uint32:
		return Modality(v), nil
	case uint64:
		return Modality(v), nil
	case int8:
		return Modality(v), nil
	case int16:
		return Modality(v), nil
	case int32:
		return Modality(v), nil
	case int64:
		return Modality(v), nil
	case int:
		return Modality(v), nil
	case uint:
		return Modality(v), nil
	default:
		return 0, fmt.Errorf("cannot parse %T into Modality", val)
	}
}

func parseArray(arr []string) (Modality, error) {
	m := Modality(0)
	for _, v := range arr {
		if s := strings.TrimSpace(strings.ToLower(v)); s != "" {
			if n, ok := namesMap[s]; ok {
				m |= n
			} else {
				return Modality(0), fmt.Errorf("invalid modality %q", v)
			}
		}
	}
	return Modality(m), nil
}

func (m Modality) String() string {
	if m == 0 {
		return "None"
	}

	sb := strings.Builder{}
	prev := false

	for i, mode := range modes {
		if m&mode != 0 {
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

func (m Modality) IsText() bool {
	return m&Text != 0
}

func (m Modality) IsImage() bool {
	return m&Image != 0
}

func (m Modality) IsAudio() bool {
	return m&Audio != 0
}

func (m Modality) IsVideo() bool {
	return m&Video != 0
}

func (m Modality) IsDocument() bool {
	return m&Document != 0
}

func (m Modality) IsSpatial() bool {
	return m&Spatial != 0
}

func (m Modality) IsTimeSeries() bool {
	return m&TimeSeries != 0
}

func (m Modality) IsRobotic() bool {
	return m&Robotic != 0
}

func (m Modality) IsGenomic() bool {
	return m&Genomic != 0
}

func (m Modality) IsZero() bool {
	return m == 0
}

// Return the set of modalities for easier validation checking.
func (m Modality) Kinds() map[Modality]struct{} {
	set := make(map[Modality]struct{})
	for _, mode := range modes {
		if m&mode != 0 {
			set[mode] = struct{}{}
		}
	}
	return set
}

// --- Conversion Helpers ---

func (m Modality) NullModality() NullModality {
	return NullModality{Modality: m, Valid: m > 0}
}

func (n NullModality) ToModality() (m Modality, ok bool) {
	return n.Modality, n.Valid
}

//============================================================================
// Database Methods
//============================================================================

// Scan implements [sql.Scanner] for database storage and expects an unsigned integer
// value. Use SMALLINT (16 bit integers) for database storage.
func (m *Modality) Scan(src any) (err error) {
	if src == nil {
		*m = 0
		return nil
	}

	// Database drivers can return integers in various types.
	switch v := src.(type) {
	case []byte:
		var i int
		if i, err = strconv.Atoi(string(v)); err != nil {
			return err
		}
		*m = Modality(i)
	case int16:
		*m = Modality(v)
	case int32:
		*m = Modality(v)
	case int64:
		*m = Modality(v)
	case uint16:
		*m = Modality(v)
	case uint32:
		*m = Modality(v)
	case uint64:
		*m = Modality(v)
	case int:
		*m = Modality(v)
	case uint:
		*m = Modality(v)
	default:
		return fmt.Errorf("cannot scan %T into Modality", src)
	}

	return nil
}

// Driver implements [driver.Valuer] for database storage and returns a signed integer
// value. Use SMALLINT (16 bit integers) for database storage.
func (m Modality) Value() (driver.Value, error) {
	return int64(m), nil
}

// Scan implements [sql.Scanner] for database storage and expects an unsigned integer
// value. Use SMALLINT (16 bit integers) for database storage. This struct is used to
// store nullable modalities in the database.
func (n *NullModality) Scan(src any) (err error) {
	if src == nil {
		n.Modality, n.Valid = 0, false
		return nil
	}

	if err = n.Modality.Scan(src); err != nil {
		return err
	}

	n.Valid = n.Modality > 0
	return nil
}

// Driver implements [driver.Valuer] for database storage and returns a signed integer
// value. Use SMALLINT (16 bit integers) for database storage. This struct is used to
// store nullable modalities in the database.
func (n NullModality) Value() (driver.Value, error) {
	if !n.Valid {
		return nil, nil
	}
	return n.Modality.Value()
}

//============================================================================
// Serialization and Deserialization
//============================================================================

// MarshalJSON returns the modality as a string array containing the names of the
// modalities that are set. If the modality is zero valued then an empty array is
// returned. Even if there is only one modality set, an array is returned.
func (m Modality) MarshalJSON() ([]byte, error) {
	val := make([]string, 0, len(modes))
	for _, mode := range modes {
		if m&mode != 0 {
			val = append(val, mode.String())
		}
	}
	return json.Marshal(val)
}

// UnmarshalJSON parses string arrays and strings into a modality.
func (m *Modality) UnmarshalJSON(data []byte) (err error) {
	// Parse data into a primitive type e.g. string or string array
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	var n Modality
	if n, err = Parse(v); err != nil {
		return err
	}

	*m = n
	return nil
}

// Returns the varint encoded binary representation of the modality.
func (m Modality) MarshalBinary() ([]byte, error) {
	buf := make([]byte, binary.MaxVarintLen16)
	n := binary.PutUvarint(buf, uint64(m))
	return buf[:n], nil
}

// Decodes the varint encoded binary representation of the modality.
func (m *Modality) UnmarshalBinary(data []byte) error {
	v, n := binary.Uvarint(data)
	if n <= 0 {
		return fmt.Errorf("invalid varint encoding: %d", n)
	}
	*m = Modality(v)
	return nil
}

// Returns the String representation of the modality.
func (m Modality) MarshalText() ([]byte, error) {
	return []byte(m.String()), nil
}

// Parses the String representation of the modality.
func (m *Modality) UnmarshalText(data []byte) (err error) {
	var o Modality
	if o, err = Parse(string(data)); err != nil {
		return err
	}
	*m = o
	return nil
}

// UnmarshalParam implements Gin's BindUnmarshaler so form/query binding accepts
// modality names (e.g. "text") instead of only numeric bitmask values.
func (m *Modality) UnmarshalParam(param string) error {
	return m.UnmarshalText([]byte(param))
}
