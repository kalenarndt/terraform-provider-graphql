package testutils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jarcoal/httpmock"
)

// MockGraphQLServer creates a mock GraphQL server for testing
func MockGraphQLServer(t *testing.T, responses map[string]interface{}) (*httptest.Server, func()) {
	httpmock.Activate()

	// Register mock responses
	for query, response := range responses {
		httpmock.RegisterResponder("POST", "http://localhost:8080/graphql",
			func(req *http.Request) (*http.Response, error) {
				// Parse the request body to match queries
				var requestBody map[string]interface{}
				if err := json.NewDecoder(req.Body).Decode(&requestBody); err != nil {
					return httpmock.NewStringResponse(400, `{"errors":[{"message":"Invalid JSON"}]}`), nil
				}

				// Check if this is the query we want to mock
				if reqQuery, ok := requestBody["query"].(string); ok && reqQuery == query {
					responseBytes, _ := json.Marshal(response)
					return httpmock.NewBytesResponse(200, responseBytes), nil
				}

				// Default response
				return httpmock.NewStringResponse(200, `{"data":{}}`), nil
			},
		)
	}

	cleanup := func() {
		httpmock.DeactivateAndReset()
	}

	return nil, cleanup
}

// MockGraphQLError creates a mock GraphQL error response
func MockGraphQLError(t *testing.T, errorMessage string) (*httptest.Server, func()) {
	httpmock.Activate()

	errorResponse := map[string]interface{}{
		"errors": []map[string]interface{}{
			{"message": errorMessage},
		},
	}

	httpmock.RegisterResponder("POST", "http://localhost:8080/graphql",
		func(req *http.Request) (*http.Response, error) {
			responseBytes, _ := json.Marshal(errorResponse)
			return httpmock.NewBytesResponse(200, responseBytes), nil
		},
	)

	cleanup := func() {
		httpmock.DeactivateAndReset()
	}

	return nil, cleanup
}

// MockOAuth2Server creates a mock OAuth2 server for testing
func MockOAuth2Server(t *testing.T, tokenResponse map[string]interface{}) (*httptest.Server, func()) {
	httpmock.Activate()

	httpmock.RegisterResponder("POST", "http://localhost:8080/oauth/token",
		func(req *http.Request) (*http.Response, error) {
			responseBytes, _ := json.Marshal(tokenResponse)
			return httpmock.NewBytesResponse(200, responseBytes), nil
		},
	)

	cleanup := func() {
		httpmock.DeactivateAndReset()
	}

	return nil, cleanup
}

// CreateMockGraphQLResponse creates a standard GraphQL response structure
func CreateMockGraphQLResponse(data map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"data": data,
	}
}

// CreateMockGraphQLError creates a standard GraphQL error response structure
func CreateMockGraphQLError(message string) map[string]interface{} {
	return map[string]interface{}{
		"errors": []map[string]interface{}{
			{"message": message},
		},
	}
}
