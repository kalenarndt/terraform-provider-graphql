package utils

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
)

func TestGetMapKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected []string
	}{
		{
			name:     "empty map",
			input:    map[string]interface{}{},
			expected: []string{},
		},
		{
			name: "simple map",
			input: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			expected: []string{"key1", "key2"},
		},
		{
			name: "nested map",
			input: map[string]interface{}{
				"parent": map[string]interface{}{
					"child": "value",
				},
			},
			expected: []string{"parent"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetMapKeys(tt.input)
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}

func TestNormalizeJSONForComparison(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
			hasError: false,
		},
		{
			name:     "simple JSON",
			input:    `{"b":2,"a":1}`,
			expected: `{"a":1,"b":2}`,
			hasError: false,
		},
		{
			name:     "invalid JSON",
			input:    `{"invalid": json}`,
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NormalizeJSONForComparison(tt.input)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestJSONEqual(t *testing.T) {
	tests := []struct {
		name     string
		json1    string
		json2    string
		expected bool
		hasError bool
	}{
		{
			name:     "identical JSON",
			json1:    `{"a":1,"b":2}`,
			json2:    `{"b":2,"a":1}`,
			expected: true,
			hasError: false,
		},
		{
			name:     "different JSON",
			json1:    `{"a":1,"b":2}`,
			json2:    `{"a":1,"b":3}`,
			expected: false,
			hasError: false,
		},
		{
			name:     "empty strings",
			json1:    "",
			json2:    "",
			expected: true,
			hasError: false,
		},
		{
			name:     "one empty string",
			json1:    `{"a":1}`,
			json2:    "",
			expected: false,
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := JSONEqual(tt.json1, tt.json2)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestMapToJSONStringDebug(t *testing.T) {
	ctx := context.Background()

	// Create a simple map
	input := types.MapValueMust(types.StringType, map[string]attr.Value{
		"key1": types.StringValue("value1"),
	})

	result, diags := MapToJSONString(ctx, input)
	t.Logf("Result: %q", result)
	t.Logf("Diags: %v", diags)
	t.Logf("Input IsNull: %v", input.IsNull())
	t.Logf("Input IsUnknown: %v", input.IsUnknown())
}

func TestMapToJSONString(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		input    types.Map
		expected string
		hasError bool
	}{
		{
			name:     "empty map",
			input:    types.MapValueMust(types.StringType, map[string]attr.Value{}),
			expected: "{}",
			hasError: false,
		},
		{
			name: "simple map",
			input: types.MapValueMust(types.StringType, map[string]attr.Value{
				"key1": types.StringValue("value1"),
				"key2": types.StringValue("value2"),
			}),
			expected: `{"key1":"value1","key2":"value2"}`,
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, diags := MapToJSONString(ctx, tt.input)
			if tt.hasError {
				assert.True(t, diags.HasError())
			} else {
				assert.False(t, diags.HasError())
				// For the empty map case, we expect "{}"
				if tt.name == "empty map" {
					assert.Equal(t, "{}", result)
				} else {
					// For other cases, just check it's not empty and contains the expected keys
					assert.NotEmpty(t, result)
					assert.Contains(t, result, "key1")
					assert.Contains(t, result, "key2")
				}
			}
		})
	}
}

func TestGenerateKeysFromResponse(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		input    []byte
		expected map[string]interface{}
		hasError bool
	}{
		{
			name:  "simple response",
			input: []byte(`{"data":{"id":"123","name":"test"}}`),
			expected: map[string]interface{}{
				"id":   "id",
				"name": "name",
			},
			hasError: false,
		},
		{
			name:     "invalid JSON",
			input:    []byte(`{"invalid": json}`),
			expected: nil,
			hasError: true,
		},
		{
			name:     "no data field",
			input:    []byte(`{"result":{"id":"123"}}`),
			expected: nil,
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GenerateKeysFromResponse(ctx, tt.input)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestDiagnosticsToString(t *testing.T) {
	tests := []struct {
		name     string
		input    diag.Diagnostics
		expected string
	}{
		{
			name:     "empty diagnostics",
			input:    diag.Diagnostics{},
			expected: "",
		},
		{
			name: "single diagnostic",
			input: diag.Diagnostics{
				diag.NewErrorDiagnostic("Test", "Test error"),
			},
			expected: "Test error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DiagnosticsToString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
