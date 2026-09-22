package api

import (
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared/constant"
	"github.com/stretchr/testify/require"
)

// TestMeta verifies that provider response metadata accepts string-like values
// while ignoring empty, nil, and unrelated values.
func TestMeta(t *testing.T) {

	t.Run("PutString", func(t *testing.T) {
		s := "rouge"
		testCases := []struct {
			value    any
			ok       bool
			expected string
		}{
			{value: "foo", ok: true, expected: "foo"},
			{value: &s, ok: true, expected: "rouge"},
			{value: constant.ChatCompletion("")},
			{value: constant.ChatCompletion("").Default(), ok: true, expected: "chat.completion"},
			{value: ""},
			{value: 42},
			{value: nil},
			{value: (*string)(nil)},
			{value: openai.ChatCompletionServiceTier("")},
			{value: openai.ChatCompletionServiceTierAuto, ok: true, expected: "auto"},
			{value: openai.ChatCompletionServiceTierDefault, ok: true, expected: "default"},
			{value: openai.ChatCompletionServiceTierFlex, ok: true, expected: "flex"},
			{value: openai.ChatCompletionServiceTierScale, ok: true, expected: "scale"},
			{value: openai.ChatCompletionServiceTierPriority, ok: true, expected: "priority"},
		}

		for _, tc := range testCases {
			m := make(Meta)
			m.PutString("key", tc.value)

			val, ok := m["key"]
			require.Equal(t, tc.ok, ok)
			if ok {
				require.Equal(t, tc.expected, val)
			} else {
				require.Nil(t, val)
			}
		}
	})
}
