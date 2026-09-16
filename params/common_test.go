package params_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/params"
)

func TestCommon(t *testing.T) {
	t.Run("Float64", func(t *testing.T) {
		p := params.New(nil)
		testCases := []struct {
			key string
			val float64
			f   func() (float64, bool)
		}{
			{key: params.FrequencyPenalty, val: 0.5, f: p.FrequencyPenalty},
			{key: params.PresencePenalty, val: 0.5, f: p.PresencePenalty},
			{key: params.Temperature, val: 0.5, f: p.Temperature},
			{key: params.TopP, val: 0.5, f: p.TopP},
		}

		// Create the params dictionary.
		for _, tc := range testCases {
			p.Set(tc.key, tc.val)
		}

		for _, tc := range testCases {
			t.Run(tc.key, func(t *testing.T) {
				val, ok := tc.f()
				require.True(t, ok)
				require.Equal(t, tc.val, val)
			})
		}
	})

	t.Run("Int64", func(t *testing.T) {
		p := params.New(nil)
		testCases := []struct {
			key string
			val int64
			f   func() (int64, bool)
		}{
			{key: params.MaxCompletionTokens, val: 100, f: p.MaxCompletionTokens},
			{key: params.MaxTokens, val: 231, f: p.MaxTokens},
			{key: params.Seed, val: 42, f: p.Seed},
			{key: params.TopLogProbs, val: 10, f: p.TopLogProbs},
			{key: params.MaxOutputTokens, val: 569, f: p.MaxOutputTokens},
			{key: params.MaxToolCalls, val: 3, f: p.MaxToolCalls},
		}

		// Create the params dictionary.
		for _, tc := range testCases {
			p.Set(tc.key, tc.val)
		}

		for _, tc := range testCases {
			t.Run(tc.key, func(t *testing.T) {
				val, ok := tc.f()
				require.True(t, ok)
				require.Equal(t, tc.val, val)
			})
		}
	})

	t.Run("String", func(t *testing.T) {
		p := params.New(nil)
		testCases := []struct {
			key string
			val string
			f   func() (string, bool)
		}{
			{key: params.PromptCacheKey, val: "foo", f: p.PromptCacheKey},
			{key: params.SafetyIdentifier, val: "bar", f: p.SafetyIdentifier},
			{key: params.PromptCacheRetention, val: "24h", f: p.PromptCacheRetention},
			{key: params.ReasoningEffort, val: "medium", f: p.ReasoningEffort},
			{key: params.ServiceTier, val: "flex", f: p.ServiceTier},
			{key: params.Verbosity, val: "medium", f: p.Verbosity},
			{key: params.ToolChoice, val: "auto", f: p.ToolChoice},
			{key: params.Truncation, val: "disabled", f: p.Truncation},
			{key: params.ReasoningSummary, val: "concise", f: p.ReasoningSummary},
		}

		// Create the params dictionary.
		for _, tc := range testCases {
			p.Set(tc.key, tc.val)
		}

		for _, tc := range testCases {
			t.Run(tc.key, func(t *testing.T) {
				val, ok := tc.f()
				require.True(t, ok)
				require.Equal(t, tc.val, val)
			})
		}
	})

	t.Run("Bool", func(t *testing.T) {
		p := params.New(nil)
		testCases := []struct {
			key string
			val bool
			f   func() (bool, bool)
		}{
			{key: params.LogProbs, val: true, f: p.LogProbs},
		}

		// Create the params dictionary.
		for _, tc := range testCases {
			p.Set(tc.key, tc.val)
		}

		for _, tc := range testCases {
			t.Run(tc.key, func(t *testing.T) {
				val, ok := tc.f()
				require.True(t, ok)
				require.Equal(t, tc.val, val)
			})
		}
	})

}

func TestMaxTokens(t *testing.T) {
	t.Run("All", func(t *testing.T) {
		p := params.New(map[string]any{
			params.MaxCompletionTokens: 100,
			params.MaxTokens:           200,
			params.MaxOutputTokens:     300,
		})

		maxCompletionTokens, _ := p.MaxCompletionTokens()
		require.Equal(t, int64(100), maxCompletionTokens)

		maxTokens, _ := p.MaxTokens()
		require.Equal(t, int64(200), maxTokens)

		maxOutputTokens, _ := p.MaxOutputTokens()
		require.Equal(t, int64(300), maxOutputTokens)
	})

	t.Run("MaxTokens", func(t *testing.T) {
		p := params.New(map[string]any{
			params.MaxTokens: 200,
		})

		maxCompletionTokens, _ := p.MaxCompletionTokens()
		require.Equal(t, int64(200), maxCompletionTokens)

		maxTokens, _ := p.MaxTokens()
		require.Equal(t, int64(200), maxTokens)

		maxOutputTokens, _ := p.MaxOutputTokens()
		require.Equal(t, int64(200), maxOutputTokens)
	})

	t.Run("NoMaxTokens", func(t *testing.T) {
		p := params.New(map[string]any{
			params.MaxCompletionTokens: 100,
			params.MaxOutputTokens:     300,
		})

		maxCompletionTokens, _ := p.MaxCompletionTokens()
		require.Equal(t, int64(100), maxCompletionTokens)

		_, ok := p.MaxTokens()
		require.False(t, ok)

		maxTokens, _ := p.MaxOutputTokens()
		require.Equal(t, int64(300), maxTokens)
	})

	t.Run("None", func(t *testing.T) {
		p := params.New(nil)

		_, ok := p.MaxCompletionTokens()
		require.False(t, ok)

		_, ok = p.MaxTokens()
		require.False(t, ok)

		_, ok = p.MaxOutputTokens()
		require.False(t, ok)
	})
}

func TestLogitBias(t *testing.T) {
	t.Run("Normal", func(t *testing.T) {
		p := params.New(map[string]any{
			params.LogitBias: map[string]int64{
				"foo": 100,
				"bar": -50,
			},
		})

		logitBias, ok := p.LogitBias()
		require.True(t, ok)
		require.Equal(t, map[string]int64{
			"foo": 100,
			"bar": -50,
		}, logitBias)
	})

	t.Run("Untyped", func(t *testing.T) {
		logits := map[string]any{
			"validInt":    42,
			"validInt64":  int64(42),
			"validInt32":  int32(42),
			"validInt16":  int16(42),
			"validInt8":   int8(42),
			"validUint":   uint(42),
			"validUint64": uint64(42),
			"validUint32": uint32(42),
			"validUint16": uint16(42),
			"validUint8":  uint8(42),
			"validNumber": json.Number("42"),
			"validString": "42",
		}

		p := params.New(map[string]any{
			params.LogitBias: logits,
		})

		logitBias, ok := p.LogitBias()
		require.True(t, ok)

		for k, v := range logitBias {
			require.Contains(t, logits, k)
			require.Equal(t, int64(42), v)
		}
	})

	t.Run("Invalid", func(t *testing.T) {
		p := params.New(map[string]any{
			params.LogitBias: "invalid",
		})

		_, ok := p.LogitBias()
		require.False(t, ok)
	})

	t.Run("CannotConvert", func(t *testing.T) {
		testCases := []map[string]any{
			{
				"foo": make(chan int),
			},
			{
				"foo": json.Number("invalid"),
			},
			{
				"foo": "invalid",
			},
		}

		for _, tc := range testCases {
			p := params.New(map[string]any{
				params.LogitBias: tc,
			})

			_, ok := p.LogitBias()
			require.False(t, ok)
		}
	})

	t.Run("None", func(t *testing.T) {
		p := params.New(nil)

		_, ok := p.LogitBias()
		require.False(t, ok)
	})
}
