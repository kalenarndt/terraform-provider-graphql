package graphql

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// GqlQuery represents a GraphQL query with variables.
type GqlQuery struct {
	Query     string                 `json:"query,omitempty"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

// GqlQueryResponse represents a GraphQL response, including errors and paginated data.
type GqlQueryResponse struct {
	Data                  map[string]interface{}   `json:"data,omitempty"`
	Errors                []GqlError               `json:"errors,omitempty"`
	PaginatedResponseData []map[string]interface{} `json:"paginatedResponseData,omitempty"`
}

// GqlError represents a GraphQL error message.
type GqlError struct {
	Message string `json:"message,omitempty"`
}

// ProcessErrors converts GraphQL errors to Terraform diagnostics.
// This provides a standardized way to handle GraphQL errors in the provider.
func (r *GqlQueryResponse) ProcessErrors() diag.Diagnostics {
	var diags diag.Diagnostics
	if len(r.Errors) > 0 {
		for _, queryErr := range r.Errors {
			msg := fmt.Sprintf("graphql server error: %s", queryErr.Message)
			diags.AddError("GraphQL Server Error", msg)
		}
	}
	return diags
}
