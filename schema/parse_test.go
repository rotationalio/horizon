package schema_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.rtnl.ai/horizon/errors"
	"go.rtnl.ai/horizon/schema"
	"go.rtnl.ai/x/mime"
)

func TestSimpleParser(t *testing.T) {
	parser := schema.NewParser(false)

	tests := []struct {
		input    string
		mimeType mime.Type
		expected any
		err      error
	}{
		{
			input:    "{ \"foo\": \"bar\" }",
			mimeType: mime.ApplicationJSON,
			expected: json.RawMessage(`{ "foo": "bar" }`),
		},
		{
			input:    "[1,2,3]",
			mimeType: mime.ApplicationJSON,
			expected: json.RawMessage(`[1,2,3]`),
		},
		{
			input:    "foo: bar",
			mimeType: mime.ApplicationYAML,
			expected: "foo: bar",
		},
		{
			input:    "```json\n{ \"foo\": \"bar\" }\n```",
			mimeType: mime.ApplicationJSON,
			err:      errors.New("could not unmarshal JSON data"),
		},
	}

	for i, tc := range tests {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			actual, err := parser.Parse(tc.input, &schema.Schema{MimeType: tc.mimeType})
			if tc.err != nil {
				require.ErrorContains(t, err, tc.err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expected, actual)
			}
		})
	}
}

func TestBestEffortParser(t *testing.T) {
	parser := schema.NewParser(true)

	tests := []struct {
		input    string
		mimeType mime.Type
		expected any
		err      error
	}{
		{
			input:    "{ \"foo\": \"bar\" }",
			mimeType: mime.ApplicationJSON,
			expected: json.RawMessage(`{ "foo": "bar" }`),
		},
		{
			input:    "foo: bar",
			mimeType: mime.ApplicationYAML,
			expected: "foo: bar",
		},
		{
			input:    "```json\n{ \"foo\": \"bar\" }\n```",
			mimeType: mime.ApplicationJSON,
			expected: json.RawMessage(`{ "foo": "bar" }`),
		},
		{
			input:    "Here is some generated JSON: { \"foo\": \"bar\" }",
			mimeType: mime.ApplicationJSON,
			expected: json.RawMessage(`{ "foo": "bar" }`),
		},
		{
			input:    "Not a JSON string",
			mimeType: mime.ApplicationJSON,
			err:      errors.New("could not unmarshal JSON data"),
		},
		{
			input:    "{ \"foo\": }",
			mimeType: mime.ApplicationJSON,
			err:      errors.New("could not unmarshal JSON data"),
		},
		{
			input:    "{ \"foo\": \"bar",
			mimeType: mime.ApplicationJSON,
			err:      errors.New("could not unmarshal JSON data"),
		},
		{
			input:    "{}",
			mimeType: mime.ApplicationJSON,
			expected: json.RawMessage(`{}`),
		},
		{
			input:    "{} {}",
			mimeType: mime.ApplicationJSON,
			err:      errors.New("could not unmarshal JSON data"),
		},
		{
			input:    "This is a JSON array: [1,2,3]",
			mimeType: mime.ApplicationJSON,
			expected: json.RawMessage(`[1,2,3]`),
		},
		{
			input:    "This is a JSON string: \"foo\"",
			mimeType: mime.ApplicationJSON,
			expected: json.RawMessage(`"foo"`),
		},
	}

	for i, tc := range tests {
		t.Run(fmt.Sprintf("test case %d", i), func(t *testing.T) {
			actual, err := parser.Parse(tc.input, &schema.Schema{MimeType: tc.mimeType})
			if tc.err != nil {
				require.ErrorContains(t, err, tc.err.Error())
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.expected, actual)
			}
		})
	}
}
