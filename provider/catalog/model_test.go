package catalog_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/provider/catalog"
)

// Round-trips the model capabilities that Horizon recognizes.
func TestModelCapabilitiesRoundTrip(t *testing.T) {
	model := catalog.Model{Capabilities: catalog.Tools | catalog.Reasoning}
	encoded, err := json.Marshal(model)
	require.NoError(t, err)

	var decoded catalog.Model
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, model.Capabilities, decoded.Capabilities)
}

// Keeps the original provider object available alongside normalized fields.
func TestModelProviderRawRoundTrip(t *testing.T) {
	providerRaw := json.RawMessage(`{"id":"provider/model","future_field":{"keep":"me"}}`)
	model := catalog.Model{ProviderRaw: providerRaw}
	encoded, err := json.Marshal(model)
	require.NoError(t, err)

	var decoded catalog.Model
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.JSONEq(t, string(providerRaw), string(decoded.ProviderRaw))
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
