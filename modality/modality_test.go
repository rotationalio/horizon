package modality_test

import (
	"encoding/json"
	"iter"
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	. "go.rtnl.ai/horizon/modality"
)

func TestModality_Parse(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		testCases := []struct {
			input    any
			expected Modality
		}{
			{input: "text", expected: Text},
			{input: "Image", expected: Image},
			{input: "AUDIO", expected: Audio},
			{input: " video ", expected: Video},
			{input: "doCumEnt", expected: Document},
			{input: "spatial  ", expected: Spatial},
			{input: "\ttimeseries", expected: TimeSeries},
			{input: "\tRobotic", expected: Robotic},
			{input: "Genomic", expected: Genomic},
			{input: []string{"Text", "image"}, expected: Text | Image},
			{input: []string{"text", "Image", "audio"}, expected: Text | Image | Audio},
			{input: []string{"text", "IMAGE", " audio", "video"}, expected: Text | Image | Audio | Video},
			{input: []string{"Text", "Image", "Audio", "Video", "Document"}, expected: Text | Image | Audio | Video | Document},
			{input: "\ttext | Image | AUDIO | vIDeo | Document    \t", expected: Text | Image | Audio | Video | Document},
			{input: uint8(16), expected: Document},
			{input: uint16(16), expected: Document},
			{input: uint32(16), expected: Document},
			{input: uint64(16), expected: Document},
			{input: int8(16), expected: Document},
			{input: int16(16), expected: Document},
			{input: int32(16), expected: Document},
			{input: int64(16), expected: Document},
			{input: int(16), expected: Document},
			{input: uint(16), expected: Document},
		}

		for _, tc := range testCases {
			m, err := Parse(tc.input)
			require.NoError(t, err, "expected no error for input %T", tc.input)
			require.Equal(t, tc.expected, m, "expected %q but got %q", tc.expected.String(), m.String())
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		testCases := []struct {
			input    any
			expected string
		}{
			{
				input:    true,
				expected: "cannot parse bool into Modality",
			},
			{
				input:    "invalid",
				expected: "invalid modality \"invalid\"",
			},
			{
				input:    []string{"text", "invalid"},
				expected: "invalid modality \"invalid\"",
			},
		}

		for _, tc := range testCases {
			m, err := Parse(tc.input)
			require.Error(t, err, "expected error but got none for input %T", tc.input)
			require.EqualError(t, err, tc.expected, "unexpected error for input %T", tc.input)
			require.Equal(t, Modality(0), m, "expected 0 but got %T", m)
		}
	})

	t.Run("Serializers", func(t *testing.T) {
		t.Run("String", func(t *testing.T) {
			for mode := range Combinations() {
				s := mode.String()
				require.NotEmpty(t, s, "expected non-empty string for modality %q", mode.String())

				cmp, err := Parse(s)
				require.NoError(t, err, "could not parse modality %q", s)
				require.Equal(t, mode, cmp, "expected %q but got %q", mode.String(), cmp.String())
			}
		})

		t.Run("Binary", func(t *testing.T) {
			for mode := range Combinations() {
				buf, err := mode.MarshalBinary()
				require.NoError(t, err, "could not marshal modality %q", mode.String())
				require.NotNil(t, buf, "expected non-nil buffer for modality %q", mode.String())

				cmp, err := Parse(buf)
				require.NoError(t, err, "could not parse modality %q", buf)
				require.Equal(t, mode, cmp, "expected %q but got %q", mode.String(), cmp.String())
			}
		})

		t.Run("Text", func(t *testing.T) {
			for mode := range Combinations() {
				buf, err := mode.MarshalText()
				require.NoError(t, err, "could not marshal modality %q", mode.String())
				require.NotNil(t, buf, "expected non-nil buffer for modality %q", mode.String())

				cmp, err := Parse(string(buf))
				require.NoError(t, err, "could not parse modality %q", string(buf))
				require.Equal(t, mode, cmp, "expected %q but got %q", mode.String(), cmp.String())
			}
		})

		t.Run("JSON", func(t *testing.T) {
			for mode := range Combinations() {
				buf, err := mode.MarshalJSON()
				require.NoError(t, err, "could not marshal modality %q", mode.String())
				require.NotNil(t, buf, "expected non-nil buffer for modality %q", mode.String())

				var v any
				require.NoError(t, json.Unmarshal(buf, &v), "could not unmarshal modality %q", buf)

				cmp, err := Parse(v)
				require.NoError(t, err, "could not parse modality %q", v)
				require.Equal(t, mode, cmp, "expected %q but got %q", mode.String(), cmp.String())
			}
		})
	})
}

func TestIntConversion(t *testing.T) {
	// Modality depends on the standard library int types converting signed and unsigned
	// values correctly. I can't imagine this will ever change, so this test is mostly
	// just proof to future me that this code works.
	us := uint16(43271)
	si := int16(us)

	require.Equal(t, si, int16(us), "converting unsigned to signed should be lossless")
	require.Equal(t, us, uint16(si), "converting signed to unsigned should be lossless")
}

// No matter the ordering of
func TestModality_String_ordering(t *testing.T) {
}

func TestModality_String(t *testing.T) {

	t.Run("SpotChecks", func(t *testing.T) {
		var testCases = []struct {
			name string
			mode Modality
		}{
			{name: "None", mode: 0},
			{name: "Text", mode: Text},
			{name: "Image", mode: Image},
			{name: "Text | Image", mode: Text | Image},
			{name: "Text | Image | Audio", mode: Text | Image | Audio},
			{name: "Audio", mode: Audio},
			{name: "Text | Audio", mode: Audio | Text},
			{name: "Text | Audio | Video", mode: Audio | Text | Video},
			{name: "Video", mode: Video},
			{name: "Document", mode: Document},
			{name: "Spatial", mode: Spatial},
			{name: "Spatial | TimeSeries", mode: TimeSeries | Spatial},
			{name: "TimeSeries", mode: TimeSeries},
			{name: "Robotic", mode: Robotic},
			{name: "Genomic", mode: Genomic},
			{
				name: "Text | Image | Audio | Video | Document | Spatial | TimeSeries | Robotic | Genomic",
				mode: Text | Image | Audio | Video | Document | Spatial | TimeSeries | Robotic | Genomic,
			},
		}

		for i, tc := range testCases {
			s := tc.mode.String()
			require.Equal(t, tc.name, s, "expected %q but got %q for %016b at index %d", tc.name, s, tc.mode, i)
		}
	})

	t.Run("Known", func(t *testing.T) {
		for mode := range Combinations() {
			require.NotEmpty(t, mode.String(), "expected non-empty string for %016b", mode)
		}
	})

	t.Run("Unknown", func(t *testing.T) {
		// The current biggest non-unknown modality is Genomic, the 9th modality, so
		// the greatest value which contains all modalities is 2^9 - 1 = 511. which has
		// the bitmask 0000000111111111. Any values larger than this that have their
		// last 9 bits cleared and are not zero should be unknown.
		for i := uint16(512); i < math.MaxUint16; i++ {
			n := i &^ 511
			if n == 0 {
				continue
			}

			require.Equal(t, "Unknown", Modality(n).String(), "expected empty string but got %q", Modality(n).String())
		}
	})
}

func TestModality_Is(t *testing.T) {
	t.Run("Text", func(t *testing.T) {
		require.True(t, Text.IsText())
		require.False(t, Text.IsImage())
		require.False(t, Text.IsAudio())
		require.False(t, Text.IsVideo())
		require.False(t, Text.IsDocument())
		require.False(t, Text.IsSpatial())
		require.False(t, Text.IsTimeSeries())
		require.False(t, Text.IsRobotic())
		require.False(t, Text.IsGenomic())
	})

	t.Run("Image", func(t *testing.T) {
		require.False(t, Image.IsText())
		require.True(t, Image.IsImage())
		require.False(t, Image.IsAudio())
		require.False(t, Image.IsVideo())
		require.False(t, Image.IsDocument())
		require.False(t, Image.IsSpatial())
		require.False(t, Image.IsTimeSeries())
		require.False(t, Image.IsRobotic())
		require.False(t, Image.IsGenomic())
	})

	t.Run("Audio", func(t *testing.T) {
		require.False(t, Audio.IsText())
		require.False(t, Audio.IsImage())
		require.True(t, Audio.IsAudio())
		require.False(t, Audio.IsVideo())
		require.False(t, Audio.IsDocument())
		require.False(t, Audio.IsSpatial())
		require.False(t, Audio.IsTimeSeries())
		require.False(t, Audio.IsRobotic())
		require.False(t, Audio.IsGenomic())
	})

	t.Run("Video", func(t *testing.T) {
		require.False(t, Video.IsText())
		require.False(t, Video.IsImage())
		require.False(t, Video.IsAudio())
		require.True(t, Video.IsVideo())
		require.False(t, Video.IsDocument())
		require.False(t, Video.IsSpatial())
		require.False(t, Video.IsTimeSeries())
		require.False(t, Video.IsRobotic())
		require.False(t, Video.IsGenomic())
	})

	t.Run("Document", func(t *testing.T) {
		require.False(t, Document.IsText())
		require.False(t, Document.IsImage())
		require.False(t, Document.IsAudio())
		require.False(t, Document.IsVideo())
		require.True(t, Document.IsDocument())
		require.False(t, Document.IsSpatial())
		require.False(t, Document.IsTimeSeries())
		require.False(t, Document.IsRobotic())
		require.False(t, Document.IsGenomic())
	})

	t.Run("Spatial", func(t *testing.T) {
		require.False(t, Spatial.IsText())
		require.False(t, Spatial.IsImage())
		require.False(t, Spatial.IsAudio())
		require.False(t, Spatial.IsVideo())
		require.False(t, Spatial.IsDocument())
		require.True(t, Spatial.IsSpatial())
		require.False(t, Spatial.IsTimeSeries())
		require.False(t, Spatial.IsRobotic())
		require.False(t, Spatial.IsGenomic())
	})

	t.Run("TimeSeries", func(t *testing.T) {
		require.False(t, TimeSeries.IsText())
		require.False(t, TimeSeries.IsImage())
		require.False(t, TimeSeries.IsAudio())
		require.False(t, TimeSeries.IsVideo())
		require.False(t, TimeSeries.IsDocument())
		require.False(t, TimeSeries.IsSpatial())
		require.True(t, TimeSeries.IsTimeSeries())
		require.False(t, TimeSeries.IsRobotic())
		require.False(t, TimeSeries.IsGenomic())
	})

	t.Run("Robotic", func(t *testing.T) {
		require.False(t, Robotic.IsText())
		require.False(t, Robotic.IsImage())
		require.False(t, Robotic.IsAudio())
		require.False(t, Robotic.IsVideo())
		require.False(t, Robotic.IsDocument())
		require.False(t, Robotic.IsSpatial())
		require.False(t, Robotic.IsTimeSeries())
		require.True(t, Robotic.IsRobotic())
		require.False(t, Robotic.IsGenomic())
	})

	t.Run("Genomic", func(t *testing.T) {
		require.False(t, Genomic.IsText())
		require.False(t, Genomic.IsImage())
		require.False(t, Genomic.IsAudio())
		require.False(t, Genomic.IsVideo())
		require.False(t, Genomic.IsDocument())
		require.False(t, Genomic.IsSpatial())
		require.False(t, Genomic.IsTimeSeries())
		require.False(t, Genomic.IsRobotic())
		require.True(t, Genomic.IsGenomic())
	})

	t.Run("All", func(t *testing.T) {
		All := Modality(511)
		require.True(t, All.IsText())
		require.True(t, All.IsImage())
		require.True(t, All.IsAudio())
		require.True(t, All.IsVideo())
		require.True(t, All.IsDocument())
		require.True(t, All.IsSpatial())
		require.True(t, All.IsTimeSeries())
		require.True(t, All.IsRobotic())
		require.True(t, All.IsGenomic())
	})

	t.Run("None", func(t *testing.T) {
		None := Modality(0)
		require.False(t, None.IsText())
		require.False(t, None.IsImage())
		require.False(t, None.IsAudio())
		require.False(t, None.IsVideo())
		require.False(t, None.IsDocument())
		require.False(t, None.IsSpatial())
		require.False(t, None.IsTimeSeries())
		require.False(t, None.IsRobotic())
		require.False(t, None.IsGenomic())
	})

	t.Run("Combinations", func(t *testing.T) {
		for mode := range Combinations() {
			require.Equal(t, mode&Text != 0, mode.IsText())
			require.Equal(t, mode&Image != 0, mode.IsImage())
			require.Equal(t, mode&Audio != 0, mode.IsAudio())
			require.Equal(t, mode&Video != 0, mode.IsVideo())
			require.Equal(t, mode&Document != 0, mode.IsDocument())
			require.Equal(t, mode&Spatial != 0, mode.IsSpatial())
			require.Equal(t, mode&TimeSeries != 0, mode.IsTimeSeries())
			require.Equal(t, mode&Robotic != 0, mode.IsRobotic())
			require.Equal(t, mode&Genomic != 0, mode.IsGenomic())
		}
	})

	t.Run("Unknown", func(t *testing.T) {
		// The current biggest non-unknown modality is Genomic, the 9th modality, so
		// the greatest value which contains all modalities is 2^9 - 1 = 511. which has
		// the bitmask 0000000111111111. Any values larger than this that have their
		// last 9 bits cleared and are not zero should be unknown.
		for i := uint16(512); i < math.MaxUint16; i++ {
			n := i &^ 511
			if n == 0 {
				continue
			}

			mode := Modality(n)
			require.False(t, mode.IsText())
			require.False(t, mode.IsImage())
			require.False(t, mode.IsAudio())
			require.False(t, mode.IsVideo())
			require.False(t, mode.IsDocument())
			require.False(t, mode.IsSpatial())
			require.False(t, mode.IsTimeSeries())
			require.False(t, mode.IsRobotic())
			require.False(t, mode.IsGenomic())
		}
	})
}

func TestModality_Scan(t *testing.T) {
	t.Run("Nil", func(t *testing.T) {
		var m Modality
		require.NoError(t, m.Scan(nil), "could not scan nil into modality")
		require.Equal(t, Modality(0), m, "expected 0 but got %T", m)
	})

	t.Run("Byte", func(t *testing.T) {
		for mode := range Combinations() {
			var mb Modality
			val := strconv.Itoa(int(mode))
			require.NoError(t, mb.Scan([]byte(val)), "could not scan []byte(%q) into modality", val)
			require.Equal(t, mode, mb, "expected %q but got %q", mode.String(), mb.String())
		}
	})

	t.Run("Int", func(t *testing.T) {
		for mode := range Combinations() {
			var mi Modality
			require.NoError(t, mi.Scan(int(mode)), "could not scan int(%d) into modality", mode)
			require.Equal(t, mode, mi, "expected %q but got %q", mode.String(), mi.String())
		}
	})

	t.Run("Uint", func(t *testing.T) {
		for mode := range Combinations() {
			var mu Modality
			require.NoError(t, mu.Scan(uint(mode)), "could not scan uint(%d) into modality", mode)
			require.Equal(t, mode, mu, "expected %q but got %q", mode.String(), mu.String())
		}
	})

	t.Run("Int16", func(t *testing.T) {
		for mode := range Combinations() {
			var mi16 Modality
			require.NoError(t, mi16.Scan(int16(mode)), "could not scan int16(%d) into modality", mode)
			require.Equal(t, mode, mi16, "expected %q but got %q", mode.String(), mi16.String())
		}
	})

	t.Run("Int32", func(t *testing.T) {
		for mode := range Combinations() {
			var mi32 Modality
			require.NoError(t, mi32.Scan(int32(mode)), "could not scan int32(%d) into modality", mode)
			require.Equal(t, mode, mi32, "expected %q but got %q", mode.String(), mi32.String())
		}
	})

	t.Run("Int64", func(t *testing.T) {
		for mode := range Combinations() {
			var mi64 Modality
			require.NoError(t, mi64.Scan(int64(mode)), "could not scan int64(%d) into modality", mode)
			require.Equal(t, mode, mi64, "expected %q but got %q", mode.String(), mi64.String())
		}
	})

	t.Run("Uint16", func(t *testing.T) {
		for mode := range Combinations() {
			var mu16 Modality
			require.NoError(t, mu16.Scan(uint16(mode)), "could not scan uint16(%d) into modality", mode)
			require.Equal(t, mode, mu16, "expected %q but got %q", mode.String(), mu16.String())
		}
	})

	t.Run("Uint32", func(t *testing.T) {
		for mode := range Combinations() {
			var mu32 Modality
			require.NoError(t, mu32.Scan(uint32(mode)), "could not scan uint32(%d) into modality", mode)
			require.Equal(t, mode, mu32, "expected %q but got %q", mode.String(), mu32.String())
		}
	})

	t.Run("Uint64", func(t *testing.T) {
		for mode := range Combinations() {
			var mu64 Modality
			require.NoError(t, mu64.Scan(uint64(mode)), "could not scan uint64(%d) into modality", mode)
			require.Equal(t, mode, mu64, "expected %q but got %q", mode.String(), mu64.String())
		}
	})

	t.Run("BadType", func(t *testing.T) {
		var m Modality
		require.Error(t, m.Scan(true), "expected error but got none for bool(true)")
	})

	t.Run("BadParse", func(t *testing.T) {
		var m Modality
		require.Error(t, m.Scan([]byte("invalid")), "expected error but got none for string(invalid)")
	})

}

func TestModality_Value(t *testing.T) {
	for mode := range Combinations() {
		val, err := mode.Value()
		require.NoError(t, err, "could not get value from modality %q", mode.String())
		require.Equal(t, int64(mode), val, "expected int64(%d) but got %T", int64(mode), val)
	}
}

func TestModality_NullModality(t *testing.T) {
	t.Run("Zero", func(t *testing.T) {
		n := Modality(0).NullModality()
		require.False(t, n.Valid)
		require.Equal(t, Modality(0), n.Modality)
	})

	t.Run("NonZero", func(t *testing.T) {
		for mode := range Combinations() {
			n := mode.NullModality()
			require.True(t, n.Valid)
			require.Equal(t, mode, n.Modality)
		}
	})
}

func TestNullModality_ToModality(t *testing.T) {
	t.Run("Invalid", func(t *testing.T) {
		m, ok := NullModality{}.ToModality()
		require.False(t, ok)
		require.Equal(t, Modality(0), m)
	})

	t.Run("Valid", func(t *testing.T) {
		for mode := range Combinations() {
			m, ok := mode.NullModality().ToModality()
			require.True(t, ok)
			require.Equal(t, mode, m)
		}
	})
}

func TestNullModality_Scan(t *testing.T) {
	t.Run("Nil", func(t *testing.T) {
		var m NullModality
		require.NoError(t, m.Scan(nil), "could not scan nil into null modality")
		require.Equal(t, Modality(0), m.Modality, "expected 0 but got %T", m.Modality)
	})

	t.Run("Valid", func(t *testing.T) {
		for mode := range Combinations() {
			var m NullModality
			require.NoError(t, m.Scan(int64(mode)), "could not scan %T into null modality", mode)
			require.Equal(t, mode, m.Modality, "expected %q but got %q", mode.String(), m.Modality.String())
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		var m NullModality
		require.Error(t, m.Scan(true), "expected error but got none for bool(true)")
	})

	t.Run("BadParse", func(t *testing.T) {
		var m NullModality
		require.Error(t, m.Scan([]byte("invalid")), "expected error but got none for string(invalid)")
	})

	t.Run("Empty", func(t *testing.T) {
		var m NullModality
		require.NoError(t, m.Scan(0), "expected error but got none for int(0)")
		require.Equal(t, Modality(0), m.Modality, "expected 0 but got %T", m.Modality)
		require.False(t, m.Valid, "expected valid to be false")
	})
}

func TestNullModality_Value(t *testing.T) {
	t.Run("Valid", func(t *testing.T) {
		for mode := range Combinations() {
			m := mode.NullModality()
			val, err := m.Value()
			require.NoError(t, err, "could not get value from null modality %q", mode.String())
			require.Equal(t, int64(mode), val, "expected int64(%d) but got %T", int64(mode), val)
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		m := NullModality{Valid: false, Modality: Text | Audio | Video}
		val, err := m.Value()
		require.NoError(t, err, "expected error but got none for null modality")
		require.Nil(t, val, "expected nil but got %T", val)
	})

}

func TestModality_SerializeJSON(t *testing.T) {
	for mode := range Combinations() {
		buf, err := mode.MarshalJSON()
		require.NoError(t, err, "could not marshal modality %q", mode.String())
		require.NotNil(t, buf, "expected non-nil buffer for modality %q", mode.String())

		var cmp Modality
		require.NoError(t, cmp.UnmarshalJSON(buf), "could not unmarshal modality %q", mode.String())
		require.Equal(t, mode, cmp, "expected %q but got %q", mode.String(), cmp.String())
	}
}

func TestModality_MarshalJSON(t *testing.T) {
	testCases := []struct {
		mode     Modality
		expected string
	}{
		{mode: 0, expected: "[]"},
		{mode: Text, expected: `["Text"]`},
		{mode: Image | Audio, expected: `["Image","Audio"]`},
		{mode: Document | Text | TimeSeries, expected: `["Text","Document","TimeSeries"]`},
	}

	for _, tc := range testCases {
		data, err := json.Marshal(tc.mode)
		require.NoError(t, err, "could not marshal modality %q", tc.mode.String())
		require.Equal(t, []byte(tc.expected), data, "expected data serialization")
	}
}

func TestModality_UnmarshalJSON(t *testing.T) {
	testCases := []struct {
		data     []byte
		expected Modality
	}{
		{data: []byte("[]"), expected: Modality(0)},
		{data: []byte(`""`), expected: Modality(0)},
		{data: []byte(`["text"]`), expected: Text},
		{data: []byte(`["image", "audio"]`), expected: Image | Audio},
		{data: []byte(`"Image | Audio | Document"`), expected: Image | Audio | Document},
	}

	for _, tc := range testCases {
		var m Modality
		require.NoError(t, json.Unmarshal(tc.data, &m), "could not unmarshal modality %q", tc.data)
		require.Equal(t, tc.expected, m, "expected %q but got %q", tc.expected.String(), m.String())
	}
}

func TestModality_SerializeBinary(t *testing.T) {
	for mode := range Combinations() {
		buf, err := mode.MarshalBinary()
		require.NoError(t, err, "could not marshal modality %q", mode.String())
		require.NotNil(t, buf, "expected non-nil buffer for modality %q", mode.String())

		var cmp Modality
		require.NoError(t, cmp.UnmarshalBinary(buf), "could not unmarshal modality %q", mode.String())
		require.Equal(t, mode, cmp, "expected %q but got %q", mode.String(), cmp.String())
	}
}

func TestModality_SerializeText(t *testing.T) {
	for mode := range Combinations() {
		buf, err := mode.MarshalText()
		require.NoError(t, err, "could not marshal modality %q", mode.String())
		require.NotNil(t, buf, "expected non-nil buffer for modality %q", mode.String())

		var cmp Modality
		require.NoError(t, cmp.UnmarshalText(buf), "could not unmarshal modality %q", mode.String())
		require.Equal(t, mode, cmp, "expected %q but got %q", mode.String(), cmp.String())
	}
}

func TestModality_UnmarshalParam(t *testing.T) {
	var m Modality
	require.NoError(t, m.UnmarshalParam("text"))
	require.Equal(t, Text, m)

	require.NoError(t, m.UnmarshalParam("text|image"))
	require.Equal(t, Text|Image, m)

	require.NoError(t, m.UnmarshalParam(""))
	require.True(t, m.IsZero())

	require.Error(t, m.UnmarshalParam("not-a-modality"))
}

// Using the formula 2^n where n is the number of modalities, we can generate all
// possible combinations just by iterating over all the uint16 values up to that number.
// Right now Text -> Genomic is 9 modalities, so 2^9 = 512 combinations. As we add more
// modalities, we will need to update this number.
func Combinations() iter.Seq[Modality] {
	return func(yield func(Modality) bool) {
		for i := uint16(1); i < uint16(512); i++ {
			if !yield(Modality(i)) {
				return
			}
		}
	}
}
