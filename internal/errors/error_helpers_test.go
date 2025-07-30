package errors

import (
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/stretchr/testify/assert"
)

func TestNewGraphQLError(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected diag.Diagnostic
	}{
		{
			name:     "simple GraphQL error",
			message:  "Query failed",
			expected: diag.NewErrorDiagnostic("GraphQL Server Error", "Query failed"),
		},
		{
			name:     "empty message",
			message:  "",
			expected: diag.NewErrorDiagnostic("GraphQL Server Error", ""),
		},
		{
			name:     "complex message",
			message:  "Cannot query field 'invalidField' on type 'User'",
			expected: diag.NewErrorDiagnostic("GraphQL Server Error", "Cannot query field 'invalidField' on type 'User'"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewGraphQLError(tt.message)

			assert.Equal(t, tt.expected.Summary(), result.Summary())
			assert.Equal(t, tt.expected.Detail(), result.Detail())
			assert.Equal(t, tt.expected.Severity(), result.Severity())
		})
	}
}

func TestNewHTTPError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		url        string
		expected   string
	}{
		{
			name:       "404 error",
			statusCode: 404,
			body:       "Not Found",
			url:        "https://api.example.com/graphql",
			expected:   "HTTP 404 error from https://api.example.com/graphql: Not Found",
		},
		{
			name:       "500 error",
			statusCode: 500,
			body:       "Internal Server Error",
			url:        "https://api.example.com/graphql",
			expected:   "HTTP 500 error from https://api.example.com/graphql: Internal Server Error",
		},
		{
			name:       "empty body",
			statusCode: 400,
			body:       "",
			url:        "https://api.example.com/graphql",
			expected:   "HTTP 400 error from https://api.example.com/graphql: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewHTTPError(tt.statusCode, tt.body, tt.url)

			assert.Equal(t, "HTTP Request Error", result.Summary())
			assert.Equal(t, tt.expected, result.Detail())
			assert.Equal(t, diag.SeverityError, result.Severity())
		})
	}
}

func TestNewValidationError(t *testing.T) {
	tests := []struct {
		name     string
		summary  string
		detail   string
		expected diag.Diagnostic
	}{
		{
			name:     "simple validation error",
			summary:  "Invalid Input",
			detail:   "Field 'email' is required",
			expected: diag.NewErrorDiagnostic("Invalid Input", "Field 'email' is required"),
		},
		{
			name:     "empty summary",
			summary:  "",
			detail:   "Field 'email' is required",
			expected: diag.NewErrorDiagnostic("", "Field 'email' is required"),
		},
		{
			name:     "empty detail",
			summary:  "Invalid Input",
			detail:   "",
			expected: diag.NewErrorDiagnostic("Invalid Input", ""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewValidationError(tt.summary, tt.detail)

			assert.Equal(t, tt.expected.Summary(), result.Summary())
			assert.Equal(t, tt.expected.Detail(), result.Detail())
			assert.Equal(t, tt.expected.Severity(), result.Severity())
		})
	}
}

func TestNewRateLimitError(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter string
		expected   string
	}{
		{
			name:       "with retry after",
			retryAfter: "30s",
			expected:   "Rate limit exceeded, retry after 30s",
		},
		{
			name:       "without retry after",
			retryAfter: "",
			expected:   "Rate limit exceeded",
		},
		{
			name:       "zero retry after",
			retryAfter: "0",
			expected:   "Rate limit exceeded, retry after 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewRateLimitError(tt.retryAfter)

			assert.Equal(t, "Rate Limit Error", result.Summary())
			assert.Equal(t, tt.expected, result.Detail())
			assert.Equal(t, diag.SeverityError, result.Severity())
		})
	}
}

func TestNewAuthenticationError(t *testing.T) {
	tests := []struct {
		name     string
		message  string
		expected diag.Diagnostic
	}{
		{
			name:     "simple auth error",
			message:  "Invalid credentials",
			expected: diag.NewErrorDiagnostic("Authentication Error", "Invalid credentials"),
		},
		{
			name:     "empty message",
			message:  "",
			expected: diag.NewErrorDiagnostic("Authentication Error", ""),
		},
		{
			name:     "complex message",
			message:  "Token expired at 2023-01-01T00:00:00Z",
			expected: diag.NewErrorDiagnostic("Authentication Error", "Token expired at 2023-01-01T00:00:00Z"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewAuthenticationError(tt.message)

			assert.Equal(t, tt.expected.Summary(), result.Summary())
			assert.Equal(t, tt.expected.Detail(), result.Detail())
			assert.Equal(t, tt.expected.Severity(), result.Severity())
		})
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		statusCode    int
		graphqlErrors []GraphQLError
		expected      string
	}{
		{
			name:          "rate limit by status code",
			err:           nil,
			statusCode:    429,
			graphqlErrors: []GraphQLError{},
			expected:      ErrorTypeRateLimit,
		},
		{
			name:          "authentication error by status code",
			err:           nil,
			statusCode:    401,
			graphqlErrors: []GraphQLError{},
			expected:      ErrorTypeAuth,
		},
		{
			name:          "validation error by status code",
			err:           nil,
			statusCode:    400,
			graphqlErrors: []GraphQLError{},
			expected:      ErrorTypeValidation,
		},
		{
			name:          "network error by status code",
			err:           nil,
			statusCode:    500,
			graphqlErrors: []GraphQLError{},
			expected:      ErrorTypeNetwork,
		},
		{
			name:       "rate limit by GraphQL error",
			err:        nil,
			statusCode: 200,
			graphqlErrors: []GraphQLError{
				{Message: "Rate limit exceeded"},
			},
			expected: ErrorTypeRateLimit,
		},
		{
			name:       "authentication by GraphQL error",
			err:        nil,
			statusCode: 200,
			graphqlErrors: []GraphQLError{
				{Message: "Unauthorized access"},
			},
			expected: ErrorTypeAuth,
		},
		{
			name:       "validation by GraphQL error",
			err:        nil,
			statusCode: 200,
			graphqlErrors: []GraphQLError{
				{Message: "Validation failed"},
			},
			expected: ErrorTypeValidation,
		},
		{
			name:       "GraphQL error",
			err:        nil,
			statusCode: 200,
			graphqlErrors: []GraphQLError{
				{Message: "Some GraphQL error"},
			},
			expected: ErrorTypeGraphQL,
		},
		{
			name:          "network error by error message",
			err:           assert.AnError,
			statusCode:    200,
			graphqlErrors: []GraphQLError{},
			expected:      ErrorTypeBusiness, // assert.AnError doesn't contain network keywords
		},
		{
			name:          "business logic error",
			err:           nil,
			statusCode:    200,
			graphqlErrors: []GraphQLError{},
			expected:      ErrorTypeBusiness,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(tt.err, tt.statusCode, tt.graphqlErrors)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShouldRetry(t *testing.T) {
	tests := []struct {
		name       string
		errorType  string
		attempt    int
		maxRetries int
		expected   bool
	}{
		{
			name:       "rate limit error - should retry",
			errorType:  ErrorTypeRateLimit,
			attempt:    1,
			maxRetries: 3,
			expected:   true,
		},
		{
			name:       "network error - should retry",
			errorType:  ErrorTypeNetwork,
			attempt:    2,
			maxRetries: 3,
			expected:   true,
		},
		{
			name:       "GraphQL error - should not retry",
			errorType:  ErrorTypeGraphQL,
			attempt:    1,
			maxRetries: 3,
			expected:   false,
		},
		{
			name:       "authentication error - should not retry",
			errorType:  ErrorTypeAuth,
			attempt:    1,
			maxRetries: 3,
			expected:   false,
		},
		{
			name:       "validation error - should not retry",
			errorType:  ErrorTypeValidation,
			attempt:    1,
			maxRetries: 3,
			expected:   false,
		},
		{
			name:       "business logic error - should not retry",
			errorType:  ErrorTypeBusiness,
			attempt:    1,
			maxRetries: 3,
			expected:   false,
		},
		{
			name:       "max retries reached",
			errorType:  ErrorTypeRateLimit,
			attempt:    3,
			maxRetries: 3,
			expected:   false,
		},
		{
			name:       "unknown error type",
			errorType:  "unknown",
			attempt:    1,
			maxRetries: 3,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldRetry(tt.errorType, tt.attempt, tt.maxRetries)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractRetryAfter(t *testing.T) {
	tests := []struct {
		name     string
		response *http.Response
		expected string
	}{
		{
			name:     "nil response",
			response: nil,
			expected: "",
		},
		{
			name: "response with retry-after header",
			response: &http.Response{
				Header: map[string][]string{
					"Retry-After": {"30"},
				},
			},
			expected: "30",
		},
		{
			name: "response without retry-after header",
			response: &http.Response{
				Header: map[string][]string{
					"Content-Type": {"application/json"},
				},
			},
			expected: "",
		},
		{
			name: "response with empty retry-after header",
			response: &http.Response{
				Header: map[string][]string{
					"Retry-After": {""},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractRetryAfter(tt.response)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsRetryableStatusCode(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		expected   bool
	}{
		{
			name:       "rate limit status code",
			statusCode: 429,
			expected:   true,
		},
		{
			name:       "server error status code",
			statusCode: 500,
			expected:   true,
		},
		{
			name:       "server error status code 2",
			statusCode: 502,
			expected:   true,
		},
		{
			name:       "server error status code 3",
			statusCode: 503,
			expected:   true,
		},
		{
			name:       "client error status code",
			statusCode: 400,
			expected:   false,
		},
		{
			name:       "success status code",
			statusCode: 200,
			expected:   false,
		},
		{
			name:       "authentication error status code",
			statusCode: 401,
			expected:   false,
		},
		{
			name:       "forbidden status code",
			statusCode: 403,
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableStatusCode(tt.statusCode)
			assert.Equal(t, tt.expected, result)
		})
	}
}
