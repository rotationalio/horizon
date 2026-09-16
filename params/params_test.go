package params_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/params"
)

func TestNew(t *testing.T) {
	t.Run("Nil", func(t *testing.T) {
		p := params.New(nil)
		require.Equal(t, 0, p.Len())
	})

	t.Run("Empty", func(t *testing.T) {
		p := params.New(map[string]any{})
		require.Equal(t, 0, p.Len())
	})

	t.Run("Values", func(t *testing.T) {
		orig := map[string]any{
			"IceCreamCone":   uint(1),
			"ice_cream_cone": int32(2),
			"ice-cream-cone": int16(3),
			"foo":            true,
			"bar":            "zap",
			"Bar":            float32(4.2),
		}

		p := params.New(orig)
		require.Equal(t, 3, p.Len())

		for key := range orig {
			_, ok := p.Get(key)
			require.True(t, ok)
		}
	})
}

func TestKeyNormalization(t *testing.T) {
	// All of the following keys should be normalized to the same value.
	keys := []string{
		"ice cream cone",             // spaces lowercase
		"ICE CREAM CONE",             // spaces uppercase
		"Ice Cream Cone",             // spaces titlecase
		"ice_cream_cone",             // underscores lowercase
		"ICE_CREAM_CONE",             // underscores uppercase
		"Ice_Cream_Cone",             // underscores titlecase
		"ice-cream-cone",             // kebab-case lowercase
		"ICE-CREAM-CONE",             // kebab-case uppercase
		"Ice-Cream-Cone",             // kebab-case titlecase
		"iceCreamCone",               // camelCase lowercase
		"IceCreamCone",               // camelCase uppercase
		"   ice    cream    cone   ", // spaces with multiple spaces
		"ICE_Cream-Cone",             // kebab-case with hyphen
	}

	p := params.New(nil)
	for _, key := range keys {
		p.Set(key, 42)

		for _, other := range keys {
			val, ok := p.Get(other)
			require.True(t, ok)
			require.Equal(t, 42, val)
		}

		p.Del(key)
		_, ok := p.Get(key)
		require.False(t, ok)
	}
}

func TestBool(t *testing.T) {
	tc := map[string]any{
		"validBoolT":        true,
		"validBoolF":        false,
		"validStringNumT":   "1",
		"validStringNumF":   "0",
		"validStringLChrT":  "t",
		"validStringLChrF":  "f",
		"validStringUChrT":  "T",
		"validStringUChrF":  "F",
		"validStringLowerT": "true",
		"validStringUpperT": "TRUE",
		"validStringTitleT": "True",
		"validStringLowerF": "false",
		"validStringUpperF": "FALSE",
		"validStringTitleF": "False",
		"invalidString":     "invalid",
		"invalidNumber":     1,
		"invalidFloat":      1,
	}

	p := params.New(tc)

	for key := range tc {
		boolVal, ok := p.Bool(key)
		if strings.HasPrefix(key, "valid") {
			require.True(t, ok)
			if strings.HasSuffix(key, "T") {
				require.True(t, boolVal)
			} else {
				require.False(t, boolVal)
			}
		} else {
			require.False(t, boolVal)
			require.False(t, ok)
		}
	}

	t.Run("Missing", func(t *testing.T) {
		p := params.New(tc)
		boolVal, ok := p.Bool("missing")
		require.False(t, ok)
		require.False(t, boolVal)
	})
}

type stringer struct{}

func (s *stringer) String() string {
	return "foo"
}

func TestString(t *testing.T) {
	tc := map[string]any{
		"valid":    "foo",
		"invalid":  42,
		"stringer": &stringer{},
	}

	p := params.New(tc)
	t.Run("Valid", func(t *testing.T) {
		stringVal, ok := p.String("valid")
		require.True(t, ok)
		require.Equal(t, "foo", stringVal)
	})
	t.Run("Stringer", func(t *testing.T) {
		stringVal, ok := p.String("stringer")
		require.True(t, ok)
		require.Equal(t, "foo", stringVal)
	})
	t.Run("Invalid", func(t *testing.T) {
		stringVal, ok := p.String("invalid")
		require.False(t, ok)
		require.Empty(t, stringVal)
	})
	t.Run("Missing", func(t *testing.T) {
		p := params.New(tc)
		stringVal, ok := p.String("missing")
		require.False(t, ok)
		require.Empty(t, stringVal)
	})
}

func TestInt(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		orig := map[string]any{
			"validInt":     int(42),
			"validInt64":   int64(42),
			"validInt32":   int32(42),
			"validInt16":   int16(42),
			"validInt8":    int8(42),
			"validUint":    uint(42),
			"validUint64":  uint64(42),
			"validUint32":  uint32(42),
			"validUint16":  uint16(42),
			"validUint8":   uint8(42),
			"validFloat64": float64(42),
			"validFloat32": float32(42),
			"validNumber":  json.Number("42"),
			"validString":  "42",
		}

		p := params.New(orig)
		for key := range orig {
			intVal, ok := p.Int(key)
			require.True(t, ok)
			require.Equal(t, int64(42), intVal)
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		orig := map[string]any{
			"invalidString": "invalid",
			"invalidNumber": json.Number("invalid"),
			"invalidType":   true,
		}

		p := params.New(orig)
		for key := range orig {
			intVal, ok := p.Int(key)
			require.False(t, ok)
			require.Equal(t, int64(0), intVal)
		}
	})

	t.Run("Missing", func(t *testing.T) {
		orig := map[string]any{"foo": 42}
		p := params.New(orig)
		intVal, ok := p.Int("missing")
		require.False(t, ok)
		require.Equal(t, int64(0), intVal)
	})
}

func TestFloat(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		orig := map[string]any{
			"validFloat64": float64(3.14),
			"validFloat32": float32(3.14),
			"validNumber":  json.Number("3.14"),
			"validString":  "3.14",
		}

		p := params.New(orig)
		for key := range orig {
			floatVal, ok := p.Float(key)
			require.True(t, ok)
			require.InEpsilon(t, float64(3.14), floatVal, 0.001)
		}
	})

	t.Run("TruncateInt", func(t *testing.T) {
		orig := map[string]any{
			"validInt":    int(42),
			"validInt64":  int64(42),
			"validInt32":  int32(42),
			"validInt16":  int16(42),
			"validInt8":   int8(42),
			"validUint":   uint(42),
			"validUint64": uint64(42),
			"validUint32": uint32(42),
			"validUint16": uint16(42),
			"validUint8":  uint8(42),
		}

		p := params.New(orig)
		for key := range orig {
			floatVal, ok := p.Float(key)
			require.True(t, ok)
			require.Equal(t, float64(42), floatVal)
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		orig := map[string]any{
			"invalidString": "invalid",
			"invalidNumber": json.Number("invalid"),
			"invalidType":   true,
		}

		p := params.New(orig)
		for key := range orig {
			floatVal, ok := p.Float(key)
			require.False(t, ok)
			require.Equal(t, float64(0), floatVal)
		}
	})

	t.Run("Missing", func(t *testing.T) {
		orig := map[string]any{"foo": 42}
		p := params.New(orig)
		floatVal, ok := p.Float("missing")
		require.False(t, ok)
		require.Equal(t, float64(0), floatVal)
	})
}
