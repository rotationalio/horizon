package metadata_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/metadata"
	"go.yaml.in/yaml/v3"
)

// Verifies supported nested containers are copied while opaque values stay shared.
func TestMetadataClone(t *testing.T) {
	opaque := &struct{ Value string }{Value: "shared"}
	original := metadata.Metadata{
		"nested": metadata.Metadata{
			"values": []any{"original", map[string]any{"count": 2}},
		},
		"strings": []string{"original"},
		"bytes":   []byte("original"),
		"opaque":  opaque,
	}

	cloned := original.Clone()
	cloned["nested"].(metadata.Metadata)["values"].([]any)[0] = "changed"
	cloned["nested"].(metadata.Metadata)["values"].([]any)[1].(map[string]any)["count"] = 3
	cloned["strings"].([]string)[0] = "changed"
	cloned["bytes"].([]byte)[0] = 'c'

	require.Equal(t, "original", original["nested"].(metadata.Metadata)["values"].([]any)[0])
	require.Equal(t, 2, original["nested"].(metadata.Metadata)["values"].([]any)[1].(map[string]any)["count"])
	require.Equal(t, "original", original["strings"].([]string)[0])
	require.Equal(t, []byte("original"), original["bytes"])
	require.Same(t, opaque, cloned["opaque"])
}

// Verifies containers through depth 10 are copied and deeper ones are retained.
func TestMetadataCloneDepthLimit(t *testing.T) {
	deepest := map[string]any{"leaf": "original"}
	var nested any = deepest
	for range metadata.MaxCloneDepth {
		nested = []any{nested}
	}

	original := metadata.Metadata{"nested": nested}
	cloned := original.Clone()
	originalAtDepth10, clonedAtDepth10 := original["nested"], cloned["nested"]
	for range metadata.MaxCloneDepth - 1 {
		originalAtDepth10 = originalAtDepth10.([]any)[0]
		clonedAtDepth10 = clonedAtDepth10.([]any)[0]
	}

	originalDepth10 := originalAtDepth10.([]any)
	clonedDepth10 := clonedAtDepth10.([]any)
	clonedDepth11 := clonedDepth10[0].(map[string]any)
	clonedDepth10[0] = "replaced"
	require.IsType(t, map[string]any{}, originalDepth10[0], "the container at depth 10 is copied")

	deepest["leaf"] = "changed"
	require.Equal(t, "changed", clonedDepth11["leaf"], "the value at depth 11 is retained")

	cyclic := make(map[string]any)
	cyclic["self"] = cyclic
	require.NotPanics(t, func() { (metadata.Metadata{"cycle": cyclic}).Clone() })
}

// Verifies Clone preserves nil and returns a non-nil empty map.
func TestMetadataCloneNilAndEmpty(t *testing.T) {
	var absent metadata.Metadata
	require.Nil(t, absent.Clone())
	require.NotNil(t, metadata.Metadata{}.Clone())
}

// Verifies standard JSON encoding and decoding for Metadata values.
func TestMetadataJSONRoundTrip(t *testing.T) {
	original := metadata.Metadata{
		"string": "value",
		"int":    42,
		"float":  3.14,
		"bool":   true,
		"null":   nil,
		"nested": map[string]any{"items": []any{"value", 7}},
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded metadata.Metadata
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, metadata.Metadata{
		"string": "value",
		"int":    42.0,
		"float":  3.14,
		"bool":   true,
		"null":   nil,
		"nested": map[string]any{"items": []any{"value", 7.0}},
	}, decoded)
}

// Verifies standard YAML encoding and decoding for Metadata values.
func TestMetadataYAMLRoundTrip(t *testing.T) {
	original := metadata.Metadata{
		"string": "value",
		"int":    42,
		"float":  3.14,
		"bool":   true,
		"null":   nil,
		"nested": metadata.Metadata{"items": []any{"value", 7}},
	}

	data, err := yaml.Marshal(original)
	require.NoError(t, err)

	var decoded metadata.Metadata
	require.NoError(t, yaml.Unmarshal(data, &decoded))
	require.Equal(t, original, decoded)
}

type cloneableValue struct {
	labels []string
}

// Clone returns an independent copy of the cloneable test value.
func (v cloneableValue) Clone() any {
	return cloneableValue{labels: slices.Clone(v.labels)}
}

var _ metadata.Cloner = cloneableValue{}

// Verifies Clone calls an exact-type Clone method.
func TestMetadataCloneUsesCustomCloner(t *testing.T) {
	originalValue := cloneableValue{labels: []string{"original"}}
	originalMetadata := metadata.Metadata{"custom": originalValue}

	clonedMetadata := originalMetadata.Clone()
	clonedValue, ok := clonedMetadata["custom"].(cloneableValue)
	require.True(t, ok, "the cloned metadata value should retain its concrete type")

	clonedValue.labels[0] = "changed"
	require.Equal(t, "original", originalValue.labels[0], "mutating the clone must not change the original")
}
