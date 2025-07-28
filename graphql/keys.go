package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/tidwall/gjson"
)

// computeMutationVariableKeys computes mutation variable keys from a key map and a response object.
// It extracts values from the response JSON using the provided paths and returns them as a map.
func computeMutationVariableKeys(keyMaps map[string]interface{}, responseJSON string) (map[string]string, error) {
	mvks := make(map[string]string)

	// Debug: Log the response structure for troubleshooting
	tflog.Debug(context.Background(), "Processing GraphQL response", map[string]any{
		"responseLength":  len(responseJSON),
		"responsePreview": responseJSON[:min(500, len(responseJSON))],
	})

	for k, v := range keyMaps {
		path, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("path for key '%s' is not a string", k)
		}

		// Try the direct path first
		fullPath := "data." + path
		result := gjson.Get(responseJSON, fullPath)

		// If not found, try paginated path
		if !result.Exists() {
			paginatedPath := "data.paginatedData.0." + path
			result = gjson.Get(responseJSON, paginatedPath)
			if !result.Exists() {
				// Fallback for responses that might not be wrapped in "data"
				result = gjson.Get(responseJSON, path)
				if !result.Exists() {
					result = gjson.Get(responseJSON, "paginatedData.0."+path)
					if !result.Exists() {
						tflog.Debug(context.Background(), "Path not found, logging available paths", map[string]any{
							"searchedPath":  fullPath,
							"paginatedPath": paginatedPath,
							"fallbackPath":  path,
							"responseKeys":  getTopLevelKeys(responseJSON),
						})
						return nil, fmt.Errorf("the path '%s' does not exist in the response (tried: '%s', '%s', '%s', '%s'). Available top-level keys: %v", path, fullPath, paginatedPath, path, "paginatedData.0."+path, getTopLevelKeys(responseJSON))
					}
				}
			}
		}

		mvks[k] = result.String()
		tflog.Debug(context.Background(), "Successfully extracted value", map[string]any{
			"key":   k,
			"path":  path,
			"value": result.String(),
		})
	}
	return mvks, nil
}

// getTopLevelKeys extracts the top-level keys from a JSON response for debugging
func getTopLevelKeys(responseJSON string) []string {
	var response map[string]interface{}
	if err := json.Unmarshal([]byte(responseJSON), &response); err != nil {
		return []string{"error_parsing_json"}
	}

	keys := make([]string, 0, len(response))
	for k := range response {
		keys = append(keys, k)
	}
	return keys
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// hash returns a stable hash code for a JSON-encoded byte slice.
// This is used to generate consistent IDs for resources based on their content.
func hash(v []byte) int {
	// Use the raw bytes directly to avoid field ordering issues
	// that can occur when unmarshaling and remarshaling JSON
	return hashCodeString(string(v))
}

// hashCodeString hashes a string to a unique non-negative integer using crc32.
// This provides a consistent way to generate numeric IDs from string content.
func hashCodeString(s string) int {
	v := int(crc32.ChecksumIEEE([]byte(s)))
	if v >= 0 {
		return v
	}
	if -v >= 0 {
		return -v
	}
	// v == MinInt
	return 0
}

// findPathForValue recursively searches through JSON to find the path to a given value.
// This is useful for debugging and understanding the structure of GraphQL responses.
func findPathForValue(jsonStr string, targetValue string) string {
	var result string

	// Parse the JSON and search recursively
	gjson.Parse(jsonStr).ForEach(func(key, value gjson.Result) bool {
		if value.String() == targetValue {
			result = key.String()
			return false // stop iterating
		}

		// If this is an object or array, search recursively
		if value.IsObject() || value.IsArray() {
			if found := findPathForValue(value.Raw, targetValue); found != "" {
				result = key.String() + "." + found
				return false // stop iterating
			}
		}

		return true // continue
	})

	return result
}
