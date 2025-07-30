package config

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"golang.org/x/time/rate"
)

// ProviderConfig holds the provider configuration
type ProviderConfig struct {
	URL                            string
	Headers                        map[string]interface{}
	OAuth2LoginQuery               string
	OAuth2LoginQueryVariables      map[string]interface{}
	OAuth2LoginQueryValueAttribute string
	OAuth2RestURL                  string
	OAuth2RestMethod               string
	OAuth2RestHeaders              map[string]interface{}
	OAuth2RestBody                 string
	OAuth2RestTokenPath            string
	QueryRateLimitDelay            time.Duration
	MutationRateLimitDelay         time.Duration
	QueryRateLimiter               *rate.Limiter
	MutationRateLimiter            *rate.Limiter
}

// NewProviderConfig creates a new provider configuration
func NewProviderConfig() *ProviderConfig {
	return &ProviderConfig{
		Headers:                   make(map[string]interface{}),
		OAuth2LoginQueryVariables: make(map[string]interface{}),
		OAuth2RestHeaders:         make(map[string]interface{}),
		QueryRateLimitDelay:       0,
		MutationRateLimitDelay:    0,
	}
}

// Validate validates the provider configuration
func (c *ProviderConfig) Validate(ctx context.Context) diag.Diagnostics {
	var diags diag.Diagnostics

	if c.URL == "" {
		diags.AddError(
			"Missing GraphQL URL",
			"The provider cannot create the GraphQL client as there is an unknown or missing configuration value for the GraphQL URL.",
		)
		return diags
	}

	// Validate OAuth2 configuration
	if c.OAuth2LoginQuery != "" && c.OAuth2LoginQueryValueAttribute == "" {
		diags.AddError(
			"Missing OAuth2 Value Attribute",
			"When using OAuth2 login query, the value attribute path must be specified.",
		)
	}

	// Validate rate limiting configuration
	if c.QueryRateLimitDelay < 0 {
		diags.AddError(
			"Invalid Query Rate Limit Delay",
			"Query rate limit delay cannot be negative.",
		)
	}

	if c.MutationRateLimitDelay < 0 {
		diags.AddError(
			"Invalid Mutation Rate Limit Delay",
			"Mutation rate limit delay cannot be negative.",
		)
	}

	return diags
}

// InitializeRateLimiters initializes the rate limiters based on configuration
func (c *ProviderConfig) InitializeRateLimiters() {
	if c.QueryRateLimitDelay > 0 {
		c.QueryRateLimiter = rate.NewLimiter(rate.Every(c.QueryRateLimitDelay), 1)
		tflog.Debug(context.Background(), "Initialized query rate limiter", map[string]any{
			"delay": c.QueryRateLimitDelay,
		})
	}

	if c.MutationRateLimitDelay > 0 {
		c.MutationRateLimiter = rate.NewLimiter(rate.Every(c.MutationRateLimitDelay), 1)
		tflog.Debug(context.Background(), "Initialized mutation rate limiter", map[string]any{
			"delay": c.MutationRateLimitDelay,
		})
	}
}

// GetRequestHeaders returns the combined headers for requests
func (c *ProviderConfig) GetRequestHeaders() map[string]interface{} {
	headers := make(map[string]interface{})

	// Add default headers
	headers["Content-Type"] = "application/json"
	headers["Accept"] = "application/json"

	// Add custom headers
	for k, v := range c.Headers {
		headers[k] = v
	}

	return headers
}

// GetAuthorizationHeaders returns the authorization headers
func (c *ProviderConfig) GetAuthorizationHeaders() map[string]interface{} {
	headers := make(map[string]interface{})

	// Add OAuth2 headers if configured
	if c.OAuth2LoginQueryValueAttribute != "" {
		// This would be populated with the actual token
		// Implementation depends on OAuth2 flow
	}

	return headers
}

// ParseDuration parses a duration string into time.Duration
func ParseDuration(durationStr string) (time.Duration, error) {
	if durationStr == "" {
		return 0, nil
	}

	return time.ParseDuration(durationStr)
}

// ConvertMapToInterface converts a types.Map to map[string]interface{}
func ConvertMapToInterface(ctx context.Context, mapValue types.Map) (map[string]interface{}, diag.Diagnostics) {
	var diags diag.Diagnostics

	if mapValue.IsNull() || mapValue.IsUnknown() {
		return make(map[string]interface{}), diags
	}

	var result map[string]interface{}
	diags.Append(mapValue.ElementsAs(ctx, &result, false)...)

	return result, diags
}
