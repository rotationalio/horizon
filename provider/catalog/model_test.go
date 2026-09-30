package catalog_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/provider/catalog"
)

// Round-trips recognized flags and descriptive entries, including unknown tags.
func TestModelCapabilitiesRoundTrip(t *testing.T) {
	tests := []struct {
		name         string
		capabilities catalog.Capabilities
		want         string
	}{
		{name: "empty", want: `null`},
		{
			name:         "known flags",
			capabilities: catalog.Capabilities{ModelCapability: catalog.Tools | catalog.Reasoning},
			want:         `{"known":["Tools","Reasoning"]}`,
		},
		{
			name:         "unknown entry only",
			capabilities: catalog.Capabilities{Entries: []catalog.Capability{{Tag: "future", Display: "Future Capability"}}},
			want:         `{"entries":[{"tag":"future","display":"Future Capability"}]}`,
		},
		{
			name: "flags and entries",
			capabilities: catalog.Capabilities{
				ModelCapability: catalog.Tools,
				Entries:         []catalog.Capability{{Tag: "tools", Display: "Tool Calling"}, {Tag: "future", Display: "Future Capability"}},
			},
			want: `{"known":["Tools"],"entries":[{"tag":"tools","display":"Tool Calling"},{"tag":"future","display":"Future Capability"}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := catalog.Model{Capabilities: tt.capabilities}
			encoded, err := json.Marshal(model)
			require.NoError(t, err)

			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(encoded, &fields))
			if tt.capabilities.IsZero() {
				require.NotContains(t, fields, "capabilities")
			} else {
				require.JSONEq(t, tt.want, string(fields["capabilities"]))
			}
			var decoded catalog.Model
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.Equal(t, model.Capabilities, decoded.Capabilities)
			require.Equal(t, tt.capabilities.ModelCapability.IsTools(), decoded.Capabilities.IsTools())
		})
	}
}

// Resets capabilities for JSON null and leaves existing values intact on errors.
func TestModelCapabilitiesUnmarshal(t *testing.T) {
	original := catalog.Capabilities{
		ModelCapability: catalog.Tools,
		Entries:         []catalog.Capability{{Tag: "tools", Display: "Tools"}},
	}
	for _, input := range []string{`{"known":["invalid"]}`, `{"entries":42}`, `[]`} {
		value := original
		require.Error(t, json.Unmarshal([]byte(input), &value))
		require.Equal(t, original, value)
	}
	value := original
	require.NoError(t, json.Unmarshal([]byte(`null`), &value))
	require.True(t, value.IsZero())
}

// Preserves absent, null, object, and array energy values without interpretation.
func TestModelEnergyUsageRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "absent", input: `{}`, want: `{}`},
		{name: "explicit null", input: `{"energy_usage":null}`, want: `{"energy_usage":null}`},
		{name: "object", input: `{"energy_usage":{"input":1.25,"unit":"kWh"}}`, want: `{"energy_usage":{"input":1.25,"unit":"kWh"}}`},
		{name: "array", input: `{"energy_usage":[1,"unknown"]}`, want: `{"energy_usage":[1,"unknown"]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var model catalog.Model
			require.NoError(t, json.Unmarshal([]byte(tt.input), &model))
			encoded, err := json.Marshal(model)
			require.NoError(t, err)

			var got, want map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(encoded, &got))
			require.NoError(t, json.Unmarshal([]byte(tt.want), &want))
			if _, expected := want["energy_usage"]; !expected {
				require.NotContains(t, got, "energy_usage")
				return
			}
			require.JSONEq(t, string(want["energy_usage"]), string(got["energy_usage"]))
		})
	}
}
