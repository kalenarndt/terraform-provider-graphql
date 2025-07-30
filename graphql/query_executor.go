package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/time/rate"
)

// Global rate limiters for coordination across all requests
var (
	queryRateLimiter    *rate.Limiter
	mutationRateLimiter *rate.Limiter
	rateLimitMutex      sync.Mutex
)

// initializeRateLimiters initializes the global rate limiters
func initializeRateLimiters(queryDelay, mutationDelay time.Duration) {
	rateLimitMutex.Lock()
	defer rateLimitMutex.Unlock()

	if queryDelay > 0 {
		queryRateLimiter = rate.NewLimiter(rate.Every(queryDelay), 1)
	}
	if mutationDelay > 0 {
		mutationRateLimiter = rate.NewLimiter(rate.Every(mutationDelay), 1)
	}
}

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

	tflog.Debug(ctx, "Parsed variables", map[string]any{
		"inputVariables": inputVariables,
	})

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
	default:
		return v
	}
}

// executeGraphQLRequestFramework executes a GraphQL request with improved rate limiting support
func executeGraphQLRequestFramework(ctx context.Context, query string, variables map[string]interface{}, config *graphqlProviderConfig) (*GqlQueryResponse, []byte, diag.Diagnostics) {
	var diags diag.Diagnostics
	maxRetries := 5
	baseDelay := time.Second

	// Determine if this is a mutation or query based on the query content
	isMutation := strings.Contains(strings.ToLower(query), "mutation")

	// Initialize rate limiters if not already done
	if queryRateLimiter == nil || mutationRateLimiter == nil {
		initializeRateLimiters(config.QueryRateLimitDelay, config.MutationRateLimitDelay)
	}

	// Wait for rate limiter before making the request
	var limiter *rate.Limiter
	if isMutation {
		limiter = mutationRateLimiter
	} else {
		limiter = queryRateLimiter
	}

	if limiter != nil {
		if err := limiter.Wait(ctx); err != nil {
			diags.AddError("Rate Limiter Error", fmt.Sprintf("failed to wait for rate limiter: %v", err))
			return nil, nil, diags
		}
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		queryResponse, bodyBytes, attemptDiags := executeSingleGraphQLRequest(ctx, query, variables, config)

		// If no errors, return success
		if !attemptDiags.HasError() {
			return queryResponse, bodyBytes, attemptDiags
		}

		// Check if this is a rate limit error
		if isRateLimitError(attemptDiags) {
			if attempt < maxRetries {
				// Try to parse retry time from the error response
				retryDelay := parseRetryDelay(attemptDiags)
				if retryDelay > 0 {
					tflog.Debug(ctx, "Rate limited, retrying with API-specified delay", map[string]any{
						"attempt":    attempt + 1,
						"retryDelay": retryDelay,
						"operation":  isMutation,
					})
					time.Sleep(retryDelay)
				} else {
					// Fallback to exponential backoff with jitter
					delay := time.Duration(attempt+1) * baseDelay
					// Add jitter to prevent thundering herd
					jitter := time.Duration(attempt+1) * 100 * time.Millisecond
					delay += jitter
					tflog.Debug(ctx, "Rate limited, retrying with exponential backoff", map[string]any{
						"attempt":   attempt + 1,
						"delay":     delay,
						"operation": isMutation,
					})
					time.Sleep(delay)
				}
				continue
			}
		}

		// Check if this is a business logic error (don't retry these)
		if isBusinessLogicError(attemptDiags) {
			tflog.Debug(ctx, "Business logic error, not retrying", map[string]any{
				"attempt":   attempt + 1,
				"operation": isMutation,
			})
			return queryResponse, bodyBytes, attemptDiags
		}

		// If not a rate limit error or max retries reached, return the error
		return queryResponse, bodyBytes, attemptDiags
	}

	return nil, nil, diags
}

// parseRetryDelay extracts the retryAfterNS from rate limit error responses
func parseRetryDelay(diags diag.Diagnostics) time.Duration {
	for _, d := range diags {
		if strings.Contains(d.Detail(), "HTTP 429") {
			// Try to extract retryAfterNS from the error message
			if strings.Contains(d.Detail(), "retryAfterNS") {
				// Look for retryAfterNS in the JSON response
				start := strings.Index(d.Detail(), `"retryAfterNS":`)
				if start != -1 {
					start += len(`"retryAfterNS":`)
					end := strings.Index(d.Detail()[start:], ",")
					if end == -1 {
						end = strings.Index(d.Detail()[start:], "}")
					}
					if end != -1 {
						retryStr := strings.TrimSpace(d.Detail()[start : start+end])
						if retry, err := strconv.ParseInt(retryStr, 10, 64); err == nil {
							// Convert nanoseconds to duration
							return time.Duration(retry) * time.Nanosecond
						}
					}
				}
			}
		}
	}
	return 0
}

// executeSingleGraphQLRequest executes a single GraphQL request
func executeSingleGraphQLRequest(ctx context.Context, query string, variables map[string]interface{}, config *graphqlProviderConfig) (*GqlQueryResponse, []byte, diag.Diagnostics) {
	var diags diag.Diagnostics

	// Prepare request body
	requestBody := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	queryBodyBuffer := &bytes.Buffer{}
	if err := json.NewEncoder(queryBodyBuffer).Encode(requestBody); err != nil {
		diags.AddError("Request Encoding Error", fmt.Sprintf("failed to encode request body: %v", err))
		return nil, nil, diags
	}

	tflog.Debug(ctx, "Sending GraphQL request", map[string]any{
		"url":           config.GQLServerUrl,
		"headers":       config.RequestHeaders,
		"variables":     variables,
		"query":         query,
		"variablesJSON": string(queryBodyBuffer.Bytes()),
	})

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", config.GQLServerUrl, queryBodyBuffer)
	if err != nil {
		diags.AddError("Request Creation Error", fmt.Sprintf("failed to create request: %v", err))
		return nil, nil, diags
	}

	// Set headers
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

	// Execute request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		diags.AddError("HTTP Request Error", fmt.Sprintf("failed to execute request: %v", err))
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

// isRateLimitError checks if the error is a rate limit error (429)
func isRateLimitError(diags diag.Diagnostics) bool {
	for _, d := range diags {
		if strings.Contains(d.Detail(), "HTTP 429") || strings.Contains(d.Detail(), "Rate limit") {
			return true
		}
	}
	return false
}

// isBusinessLogicError checks if the error is a business logic error (not rate limit)
func isBusinessLogicError(diags diag.Diagnostics) bool {
	for _, d := range diags {
		detail := d.Detail()
		if strings.Contains(detail, "Can't enable multiple versions") ||
			strings.Contains(detail, "already enabled") ||
			strings.Contains(detail, "already exists") ||
			strings.Contains(detail, "conflict") {
			return true
		}
	}
	return false
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
