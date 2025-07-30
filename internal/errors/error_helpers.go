package errors

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// GraphQLError represents a GraphQL error
type GraphQLError struct {
	Message    string                 `json:"message"`
	Locations  []GraphQLErrorLocation `json:"locations,omitempty"`
	Path       []string               `json:"path,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// GraphQLErrorLocation represents the location of a GraphQL error
type GraphQLErrorLocation struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// HTTPError represents an HTTP error
type HTTPError struct {
	StatusCode int
	Body       string
	URL        string
}

// Error types for classification
const (
	ErrorTypeNetwork    = "network"
	ErrorTypeGraphQL    = "graphql"
	ErrorTypeValidation = "validation"
	ErrorTypeRateLimit  = "rate_limit"
	ErrorTypeAuth       = "authentication"
	ErrorTypeBusiness   = "business_logic"
)

// NewGraphQLError creates a new GraphQL error diagnostic
func NewGraphQLError(message string) diag.Diagnostic {
	return diag.NewErrorDiagnostic(
		"GraphQL Server Error",
		message,
	)
}

// NewHTTPError creates a new HTTP error diagnostic
func NewHTTPError(statusCode int, body, url string) diag.Diagnostic {
	return diag.NewErrorDiagnostic(
		"HTTP Request Error",
		fmt.Sprintf("HTTP %d error from %s: %s", statusCode, url, body),
	)
}

// NewValidationError creates a new validation error diagnostic
func NewValidationError(summary, detail string) diag.Diagnostic {
	return diag.NewErrorDiagnostic(summary, detail)
}

// NewRateLimitError creates a new rate limit error diagnostic
func NewRateLimitError(retryAfter string) diag.Diagnostic {
	message := "Rate limit exceeded"
	if retryAfter != "" {
		message += fmt.Sprintf(", retry after %s", retryAfter)
	}
	return diag.NewErrorDiagnostic("Rate Limit Error", message)
}

// NewAuthenticationError creates a new authentication error diagnostic
func NewAuthenticationError(message string) diag.Diagnostic {
	return diag.NewErrorDiagnostic("Authentication Error", message)
}

// ClassifyError classifies an error based on its characteristics
func ClassifyError(err error, statusCode int, graphqlErrors []GraphQLError) string {
	// Check for HTTP status codes
	if statusCode == 429 {
		return ErrorTypeRateLimit
	}
	if statusCode == 401 || statusCode == 403 {
		return ErrorTypeAuth
	}
	if statusCode >= 400 && statusCode < 500 {
		return ErrorTypeValidation
	}
	if statusCode >= 500 {
		return ErrorTypeNetwork
	}

	// Check for GraphQL errors
	if len(graphqlErrors) > 0 {
		for _, gqlErr := range graphqlErrors {
			message := strings.ToLower(gqlErr.Message)
			if strings.Contains(message, "rate limit") || strings.Contains(message, "too many requests") {
				return ErrorTypeRateLimit
			}
			if strings.Contains(message, "unauthorized") || strings.Contains(message, "forbidden") {
				return ErrorTypeAuth
			}
			if strings.Contains(message, "validation") || strings.Contains(message, "invalid") {
				return ErrorTypeValidation
			}
		}
		return ErrorTypeGraphQL
	}

	// Check for network errors
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "timeout") ||
			strings.Contains(errMsg, "connection refused") ||
			strings.Contains(errMsg, "no route to host") {
			return ErrorTypeNetwork
		}
	}

	return ErrorTypeBusiness
}

// ShouldRetry determines if an error should be retried
func ShouldRetry(errorType string, attempt int, maxRetries int) bool {
	if attempt >= maxRetries {
		return false
	}

	switch errorType {
	case ErrorTypeRateLimit:
		return true
	case ErrorTypeNetwork:
		return true
	case ErrorTypeGraphQL:
		// Only retry certain GraphQL errors
		return false
	case ErrorTypeAuth:
		return false
	case ErrorTypeValidation:
		return false
	case ErrorTypeBusiness:
		return false
	default:
		return false
	}
}

// LogError logs an error with appropriate context
func LogError(ctx context.Context, errorType string, err error, additionalFields map[string]interface{}) {
	fields := map[string]interface{}{
		"error_type": errorType,
		"error":      err.Error(),
	}

	// Add additional fields
	for k, v := range additionalFields {
		fields[k] = v
	}

	switch errorType {
	case ErrorTypeRateLimit:
		tflog.Warn(ctx, "Rate limit error occurred", fields)
	case ErrorTypeNetwork:
		tflog.Error(ctx, "Network error occurred", fields)
	case ErrorTypeAuth:
		tflog.Error(ctx, "Authentication error occurred", fields)
	case ErrorTypeValidation:
		tflog.Error(ctx, "Validation error occurred", fields)
	case ErrorTypeGraphQL:
		tflog.Error(ctx, "GraphQL error occurred", fields)
	default:
		tflog.Error(ctx, "Unexpected error occurred", fields)
	}
}

// ExtractRetryAfter extracts retry-after header from HTTP response
func ExtractRetryAfter(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	return resp.Header.Get("Retry-After")
}

// IsRetryableStatusCode checks if an HTTP status code indicates a retryable error
func IsRetryableStatusCode(statusCode int) bool {
	return statusCode == 429 || // Rate limit
		statusCode >= 500 && statusCode < 600 // Server errors
}
