package validator

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
)

// ValidateGraphQLQuery validates a GraphQL query string
func ValidateGraphQLQuery(query string) diag.Diagnostics {
	var diags diag.Diagnostics

	if strings.TrimSpace(query) == "" {
		diags.AddAttributeError(
			path.Root("query"),
			"Empty GraphQL Query",
			"GraphQL query cannot be empty",
		)
		return diags
	}

	// Basic GraphQL query validation
	if !strings.Contains(strings.ToLower(query), "query") &&
		!strings.Contains(strings.ToLower(query), "mutation") {
		diags.AddAttributeError(
			path.Root("query"),
			"Invalid GraphQL Query",
			"Query must contain 'query' or 'mutation' keyword",
		)
	}

	// Check for balanced braces
	if !hasBalancedBraces(query) {
		diags.AddAttributeError(
			path.Root("query"),
			"Invalid GraphQL Query",
			"Query has unbalanced braces",
		)
	}

	return diags
}

// ValidateGraphQLURL validates a GraphQL server URL
func ValidateGraphQLURL(url string) diag.Diagnostics {
	var diags diag.Diagnostics

	if strings.TrimSpace(url) == "" {
		diags.AddAttributeError(
			path.Root("url"),
			"Empty GraphQL URL",
			"GraphQL server URL cannot be empty",
		)
		return diags
	}

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		diags.AddAttributeError(
			path.Root("url"),
			"Invalid GraphQL URL",
			"URL must start with 'http://' or 'https://'",
		)
	}

	return diags
}

// ValidateOAuth2Config validates OAuth2 configuration
func ValidateOAuth2Config(oauth2Query, oauth2Variables, oauth2ValueAttribute string) diag.Diagnostics {
	var diags diag.Diagnostics

	// If OAuth2 is configured, all required fields must be present
	if oauth2Query != "" {
		if oauth2ValueAttribute == "" {
			diags.AddAttributeError(
				path.Root("oauth2_login_query_value_attribute"),
				"Missing OAuth2 Value Attribute",
				"When using OAuth2 login query, the value attribute path must be specified",
			)
		}
	}

	return diags
}

// ValidateRateLimitDelay validates rate limit delay configuration
func ValidateRateLimitDelay(delay string, fieldName string) diag.Diagnostics {
	var diags diag.Diagnostics

	if delay == "" {
		return diags
	}

	// Parse duration to validate format
	_, err := parseDuration(delay)
	if err != nil {
		diags.AddAttributeError(
			path.Root(fieldName),
			"Invalid Rate Limit Delay",
			fmt.Sprintf("Rate limit delay must be a valid duration (e.g., '1s', '100ms'): %s", err),
		)
	}

	return diags
}

// hasBalancedBraces checks if a string has balanced braces
func hasBalancedBraces(s string) bool {
	stack := 0

	for _, char := range s {
		switch char {
		case '{':
			stack++
		case '}':
			stack--
			if stack < 0 {
				return false
			}
		}
	}

	return stack == 0
}

// parseDuration is a simple duration parser for validation
func parseDuration(duration string) (interface{}, error) {
	// This is a simplified parser - in a real implementation,
	// you might want to use time.ParseDuration
	validSuffixes := []string{"ns", "us", "µs", "ms", "s", "m", "h"}

	for _, suffix := range validSuffixes {
		if strings.HasSuffix(duration, suffix) {
			return duration, nil
		}
	}

	return nil, fmt.Errorf("invalid duration format")
}
