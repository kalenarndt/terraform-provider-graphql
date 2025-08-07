package utils

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// StateComparison provides utilities for comparing Terraform state
type StateComparison struct{}

// NewStateComparison creates a new StateComparison instance
func NewStateComparison() *StateComparison {
	return &StateComparison{}
}

// DetectFieldChanges compares the desired state with the current platform state
// to determine if there are actual changes that need to be applied
func (sc *StateComparison) DetectFieldChanges(ctx context.Context, desiredState map[string]interface{}, currentStateData interface{}) bool {
	// This is a generic interface - the actual implementation would depend on the specific data model
	// For now, we'll provide a basic implementation that can be extended

	tflog.Debug(ctx, "Detecting field changes", map[string]any{
		"desiredStateKeys": GetMapKeys(desiredState),
	})

	// Extract the fields we want to monitor from the desired state
	fieldsToMonitor := desiredState

	if fieldsToMonitor == nil {
		tflog.Debug(ctx, "No fields to monitor found in desired state")
		return true // Assume changes if we can't determine fields
	}

	// Get the current platform state (this would need to be implemented based on the specific data model)
	currentState := sc.extractCurrentState(ctx, currentStateData)
	if currentState == nil {
		tflog.Debug(ctx, "Could not extract current state, assuming changes")
		return true
	}

	// Compare only the fields we're monitoring
	hasChanges := false
	for fieldName, desiredValue := range fieldsToMonitor {
		currentValue, exists := currentState[fieldName]

		if !exists {
			tflog.Debug(ctx, "Field not found in current state", map[string]any{
				"field":        fieldName,
				"desiredValue": desiredValue,
			})
			hasChanges = true
			continue
		}

		// Compare values (handle different types)
		if !sc.ValuesEqual(desiredValue, currentValue) {
			tflog.Debug(ctx, "Field value changed", map[string]any{
				"field":        fieldName,
				"desiredValue": desiredValue,
				"currentValue": currentValue,
			})
			hasChanges = true
		}
	}

	tflog.Debug(ctx, "Field change detection result", map[string]any{
		"hasChanges":      hasChanges,
		"fieldsMonitored": len(fieldsToMonitor),
	})

	return hasChanges
}

// ValuesEqual compares two values for equality, handling different types
func (sc *StateComparison) ValuesEqual(desired, current interface{}) bool {
	// Handle nil values
	if desired == nil && current == nil {
		return true
	}
	if desired == nil || current == nil {
		// Special case: if one is nil and the other is an empty object or object with all null values, consider them equal
		if desired == nil {
			return sc.isEffectivelyNull(current)
		}
		if current == nil {
			return sc.isEffectivelyNull(desired)
		}
		return false
	}

	// Handle "__secret_content__" case for sensitive fields
	// If the current value is "__secret_content__", treat it as equivalent to any actual value
	if currentStr, ok := current.(string); ok && currentStr == "__secret_content__" {
		// For sensitive fields, treat "__secret_content__" as equivalent to any actual value
		// This prevents the provider from trying to update sensitive fields unnecessarily
		return true
	}

	// Handle different types by converting to comparable format
	switch desiredVal := desired.(type) {
	case bool:
		if currentVal, ok := current.(bool); ok {
			return desiredVal == currentVal
		}
	case string:
		if currentVal, ok := current.(string); ok {
			return desiredVal == currentVal
		}
	case float64:
		if currentVal, ok := current.(float64); ok {
			return desiredVal == currentVal
		}
	case int:
		if currentVal, ok := current.(int); ok {
			return desiredVal == currentVal
		}
	case map[string]interface{}:
		if currentVal, ok := current.(map[string]interface{}); ok {
			return sc.mapsEqual(desiredVal, currentVal)
		}
	case []interface{}:
		if currentVal, ok := current.([]interface{}); ok {
			return sc.slicesEqual(desiredVal, currentVal)
		}
	}

	// For complex types, try to normalize the values before comparison
	// This helps with cases where the same logical value might be represented differently
	desiredNormalized := sc.normalizeValue(desired)
	currentNormalized := sc.normalizeValue(current)

	// Fallback to JSON comparison for complex types
	desiredJSON, _ := json.Marshal(desiredNormalized)
	currentJSON, _ := json.Marshal(currentNormalized)
	return string(desiredJSON) == string(currentJSON)
}

// isEffectivelyNull checks if a value is effectively null (empty object or object with all null values)
func (sc *StateComparison) isEffectivelyNull(value interface{}) bool {
	if value == nil {
		return true
	}

	switch v := value.(type) {
	case map[string]interface{}:
		// Check if it's an empty map
		if len(v) == 0 {
			return true
		}
		// Check if all values in the map are null
		for _, val := range v {
			if val != nil {
				return false
			}
		}
		return true
	case []interface{}:
		// Check if it's an empty slice
		if len(v) == 0 {
			return true
		}
		// Check if all values in the slice are null
		for _, val := range v {
			if val != nil {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// normalizeValue normalizes a value for comparison by handling common edge cases
func (sc *StateComparison) normalizeValue(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case string:
		// Trim whitespace for string comparisons
		return strings.TrimSpace(v)
	case map[string]interface{}:
		// Recursively normalize map values
		normalized := make(map[string]interface{})
		for key, val := range v {
			normalized[key] = sc.normalizeValue(val)
		}
		return normalized
	case []interface{}:
		// If all elements are strings, sort them
		allStrings := true
		strSlice := make([]string, len(v))
		for i, val := range v {
			s, ok := val.(string)
			if !ok {
				allStrings = false
				break
			}
			strSlice[i] = s
		}
		if allStrings {
			sort.Strings(strSlice)
			// Convert back to []interface{}
			sorted := make([]interface{}, len(strSlice))
			for i, s := range strSlice {
				sorted[i] = s
			}
			return sorted
		}
		// Fallback: recursively normalize slice values
		normalized := make([]interface{}, len(v))
		for i, val := range v {
			normalized[i] = sc.normalizeValue(val)
		}
		return normalized
	default:
		return value
	}
}

// MapsEqual compares two maps for equality
func (sc *StateComparison) mapsEqual(desired, current map[string]interface{}) bool {
	// Only compare fields that exist in the desired configuration
	// Don't require that both maps have the same length, as the remote state
	// may have additional fields that aren't defined in the Terraform configuration
	for key, desiredValue := range desired {
		currentValue, exists := current[key]
		if !exists {
			return false
		}
		if !sc.ValuesEqual(desiredValue, currentValue) {
			return false
		}
	}
	return true
}

// SlicesEqual compares two slices for equality
func (sc *StateComparison) slicesEqual(desired, current []interface{}) bool {
	if len(desired) != len(current) {
		return false
	}

	for i, desiredValue := range desired {
		if !sc.ValuesEqual(desiredValue, current[i]) {
			return false
		}
	}
	return true
}

// FindChangedFields finds the fields that have changed between two states
func (sc *StateComparison) FindChangedFields(ctx context.Context, current, previous map[string]interface{}) map[string]interface{} {
	changedFields := make(map[string]interface{})

	// Check fields in current state
	for key, currentValue := range current {
		if previousValue, exists := previous[key]; !exists || !sc.ValuesEqual(currentValue, previousValue) {
			changedFields[key] = currentValue
		}
	}

	// Check for fields that were removed
	for key := range previous {
		if _, exists := current[key]; !exists {
			changedFields[key] = nil
		}
	}

	tflog.Debug(ctx, "Found changed fields", map[string]any{
		"changedFields": GetMapKeys(changedFields),
	})

	return changedFields
}

// ExtractCurrentStateFromQueryResponse extracts current state from a GraphQL query response
func (sc *StateComparison) ExtractCurrentStateFromQueryResponse(ctx context.Context, queryResponse map[string]interface{}) map[string]interface{} {
	if queryResponse == nil {
		return nil
	}

	// Look for data in the response
	if data, ok := queryResponse["data"].(map[string]interface{}); ok {
		return sc.genericExtractCurrentState(data)
	}

	// If no data wrapper, use the response directly
	return sc.genericExtractCurrentState(queryResponse)
}

// GenericExtractCurrentState extracts current state from any JSON structure
func (sc *StateComparison) genericExtractCurrentState(data map[string]interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}

	// This is a simplified extraction - in practice, you might need more sophisticated logic
	// based on the specific GraphQL schema and response structure
	return data
}

// extractCurrentState is a placeholder for extracting current state from the data model
// This would need to be implemented based on the specific data model being used
func (sc *StateComparison) extractCurrentState(ctx context.Context, data interface{}) map[string]interface{} {
	// This is a placeholder - the actual implementation would depend on the specific data model
	// For example, if using GraphqlMutationResourceModel, you would extract the relevant fields

	// For now, return nil to indicate no current state
	return nil
}

// HasConfigurationChanges checks if there are configuration changes in the data model
func (sc *StateComparison) HasConfigurationChanges(ctx context.Context, data interface{}) bool {
	// This is a placeholder - the actual implementation would depend on the specific data model
	// For example, if using GraphqlMutationResourceModel, you would check if any configuration
	// fields have changed

	// For now, return false to indicate no configuration changes
	return false
}
