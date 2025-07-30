package utils

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// ResponseExtraction provides utilities for extracting data from GraphQL responses
type ResponseExtraction struct{}

// ExtractCurrentStateFromQueryResponse intelligently extracts the current state
// from a GraphQL query response using heuristics to determine the best data source
func (re *ResponseExtraction) ExtractCurrentStateFromQueryResponse(ctx context.Context, queryResponse map[string]interface{}) map[string]interface{} {
	tflog.Debug(ctx, "Extracting current state from query response", map[string]any{
		"queryResponse": queryResponse,
	})

	// Handle the specific GraphQL response structure
	if data, ok := queryResponse["data"].(map[string]interface{}); ok {
		tflog.Debug(ctx, "Found data object in response", map[string]any{
			"dataKeys": re.getMapKeys(data),
		})

		// Strategy 1: Look for paginated responses with nodes (most common pattern)
		for key, value := range data {
			if resourceData, ok := value.(map[string]interface{}); ok {
				// Handle paginated responses with nodes
				if nodes, hasNodes := resourceData["nodes"].([]interface{}); hasNodes && len(nodes) > 0 {
					tflog.Debug(ctx, "Found nodes array", map[string]any{
						"key":       key,
						"nodeCount": len(nodes),
					})

					if firstNode, ok := nodes[0].(map[string]interface{}); ok {
						tflog.Debug(ctx, "Extracted first node", map[string]any{
							"firstNode": firstNode,
						})

						// Remove computed fields that shouldn't be compared
						re.removeComputedFields(firstNode)

						tflog.Debug(ctx, "Returning extracted state from nodes", map[string]any{
							"extractedState": firstNode,
						})

						return firstNode
					}
				}
			}
		}

		// Strategy 2: Use resource scoring to find the best candidate
		bestKey := ""
		bestScore := 0
		bestData := map[string]interface{}{}

		for key, value := range data {
			if resourceData, ok := value.(map[string]interface{}); ok {
				score := re.scoreResourceCandidate(resourceData)
				tflog.Debug(ctx, "Evaluating resource candidate", map[string]any{
					"key":   key,
					"score": score,
				})

				if score > bestScore {
					bestScore = score
					bestKey = key
					bestData = resourceData
				}
			}
		}

		if bestScore > 0 {
			// Remove computed fields that shouldn't be compared
			re.removeComputedFields(bestData)

			tflog.Debug(ctx, "Returning best resource data", map[string]any{
				"bestKey":      bestKey,
				"bestScore":    bestScore,
				"resourceData": bestData,
			})

			return bestData
		}
	}

	// Fallback: return the entire response
	tflog.Debug(ctx, "No suitable data found, returning entire response")
	return queryResponse
}

// scoreResourceCandidate scores a resource data object based on its characteristics
// Higher scores indicate better candidates for state extraction
func (re *ResponseExtraction) scoreResourceCandidate(data map[string]interface{}) int {
	score := 0

	// Score based on field types and presence
	for key, value := range data {
		switch key {
		case "id":
			if str, ok := value.(string); ok && str != "" {
				score += 20 // High score for valid ID
			}
		case "name":
			if str, ok := value.(string); ok && str != "" {
				score += 15 // Good score for name
			}
		case "enabled":
			if _, ok := value.(bool); ok {
				score += 10 // Good score for boolean fields
			}
		case "status":
			if str, ok := value.(string); ok && str != "" {
				score += 10 // Good score for status
			}
		case "type":
			if str, ok := value.(string); ok && str != "" {
				score += 8 // Moderate score for type
			}
		case "config":
			if _, ok := value.(map[string]interface{}); ok {
				score += 12 // Good score for config objects
			}
		case "authParams":
			if _, ok := value.(map[string]interface{}); ok {
				score += 10 // Good score for auth params
			}
		case "extraConfig":
			if _, ok := value.(map[string]interface{}); ok {
				score += 10 // Good score for extra config
			}
		default:
			// Generic scoring for other fields
			switch v := value.(type) {
			case string:
				if v != "" {
					score += 3
				}
			case bool:
				score += 5
			case map[string]interface{}:
				score += 8
			case []interface{}:
				score += 4
			}
		}
	}

	// Bonus for having multiple good fields
	if len(data) > 3 {
		score += 5
	}

	return score
}

// removeComputedFields removes fields that shouldn't be compared in state comparison
func (re *ResponseExtraction) removeComputedFields(data map[string]interface{}) {
	computedFields := []string{"id", "createdAt", "updatedAt", "status"}
	for _, field := range computedFields {
		delete(data, field)
	}
}

// getMapKeys returns the keys of a map as a slice of strings
func (re *ResponseExtraction) getMapKeys(data map[string]interface{}) []string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	return keys
}

// ExtractValueFromPath extracts a value from a nested map using a dot-separated path
func (re *ResponseExtraction) ExtractValueFromPath(data map[string]interface{}, path string) (interface{}, error) {
	parts := strings.Split(path, ".")
	current := data

	for i, part := range parts {
		if i == len(parts)-1 {
			// Last part - return the value
			if value, exists := current[part]; exists {
				return value, nil
			}
			return nil, fmt.Errorf("path not found: %s", path)
		}

		// Navigate to the next level
		if next, ok := current[part].(map[string]interface{}); ok {
			current = next
		} else {
			return nil, fmt.Errorf("invalid path at %s: expected map, got %T", part, current[part])
		}
	}

	return nil, fmt.Errorf("path not found: %s", path)
}

// IsValidResourceData checks if the given data looks like a valid resource
func (re *ResponseExtraction) IsValidResourceData(data map[string]interface{}) bool {
	// Check for common resource identifiers
	hasID := false
	hasName := false
	hasType := false

	for key, value := range data {
		switch key {
		case "id":
			if str, ok := value.(string); ok && str != "" {
				hasID = true
			}
		case "name":
			if str, ok := value.(string); ok && str != "" {
				hasName = true
			}
		case "type":
			if str, ok := value.(string); ok && str != "" {
				hasType = true
			}
		}
	}

	// A valid resource should have at least an ID or a name
	return hasID || hasName || hasType
}
