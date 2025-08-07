package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/kalenarndt/terraform-provider-graphql/internal/utils"
	"github.com/tidwall/gjson"
)

// Ensure the implementation satisfies the expected interfaces
var (
	_ provider.Provider = &GraphqlProvider{}
)

// GraphqlProvider is the provider implementation.
type GraphqlProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// GraphqlProviderModel describes the provider data model
type GraphqlProviderModel struct {
	URL                            types.String `tfsdk:"url"`
	Headers                        types.Map    `tfsdk:"headers"`
	OAuth2LoginQuery               types.String `tfsdk:"oauth2_login_query"`
	OAuth2LoginQueryVariables      types.Map    `tfsdk:"oauth2_login_query_variables"`
	OAuth2LoginQueryValueAttribute types.String `tfsdk:"oauth2_login_query_value_attribute"`
	// REST OAuth2 support
	OAuth2RestURL          types.String `tfsdk:"oauth2_rest_url"`
	OAuth2RestMethod       types.String `tfsdk:"oauth2_rest_method"`
	OAuth2RestHeaders      types.Map    `tfsdk:"oauth2_rest_headers"`
	OAuth2RestBody         types.String `tfsdk:"oauth2_rest_body"`
	OAuth2RestTokenPath    types.String `tfsdk:"oauth2_rest_token_path"`
	QueryRateLimitDelay    types.String `tfsdk:"query_rate_limit_delay"`
	MutationRateLimitDelay types.String `tfsdk:"mutation_rate_limit_delay"`
}

// Metadata returns the provider type name.
func (p *GraphqlProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "graphql"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration data.
func (p *GraphqlProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = providerschema.Schema{
		Description: "Interact with GraphQL APIs.",
		Attributes: map[string]providerschema.Attribute{
			"url": providerschema.StringAttribute{
				Required:    true,
				Description: "The URL of the GraphQL server.",
			},
			"headers": providerschema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "Additional headers to send with requests.",
			},
			"oauth2_login_query": providerschema.StringAttribute{
				Optional:    true,
				Description: "GraphQL query for OAuth2 login.",
			},
			"oauth2_login_query_variables": providerschema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "Variables for the OAuth2 login query.",
			},
			"oauth2_login_query_value_attribute": providerschema.StringAttribute{
				Optional:    true,
				Description: "Attribute path to extract the token from the OAuth2 login response.",
			},
			"oauth2_rest_url": providerschema.StringAttribute{
				Optional:    true,
				Description: "REST URL for OAuth2 token endpoint (alternative to GraphQL OAuth2).",
			},
			"oauth2_rest_method": providerschema.StringAttribute{
				Optional:    true,
				Description: "HTTP method for REST OAuth2 request (default: POST).",
			},
			"oauth2_rest_headers": providerschema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "Headers for REST OAuth2 request.",
			},
			"oauth2_rest_body": providerschema.StringAttribute{
				Optional:    true,
				Description: "Request body for REST OAuth2 request (e.g., form-encoded or JSON). Supports environment variable substitution: use ${var.wiz_client_id} or $wiz_client_id to reference WIZ_CLIENT_ID environment variable, and ${var.wiz_client_secret} or $wiz_client_secret for WIZ_CLIENT_SECRET.",
			},
			"oauth2_rest_token_path": providerschema.StringAttribute{
				Optional:    true,
				Description: "JSON path to extract token from REST OAuth2 response (e.g., 'access_token').",
			},
			"query_rate_limit_delay": providerschema.StringAttribute{
				Optional:    true,
				Description: "Delay between query requests (e.g., '100ms'). Default: 100ms for queries (10/sec).",
			},
			"mutation_rate_limit_delay": providerschema.StringAttribute{
				Optional:    true,
				Description: "Delay between mutation requests (e.g., '400ms'). Default: 400ms for mutations (3/sec).",
			},
		},
	}
}

// Configure prepares a GraphQL client for data sources and resources.
func (p *GraphqlProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	tflog.Info(ctx, "Configuring GraphQL client")

	var data GraphqlProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Validate required fields
	if data.URL.IsUnknown() || data.URL.IsNull() {
		resp.Diagnostics.AddError(
			"Missing GraphQL URL",
			"The provider cannot create the GraphQL client as there is an unknown or missing configuration value for the GraphQL URL. "+
				"Either target apply the source of the value first, set the value statically in the configuration, or use the GRAPHQL_URL environment variable.",
		)
		return
	}

	config := &graphqlProviderConfig{
		GQLServerUrl:   data.URL.ValueString(),
		RequestHeaders: make(map[string]interface{}),
	}

	// Convert headers from types.Map to map[string]interface{}
	if !data.Headers.IsNull() && !data.Headers.IsUnknown() {
		elements := make(map[string]types.String)
		resp.Diagnostics.Append(data.Headers.ElementsAs(ctx, &elements, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		for k, v := range elements {
			config.RequestHeaders[k] = v.ValueString()
		}
	}

	// Handle OAuth2 configuration
	if !data.OAuth2LoginQuery.IsNull() && !data.OAuth2LoginQuery.IsUnknown() {
		if data.OAuth2LoginQueryVariables.IsNull() || data.OAuth2LoginQueryVariables.IsUnknown() ||
			data.OAuth2LoginQueryValueAttribute.IsNull() || data.OAuth2LoginQueryValueAttribute.IsUnknown() {
			resp.Diagnostics.AddError(
				"Incomplete OAuth 2.0 provider configuration",
				"All three attributes must be set: `oauth2_login_query`, `oauth2_login_query_variables` and `oauth2_login_query_value_attribute`.",
			)
			return
		}

		// Perform GraphQL OAuth2 login
		token, diags := p.performOAuth2Login(ctx, config, data)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
		config.RequestAuthorizationHeaders = map[string]interface{}{
			"Authorization": "Bearer " + token,
		}
	} else if !data.OAuth2RestURL.IsNull() && !data.OAuth2RestURL.IsUnknown() {
		// Handle REST OAuth2 configuration
		if data.OAuth2RestTokenPath.IsNull() || data.OAuth2RestTokenPath.IsUnknown() {
			resp.Diagnostics.AddError(
				"Incomplete REST OAuth2 configuration",
				"Both `oauth2_rest_url` and `oauth2_rest_token_path` must be set for REST OAuth2.",
			)
			return
		}

		// Perform REST OAuth2 login
		token, diags := p.performRestOAuth2Login(ctx, data)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}
		config.RequestAuthorizationHeaders = map[string]interface{}{
			"Authorization": "Bearer " + token,
		}
	}

	// Handle rate limit delay
	if !data.QueryRateLimitDelay.IsNull() && !data.QueryRateLimitDelay.IsUnknown() {
		delay, err := time.ParseDuration(data.QueryRateLimitDelay.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid Query Rate Limit Delay", fmt.Sprintf("failed to parse query_rate_limit_delay: %v", err))
			return
		}
		config.QueryRateLimitDelay = delay
	} else {
		// Default to 100ms for queries
		config.QueryRateLimitDelay = 100 * time.Millisecond
	}

	if !data.MutationRateLimitDelay.IsNull() && !data.MutationRateLimitDelay.IsUnknown() {
		delay, err := time.ParseDuration(data.MutationRateLimitDelay.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid Mutation Rate Limit Delay", fmt.Sprintf("failed to parse mutation_rate_limit_delay: %v", err))
			return
		}
		config.MutationRateLimitDelay = delay
	} else {
		// Default to 400ms for mutations
		config.MutationRateLimitDelay = 400 * time.Millisecond
	}

	// Make the GraphQL client available during DataSource and Resource
	// type Configure methods.
	resp.DataSourceData = config
	resp.ResourceData = config

	tflog.Info(ctx, "Configured GraphQL client", map[string]any{"success": true})
}

// performOAuth2Login performs OAuth2 login and returns the access token.
func (p *GraphqlProvider) performOAuth2Login(ctx context.Context, config *graphqlProviderConfig, data GraphqlProviderModel) (string, diag.Diagnostics) {
	var diags diag.Diagnostics

	tflog.Debug(ctx, "Performing OAuth2 login")

	// Convert variables to JSON string
	var variablesJSON string
	if !data.OAuth2LoginQueryVariables.IsNull() && !data.OAuth2LoginQueryVariables.IsUnknown() {
		elements := make(map[string]types.String)
		diags.Append(data.OAuth2LoginQueryVariables.ElementsAs(ctx, &elements, false)...)
		if diags.HasError() {
			return "", diags
		}

		variablesMap := make(map[string]interface{})
		for k, v := range elements {
			variablesMap[k] = v.ValueString()
		}

		variablesBytes, err := json.Marshal(variablesMap)
		if err != nil {
			diags.AddError("OAuth2 Variable Marshaling Error", fmt.Sprintf("failed to marshal oauth2_login_query_variables: %v", err))
			return "", diags
		}
		variablesJSON = string(variablesBytes)
	}

	// Execute login query
	queryResponse, resBytes, diags := queryExecuteFramework(ctx, config, data.OAuth2LoginQuery.ValueString(), variablesJSON, false)
	if diags.HasError() {
		return "", diags
	}

	if len(queryResponse.Errors) > 0 {
		for _, gqlErr := range queryResponse.Errors {
			diags.AddError("OAuth2 Login Error", gqlErr.Message)
		}
		return "", diags
	}

	// Extract token
	tokenMap, err := computeMutationVariableKeys(
		map[string]interface{}{"token": data.OAuth2LoginQueryValueAttribute.ValueString()},
		string(resBytes),
	)
	if err != nil {
		diags.AddError("OAuth2 Token Extraction Error", err.Error())
		return "", diags
	}

	token, ok := tokenMap["token"]
	if !ok {
		diags.AddError("OAuth2 Token Not Found", "Could not extract token from response using the provided attribute path.")
		return "", diags
	}

	tflog.Debug(ctx, "OAuth2 login successful")
	return token, diags
}

// performRestOAuth2Login performs REST OAuth2 login and returns the access token.
func (p *GraphqlProvider) performRestOAuth2Login(ctx context.Context, data GraphqlProviderModel) (string, diag.Diagnostics) {
	var diags diag.Diagnostics

	tflog.Debug(ctx, "Performing REST OAuth2 login")

	// Determine HTTP method
	method := "POST"
	if !data.OAuth2RestMethod.IsNull() && !data.OAuth2RestMethod.IsUnknown() {
		method = data.OAuth2RestMethod.ValueString()
	}

	// Get request body, potentially enhanced with environment variables
	body := data.OAuth2RestBody.ValueString()

	// Check for environment variables and enhance the body if needed
	if strings.Contains(body, "${var.wiz_client_id}") || strings.Contains(body, "$wiz_client_id") {
		if envClientId := os.Getenv("WIZ_CLIENT_ID"); envClientId != "" {
			body = strings.ReplaceAll(body, "${var.wiz_client_id}", envClientId)
			body = strings.ReplaceAll(body, "$wiz_client_id", envClientId)
			tflog.Debug(ctx, "Using WIZ_CLIENT_ID from environment variable")
		} else {
			diags.AddWarning("Missing Environment Variable", "WIZ_CLIENT_ID environment variable not found, using value from configuration")
		}
	}

	if strings.Contains(body, "${var.wiz_client_secret}") || strings.Contains(body, "$wiz_client_secret") {
		if envClientSecret := os.Getenv("WIZ_CLIENT_SECRET"); envClientSecret != "" {
			body = strings.ReplaceAll(body, "${var.wiz_client_secret}", envClientSecret)
			body = strings.ReplaceAll(body, "$wiz_client_secret", envClientSecret)
			tflog.Debug(ctx, "Using WIZ_CLIENT_SECRET from environment variable")
		} else {
			diags.AddWarning("Missing Environment Variable", "WIZ_CLIENT_SECRET environment variable not found, using value from configuration")
		}
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, data.OAuth2RestURL.ValueString(), strings.NewReader(body))
	if err != nil {
		diags.AddError("REST OAuth2 Request Creation Error", fmt.Sprintf("failed to create request: %v", err))
		return "", diags
	}

	// Set default headers if not provided
	if data.OAuth2RestHeaders.IsNull() || data.OAuth2RestHeaders.IsUnknown() {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
	} else {
		// Set custom headers
		elements := make(map[string]types.String)
		diags.Append(data.OAuth2RestHeaders.ElementsAs(ctx, &elements, false)...)
		if diags.HasError() {
			return "", diags
		}
		for k, v := range elements {
			req.Header.Set(k, v.ValueString())
		}
	}

	// Execute request
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		diags.AddError("REST OAuth2 Request Error", fmt.Sprintf("failed to execute request: %v", err))
		return "", diags
	}
	defer resp.Body.Close()

	// Read response
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		diags.AddError("REST OAuth2 Response Error", fmt.Sprintf("failed to read response: %v", err))
		return "", diags
	}

	if resp.StatusCode != http.StatusOK {
		diags.AddError("REST OAuth2 HTTP Error", fmt.Sprintf("received HTTP %d: %s", resp.StatusCode, string(bodyBytes)))
		return "", diags
	}

	// Extract token using JSON path
	result := gjson.Get(string(bodyBytes), data.OAuth2RestTokenPath.ValueString())
	if !result.Exists() {
		diags.AddError("REST OAuth2 Token Extraction Error", fmt.Sprintf("token path '%s' not found in response", data.OAuth2RestTokenPath.ValueString()))
		return "", diags
	}

	token := result.String()
	if token == "" {
		diags.AddError("REST OAuth2 Token Error", "extracted token is empty")
		return "", diags
	}

	tflog.Debug(ctx, "REST OAuth2 login successful")
	return token, diags
}

// Resources defines the resources implemented in the provider.
func (p *GraphqlProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewGraphqlMutationResource,
	}
}

// DataSources defines the data sources implemented in the provider.
func (p *GraphqlProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewGraphqlQueryDataSource,
	}
}

// New is a helper function to simplify provider server and testing implementation.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &GraphqlProvider{
			version: version,
		}
	}
}

// GraphqlQueryDataSource represents the GraphQL query data source
type GraphqlQueryDataSource struct {
	config *graphqlProviderConfig
}

// Metadata returns the data source type name.
func (d *GraphqlQueryDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_query"
}

// Schema defines the schema for the data source.
func (d *GraphqlQueryDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = datasourceschema.Schema{
		Description: "A GraphQL query data source that can execute GraphQL queries.",
		Attributes: map[string]datasourceschema.Attribute{
			"query": datasourceschema.StringAttribute{
				Required:    true,
				Description: "The GraphQL query to execute.",
			},
			"query_variables": datasourceschema.DynamicAttribute{
				Optional:    true,
				Description: "Variables for the GraphQL query. Can be any valid JSON value (object, array, string, number, boolean, null).",
			},
			"query_response": datasourceschema.StringAttribute{
				Computed:    true,
				Description: "The raw body of the HTTP response from the last read of the object.",
			},
			"paginated": datasourceschema.BoolAttribute{
				Optional:    true,
				Description: "Whether the query is paginated.",
			},
			"id": datasourceschema.StringAttribute{
				Computed:    true,
				Description: "The ID of the data source result.",
			},
		},
	}
}

// Configure adds the provider configured client to the data source.
func (d *GraphqlQueryDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*graphqlProviderConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *graphqlProviderConfig, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.config = config
}

// Read refreshes the Terraform state with the latest data.
func (d *GraphqlQueryDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Debug(ctx, "Preparing to read GraphQL query data source")

	var data GraphqlQueryDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Convert query variables to JSON string
	var variablesJSON string
	if !data.QueryVariables.IsNull() && !data.QueryVariables.IsUnknown() {
		variablesJSON, resp.Diagnostics = utils.DynamicToJSONString(ctx, data.QueryVariables)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	usePagination := data.Paginated.ValueBool()

	queryResponse, resBytes, diags := queryExecuteFramework(ctx, d.config, data.Query.ValueString(), variablesJSON, usePagination)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	if len(queryResponse.Errors) > 0 {
		for _, gqlErr := range queryResponse.Errors {
			resp.Diagnostics.AddError("GraphQL Server Error", gqlErr.Message)
		}
		return
	}

	data.QueryResponse = types.StringValue(string(resBytes))
	data.ID = types.StringValue(fmt.Sprintf("%d", hash(resBytes)))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	tflog.Debug(ctx, "Finished reading GraphQL query data source", map[string]any{"success": true})
}

// NewGraphqlMutationResource creates a new GraphQL mutation resource
func NewGraphqlMutationResource() resource.Resource {
	return &GraphqlMutationResource{}
}

// NewGraphqlQueryDataSource creates a new GraphQL query data source
func NewGraphqlQueryDataSource() datasource.DataSource {
	return &GraphqlQueryDataSource{
		config: nil, // Will be set in Configure
	}
}

// GraphqlQueryDataSourceModel describes the data source data model
type GraphqlQueryDataSourceModel struct {
	Query          types.String  `tfsdk:"query"`
	QueryVariables types.Dynamic `tfsdk:"query_variables"`
	QueryResponse  types.String  `tfsdk:"query_response"`
	Paginated      types.Bool    `tfsdk:"paginated"`
	ID             types.String  `tfsdk:"id"`
}

// graphqlProviderConfig holds the provider configuration
type graphqlProviderConfig struct {
	GQLServerUrl                string
	RequestHeaders              map[string]interface{}
	RequestAuthorizationHeaders map[string]interface{}
	QueryRateLimitDelay         time.Duration
	MutationRateLimitDelay      time.Duration
}
