package graphql

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/kalenarndt/terraform-provider-graphql/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGraphqlProvider_Metadata(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected string
	}{
		{
			name:     "test version",
			version:  "test",
			expected: "test",
		},
		{
			name:     "dev version",
			version:  "dev",
			expected: "dev",
		},
		{
			name:     "release version",
			version:  "v1.0.0",
			expected: "v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &GraphqlProvider{version: tt.version}
			req := provider.MetadataRequest{}
			resp := &provider.MetadataResponse{}

			p.Metadata(context.Background(), req, resp)

			assert.Equal(t, "graphql", resp.TypeName)
			assert.Equal(t, tt.expected, resp.Version)
		})
	}
}

func TestGraphqlProvider_Schema(t *testing.T) {
	p := &GraphqlProvider{}
	req := provider.SchemaRequest{}
	resp := &provider.SchemaResponse{}

	p.Schema(context.Background(), req, resp)

	require.NotNil(t, resp.Schema)
	assert.Equal(t, "Interact with GraphQL APIs.", resp.Schema.Description)

	// Check required fields
	urlAttr, ok := resp.Schema.Attributes["url"]
	require.True(t, ok)
	assert.True(t, urlAttr.IsRequired())

	// Check optional fields
	headersAttr, ok := resp.Schema.Attributes["headers"]
	require.True(t, ok)
	assert.False(t, headersAttr.IsRequired())
}

func TestGraphqlProvider_Resources(t *testing.T) {
	p := &GraphqlProvider{}
	resources := p.Resources(context.Background())

	assert.Len(t, resources, 1)

	// Test that the resource factory returns a valid resource
	resource := resources[0]()
	require.NotNil(t, resource)
}

func TestGraphqlProvider_DataSources(t *testing.T) {
	p := &GraphqlProvider{}
	datasources := p.DataSources(context.Background())

	assert.Len(t, datasources, 1)

	// Test that the datasource factory returns a valid datasource
	datasource := datasources[0]()
	require.NotNil(t, datasource)
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		version string
	}{
		{
			name:    "test version",
			version: "test",
		},
		{
			name:    "dev version",
			version: "dev",
		},
		{
			name:    "release version",
			version: "v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory := New(tt.version)
			require.NotNil(t, factory)

			provider := factory()
			require.NotNil(t, provider)

			// Test that it's the correct type
			_, ok := provider.(*GraphqlProvider)
			assert.True(t, ok)
		})
	}
}

func TestNewGraphqlMutationResource(t *testing.T) {
	resource := NewGraphqlMutationResource()
	require.NotNil(t, resource)
}

func TestNewGraphqlQueryDataSource(t *testing.T) {
	datasource := NewGraphqlQueryDataSource()
	require.NotNil(t, datasource)
}

// Test helper functions
func TestDiagnosticsToString(t *testing.T) {
	tests := []struct {
		name     string
		diags    diag.Diagnostics
		expected string
	}{
		{
			name:     "empty diagnostics",
			diags:    diag.Diagnostics{},
			expected: "",
		},
		{
			name: "single error",
			diags: diag.Diagnostics{
				diag.NewErrorDiagnostic("Test", "Test error"),
			},
			expected: "Test error",
		},
		{
			name: "multiple errors",
			diags: diag.Diagnostics{
				diag.NewErrorDiagnostic("Error1", "First error"),
				diag.NewErrorDiagnostic("Error2", "Second error"),
			},
			expected: "First error; Second error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := utils.DiagnosticsToString(tt.diags)
			assert.Equal(t, tt.expected, result)
		})
	}
}
