package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// GetMapKeys returns the keys of a map as a slice of strings
func GetMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// NormalizeJSONForComparison normalizes JSON by marshaling and unmarshaling to ensure consistent field ordering
func NormalizeJSONForComparison(jsonStr string) (string, error) {
	if jsonStr == "" {
		return "", nil
	}

	var data interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return "", err
	}

	normalized, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	return string(normalized), nil
}

// JSONEqual compares two JSON strings for semantic equality, ignoring field ordering
func JSONEqual(json1, json2 string) (bool, error) {
	if json1 == "" && json2 == "" {
		return true, nil
	}
	if json1 == "" || json2 == "" {
		return false, nil
	}

	normalized1, err := NormalizeJSONForComparison(json1)
	if err != nil {
		return false, err
	}

	normalized2, err := NormalizeJSONForComparison(json2)
	if err != nil {
		return false, err
	}

	return normalized1 == normalized2, nil
}

// MapToJSONString converts a types.Map to a JSON string
func MapToJSONString(ctx context.Context, mapValue types.Map) (string, diag.Diagnostics) {
	if mapValue.IsNull() || mapValue.IsUnknown() {
		return "", nil
	}

	// Convert the map elements to a regular map
	mapData := make(map[string]interface{})
	elements := mapValue.Elements()

	for key, value := range elements {
		// Convert the attr.Value to interface{}
		switch v := value.(type) {
		case types.String:
			mapData[key] = v.ValueString()
		case types.Number:
			mapData[key] = v.ValueBigFloat()
		case types.Bool:
			mapData[key] = v.ValueBool()
		default:
			// For other types, try to convert using DynamicAttrValueToGo
			converted, diags := DynamicAttrValueToGo(ctx, value)
			if diags.HasError() {
				return "", diags
			}
			mapData[key] = converted
		}
	}

	jsonBytes, err := json.Marshal(mapData)
	if err != nil {
		diags := diag.Diagnostics{}
		diags.AddError("JSON Marshal Error", fmt.Sprintf("Failed to marshal map to JSON: %s", err))
		return "", diags
	}

	return string(jsonBytes), nil
}

// DynamicAttrValueToGo recursively converts attr.Value to Go values for JSON serialization
func DynamicAttrValueToGo(ctx context.Context, v attr.Value) (interface{}, diag.Diagnostics) {
	if v.IsNull() || v.IsUnknown() {
		return nil, nil
	}

	switch val := v.(type) {
	case types.String:
		return val.ValueString(), nil
	case types.Number:
		return val.ValueBigFloat(), nil
	case types.Bool:
		return val.ValueBool(), nil
	case types.List:
		var result []interface{}
		listVals := val.Elements()
		for _, elem := range listVals {
			converted, diags := DynamicAttrValueToGo(ctx, elem)
			if diags != nil && diags.HasError() {
				return nil, diags
			}
			result = append(result, converted)
		}
		return result, nil
	case types.Set:
		var result []interface{}
		setVals := val.Elements()
		for _, elem := range setVals {
			converted, diags := DynamicAttrValueToGo(ctx, elem)
			if diags != nil && diags.HasError() {
				return nil, diags
			}
			result = append(result, converted)
		}
		return result, nil
	case types.Map:
		result := make(map[string]interface{})
		mapVals := val.Elements()
		for k, elem := range mapVals {
			converted, diags := DynamicAttrValueToGo(ctx, elem)
			if diags != nil && diags.HasError() {
				return nil, diags
			}
			result[k] = converted
		}
		return result, nil
	case types.Object:
		result := make(map[string]interface{})
		objVals := val.Attributes()
		for k, elem := range objVals {
			converted, diags := DynamicAttrValueToGo(ctx, elem)
			if diags != nil && diags.HasError() {
				return nil, diags
			}
			result[k] = converted
		}
		return result, nil
	case basetypes.TupleValue:
		var result []interface{}
		tupleVals := val.Elements()
		for _, elem := range tupleVals {
			converted, diags := DynamicAttrValueToGo(ctx, elem)
			if diags != nil && diags.HasError() {
				return nil, diags
			}
			result = append(result, converted)
		}
		return result, nil
	default:
		// For other types, try to marshal directly
		return val, nil
	}
}

// DynamicToJSONString converts a types.Dynamic to a JSON string
func DynamicToJSONString(ctx context.Context, dynamicValue types.Dynamic) (string, diag.Diagnostics) {
	if dynamicValue.IsNull() || dynamicValue.IsUnknown() {
		return "", nil
	}

	underlyingValue := dynamicValue.UnderlyingValue()
	if underlyingValue == nil {
		return "", nil
	}

	value, diags := DynamicAttrValueToGo(ctx, underlyingValue)
	if diags != nil && diags.HasError() {
		return "", diags
	}

	jsonBytes, err := json.Marshal(value)
	if err != nil {
		diags := diag.Diagnostics{}
		diags.AddError("JSON Marshal Error", fmt.Sprintf("Failed to marshal dynamic to JSON: %s", err))
		return "", diags
	}

	return string(jsonBytes), nil
}

// GenerateKeysFromResponse extracts keys from a GraphQL response
func GenerateKeysFromResponse(ctx context.Context, responseBytes []byte) (map[string]interface{}, error) {
	var robj map[string]interface{}
	if err := json.Unmarshal(responseBytes, &robj); err != nil {
		return nil, err
	}

	data, ok := robj["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("response JSON does not contain a 'data' object")
	}

	generatedKeys := make(map[string]interface{})
	FlattenRecursive(ctx, "", data, generatedKeys)
	return generatedKeys, nil
}

// FlattenRecursive flattens nested JSON structures into dot-notation keys
func FlattenRecursive(ctx context.Context, prefix string, data interface{}, keyMap map[string]interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, val := range v {
			newPrefix := key
			if prefix != "" {
				newPrefix = prefix + "." + key
			}
			FlattenRecursive(ctx, newPrefix, val, keyMap)
		}
	case []interface{}:
		for i, val := range v {
			newPrefix := fmt.Sprintf("%s.%d", prefix, i)
			FlattenRecursive(ctx, newPrefix, val, keyMap)
		}
	default:
		pathParts := strings.Split(prefix, ".")
		leafKey := pathParts[len(pathParts)-1]
		if _, exists := keyMap[leafKey]; !exists {
			keyMap[leafKey] = prefix
			tflog.Debug(ctx, fmt.Sprintf("Auto-generated key: '%s' -> '%s'", leafKey, prefix))
		}
	}
}

// ParseGraphQLQueryFields extracts field names from a GraphQL query
func ParseGraphQLQueryFields(query string) []string {
	var fields []string
	lines := strings.Split(query, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "query") && !strings.HasPrefix(line, "mutation") {
			// Remove common GraphQL syntax
			line = strings.Trim(line, "{}")
			line = strings.TrimSpace(line)

			if line != "" {
				fields = append(fields, line)
			}
		}
	}

	return fields
}

// DiagnosticsToString converts diagnostics to a string for logging
func DiagnosticsToString(diags diag.Diagnostics) string {
	if diags == nil {
		return ""
	}

	var messages []string
	for _, d := range diags {
		messages = append(messages, d.Detail())
	}

	return strings.Join(messages, "; ")
}
