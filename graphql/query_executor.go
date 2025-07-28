package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// queryExecuteFramework executes a GraphQL query using the new framework patterns
func queryExecuteFramework(ctx context.Context, config *graphqlProviderConfig, query, variableSource string, usePagination bool) (*GqlQueryResponse, []byte, diag.Diagnostics) {
	var diags diag.Diagnostics

	tflog.Debug(ctx, "Executing GraphQL query", map[string]any{
		"query":          query,
		"variableSource": variableSource,
		"usePagination":  usePagination,
	})

	var inputVariables map[string]interface{}
	if variableSource != "" {
		if err := json.Unmarshal([]byte(variableSource), &inputVariables); err != nil {
			diags.AddError("Variable Parsing Error", fmt.Sprintf("failed to unmarshal variables from JSON string: %v", err))
			return nil, nil, diags
		}
	}

	if usePagination {
		return executePaginatedQueryFramework(ctx, query, inputVariables, config)
	}

	return executeSingleQueryFramework(ctx, query, inputVariables, config)
}

// prepareQueryVariables prepares variables for GraphQL queries, handling pagination
func prepareQueryVariables(inputVariables map[string]interface{}, cursor string) map[string]interface{} {
	if inputVariables == nil {
		if cursor != "" {
			return map[string]interface{}{"after": cursor}
		}
		return make(map[string]interface{})
	}

	processedVars := recursivelyPrepareVariables(inputVariables).(map[string]interface{})

	if cursor != "" {
		processedVars["after"] = cursor
	}

	return processedVars
}

// recursivelyPrepareVariables recursively processes variables to handle JSON strings
func recursivelyPrepareVariables(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		newMap := make(map[string]interface{}, len(v))
		for key, val := range v {
			newMap[key] = recursivelyPrepareVariables(val)
		}
		return newMap
	case []interface{}:
		newSlice := make([]interface{}, len(v))
		for i, val := range v {
			newSlice[i] = recursivelyPrepareVariables(val)
		}
		return newSlice
	case string:
		var js interface{}
		if err := json.Unmarshal([]byte(v), &js); err == nil {
			return js
		}
		return v
	default:
		return v
	}
}

// executeGraphQLRequestFramework executes a single GraphQL request
func executeGraphQLRequestFramework(ctx context.Context, query string, variables map[string]interface{}, config *graphqlProviderConfig) (*GqlQueryResponse, []byte, diag.Diagnostics) {
	var diags diag.Diagnostics
	var queryBodyBuffer bytes.Buffer

	queryObj := GqlQuery{
		Query:     query,
		Variables: variables,
	}

	if err := json.NewEncoder(&queryBodyBuffer).Encode(queryObj); err != nil {
		diags.AddError("Request Encoding Error", fmt.Sprintf("failed to encode query: %v", err))
		return nil, nil, diags
	}

	req, err := http.NewRequestWithContext(ctx, "POST", config.GQLServerUrl, &queryBodyBuffer)
	if err != nil {
		diags.AddError("Request Creation Error", fmt.Sprintf("failed to create request: %v", err))
		return nil, nil, diags
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json; charset=utf-8")

	// Add authorization headers
	for key, value := range config.RequestAuthorizationHeaders {
		req.Header.Set(key, fmt.Sprintf("%v", value))
	}

	// Add custom headers
	for key, value := range config.RequestHeaders {
		req.Header.Set(key, fmt.Sprintf("%v", value))
	}

	tflog.Debug(ctx, "Sending GraphQL request", map[string]any{
		"url":     config.GQLServerUrl,
		"headers": req.Header,
	})

	// Create HTTP client with timeouts for better reliability
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		diags.AddError("Request Execution Error", fmt.Sprintf("failed to execute request: %v", err))
		return nil, nil, diags
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		diags.AddError("Response Reading Error", fmt.Sprintf("failed to read response body: %v", err))
		return nil, nil, diags
	}

	tflog.Debug(ctx, "Received GraphQL response", map[string]any{
		"statusCode": resp.StatusCode,
		"bodyLength": len(bodyBytes),
	})

	if resp.StatusCode != http.StatusOK {
		diags.AddError("HTTP Error", fmt.Sprintf("received HTTP %d: %s", resp.StatusCode, string(bodyBytes)))
		return nil, nil, diags
	}

	var queryResponse GqlQueryResponse
	if err := json.Unmarshal(bodyBytes, &queryResponse); err != nil {
		diags.AddError("Response Parsing Error", fmt.Sprintf("failed to parse response: %v", err))
		return nil, nil, diags
	}

	return &queryResponse, bodyBytes, diags
}

// executeSingleQueryFramework executes a single GraphQL query
func executeSingleQueryFramework(ctx context.Context, query string, inputVariables map[string]interface{}, config *graphqlProviderConfig) (*GqlQueryResponse, []byte, diag.Diagnostics) {
	variables := prepareQueryVariables(inputVariables, "")
	return executeGraphQLRequestFramework(ctx, query, variables, config)
}

// executePaginatedQueryFramework executes a paginated GraphQL query
func executePaginatedQueryFramework(ctx context.Context, query string, inputVariables map[string]interface{}, config *graphqlProviderConfig) (*GqlQueryResponse, []byte, diag.Diagnostics) {
	var diags diag.Diagnostics
	var allData []map[string]interface{}
	var cursor string

	for {
		variables := prepareQueryVariables(inputVariables, cursor)
		queryResponse, _, queryDiags := executeGraphQLRequestFramework(ctx, query, variables, config)
		if queryDiags.HasError() {
			diags.Append(queryDiags...)
			return nil, nil, diags
		}

		if len(queryResponse.Errors) > 0 {
			for _, gqlErr := range queryResponse.Errors {
				diags.AddError("GraphQL Server Error", gqlErr.Message)
			}
			return nil, nil, diags
		}

		// Extract data from response
		data, hasNextPage, nextCursor := extractPaginatedData(queryResponse.Data)
		if data != nil {
			allData = append(allData, data)
		}

		if !hasNextPage {
			break
		}

		cursor = nextCursor
		if cursor == "" {
			break
		}
	}

	// Create combined response
	combinedResponse := &GqlQueryResponse{
		Data: map[string]interface{}{
			"paginatedData": allData,
		},
		PaginatedResponseData: allData,
	}

	// Marshal the combined response
	combinedBytes, err := json.Marshal(combinedResponse)
	if err != nil {
		diags.AddError("Response Marshaling Error", fmt.Sprintf("failed to marshal combined response: %v", err))
		return nil, nil, diags
	}

	return combinedResponse, combinedBytes, diags
}

// extractPaginatedData extracts data from a paginated response
func extractPaginatedData(data map[string]interface{}) (map[string]interface{}, bool, string) {
	if data == nil {
		return nil, false, ""
	}

	// Look for common pagination patterns
	for _, value := range data {
		if pageInfo, ok := value.(map[string]interface{}); ok {
			// Check if this looks like a paginated result
			if _, hasEdges := pageInfo["edges"]; hasEdges {
				if pageInfoData, ok := pageInfo["pageInfo"].(map[string]interface{}); ok {
					hasNextPage := false
					if hasNextPageVal, ok := pageInfoData["hasNextPage"].(bool); ok {
						hasNextPage = hasNextPageVal
					}

					endCursor := ""
					if endCursorVal, ok := pageInfoData["endCursor"].(string); ok {
						endCursor = endCursorVal
					}

					return pageInfo, hasNextPage, endCursor
				}
			}
		}
	}

	// If no pagination structure found, return the data as-is
	return data, false, ""
}

// findPageInfo finds page information in a response
func findPageInfo(data map[string]interface{}) (map[string]interface{}, bool) {
	if data == nil {
		return nil, false
	}

	// Look for pageInfo in common locations
	for _, value := range data {
		if pageInfo, ok := value.(map[string]interface{}); ok {
			if _, hasPageInfo := pageInfo["pageInfo"]; hasPageInfo {
				return pageInfo, true
			}
		}
	}

	return nil, false
}
