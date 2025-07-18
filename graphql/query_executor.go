package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/logging"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func queryExecute(ctx context.Context, d *schema.ResourceData, m interface{}, querySource, variableSource string, usePagination bool) (*GqlQueryResponse, []byte, error) {
	query := d.Get(querySource).(string)

	var inputVariables map[string]interface{}
	rawVars, ok := d.GetOk(variableSource)
	if ok {
		if varsStr, isString := rawVars.(string); isString {
			if varsStr != "" {
				if err := json.Unmarshal([]byte(varsStr), &inputVariables); err != nil {
					return nil, nil, fmt.Errorf("failed to unmarshal variables from JSON string for key '%s': %w", variableSource, err)
				}
			}
		} else if varsMap, isMap := rawVars.(map[string]interface{}); isMap {
			inputVariables = varsMap
		} else {
			return nil, nil, fmt.Errorf("unexpected type for variable source '%s': expected json string or map", variableSource)
		}
	}

	apiURL := m.(*graphqlProviderConfig).GQLServerUrl
	headers := m.(*graphqlProviderConfig).RequestHeaders
	authorizationHeaders := m.(*graphqlProviderConfig).RequestAuthorizationHeaders

	if usePagination && querySource == "read_query" {
		return executePaginatedQuery(ctx, query, inputVariables, apiURL, headers, authorizationHeaders)
	}

	return executeSingleQuery(ctx, query, inputVariables, apiURL, headers, authorizationHeaders)
}

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

func executeGraphQLRequest(ctx context.Context, query string, variables map[string]interface{}, apiURL string, headers, authorizationHeaders map[string]interface{}) (*GqlQueryResponse, []byte, error) {
	var queryBodyBuffer bytes.Buffer

	queryObj := GqlQuery{
		Query:     query,
		Variables: variables,
	}

	if err := json.NewEncoder(&queryBodyBuffer).Encode(queryObj); err != nil {
		return nil, nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, &queryBodyBuffer)
	if err != nil {
		return nil, nil, err
	}

	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json; charset=utf-8")
	for key, value := range authorizationHeaders {
		req.Header.Set(key, value.(string))
	}
	for key, value := range headers {
		req.Header.Set(key, value.(string))
	}

	client := &http.Client{}
	if logging.IsDebugOrHigher() {
		log.Printf("[DEBUG] Enabling HTTP requests/responses tracing")
		client.Transport = logging.NewTransport("GraphQL", http.DefaultTransport)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var gqlResponse GqlQueryResponse
	if err := json.Unmarshal(body, &gqlResponse); err != nil {
		return nil, nil, fmt.Errorf("unable to parse graphql server response: %v ---> %s", err, string(body))
	}

	return &gqlResponse, body, nil
}

func executeSingleQuery(ctx context.Context, query string, inputVariables map[string]interface{}, apiURL string, headers, authorizationHeaders map[string]interface{}) (*GqlQueryResponse, []byte, error) {
	variables := prepareQueryVariables(inputVariables, "")
	return executeGraphQLRequest(ctx, query, variables, apiURL, headers, authorizationHeaders)
}

func executePaginatedQuery(ctx context.Context, query string, inputVariables map[string]interface{}, apiURL string, headers, authorizationHeaders map[string]interface{}) (*GqlQueryResponse, []byte, error) {
	var allResponses []GqlQueryResponse
	var finalResponseData []map[string]interface{}
	var finalResponseErrors []GqlError
	var lastCursor string

	for {
		variables := prepareQueryVariables(inputVariables, lastCursor)

		gqlResponse, _, err := executeGraphQLRequest(ctx, query, variables, apiURL, headers, authorizationHeaders)
		if err != nil {
			return nil, nil, err
		}

		allResponses = append(allResponses, *gqlResponse)

		pageInfo, ok := findPageInfo(gqlResponse.Data)
		if !ok {
			return nil, nil, fmt.Errorf("paginated query enabled but no pageInfo found in response (updated)")
		}

		hasNextPage, ok := pageInfo["hasNextPage"].(bool)
		if !ok {
			return nil, nil, fmt.Errorf("invalid or missing hasNextPage in pageInfo")
		}

		if !hasNextPage {
			break
		}

		endCursor, ok := pageInfo["endCursor"].(string)
		if !ok {
			return nil, nil, fmt.Errorf("invalid or missing endCursor in pageInfo")
		}
		lastCursor = endCursor
	}

	for _, resp := range allResponses {
		finalResponseData = append(finalResponseData, resp.Data)
		finalResponseErrors = append(finalResponseErrors, resp.Errors...)
	}

	finalResponse := GqlQueryResponse{
		PaginatedResponseData: finalResponseData,
		Errors:                finalResponseErrors,
	}

	responseBytes, err := json.Marshal(finalResponse)
	if err != nil {
		return nil, nil, fmt.Errorf("error marshaling merged response: %v", err)
	}
	return &finalResponse, responseBytes, nil
}

// findPageInfo recursively searches for the "pageInfo" key in a nested map[string]interface{}
func findPageInfo(data map[string]interface{}) (map[string]interface{}, bool) {
	for key, value := range data {
		if key == "pageInfo" {
			if pageInfo, ok := value.(map[string]interface{}); ok {
				return pageInfo, true
			}
		}
		if nestedMap, ok := value.(map[string]interface{}); ok {
			if pageInfo, found := findPageInfo(nestedMap); found {
				return pageInfo, true
			}
		}
	}
	return nil, false
}
