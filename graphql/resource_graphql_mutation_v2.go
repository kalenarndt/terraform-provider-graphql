package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/kalenarndt/terraform-provider-graphql/internal/utils"
)

// GraphqlMutationV2Resource represents the GraphQL mutation v2 resource
type GraphqlMutationV2Resource struct {
	config *graphqlProviderConfig
}

// GraphqlMutationV2ResourceModel describes the resource data model
type GraphqlMutationV2ResourceModel struct {
	ID                            types.String  `tfsdk:"id"`
	Name                          types.String  `tfsdk:"name"`
	CreateMutationFile            types.String  `tfsdk:"create_mutation_file"`
	UpdateMutationFile            types.String  `tfsdk:"update_mutation_file"`
	DeleteMutationFile            types.String  `tfsdk:"delete_mutation_file"`
	ReadQueryFile                 types.String  `tfsdk:"read_query_file"`
	CreateVariables               types.Dynamic `tfsdk:"create_variables"`
	UpdateVariables               types.Dynamic `tfsdk:"update_variables"`
	DeleteVariables               types.Dynamic `tfsdk:"delete_variables"`
	ReadVariables                 types.Dynamic `tfsdk:"read_variables"`
	ComputeKeys                   types.Map     `tfsdk:"compute_keys"`
	ReadComputeKeys               types.Map     `tfsdk:"read_compute_keys"`
	ComputedValues                types.Map     `tfsdk:"computed_values"`
	CreateOnlyFields              types.List    `tfsdk:"create_only_fields"`
	ForceReplace                  types.Bool    `tfsdk:"force_replace"`
	EnableRemoteStateVerification types.Bool    `tfsdk:"enable_remote_state_verification"`
	Response                      types.String  `tfsdk:"response"`
	CurrentRemoteState            types.String  `tfsdk:"current_remote_state"`
	LastAppliedVariables          types.String  `tfsdk:"last_applied_variables"`
}

// Metadata returns the resource type name.
func (r *GraphqlMutationV2Resource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mutation_v2"
}

// Schema defines the schema for the resource.
func (r *GraphqlMutationV2Resource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A GraphQL mutation v2 resource that stores queries in files and tracks variables in state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "A unique name for this resource instance.",
			},
			"create_mutation_file": schema.StringAttribute{
				Required:    true,
				Description: "Path to the file containing the GraphQL mutation to create the resource.",
			},
			"update_mutation_file": schema.StringAttribute{
				Optional:    true,
				Description: "Path to the file containing the GraphQL mutation to update the resource.",
			},
			"delete_mutation_file": schema.StringAttribute{
				Required:    true,
				Description: "Path to the file containing the GraphQL mutation to delete the resource.",
			},
			"read_query_file": schema.StringAttribute{
				Optional:    true,
				Description: "Path to the file containing the GraphQL query to read the resource.",
			},
			"create_variables": schema.DynamicAttribute{
				Required:    true,
				Description: "Variables for the create mutation.",
			},
			"update_variables": schema.DynamicAttribute{
				Optional:    true,
				Description: "Variables for the update mutation.",
			},
			"delete_variables": schema.DynamicAttribute{
				Optional:    true,
				Description: "Variables for the delete mutation.",
			},
			"read_variables": schema.DynamicAttribute{
				Optional:    true,
				Description: "Variables for the read query.",
			},
			"compute_keys": schema.MapAttribute{
				ElementType: types.StringType,
				Required:    true,
				Description: "Map of computed keys from the create mutation response.",
			},
			"read_compute_keys": schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "Map of computed keys from the read query response.",
			},
			"computed_values": schema.MapAttribute{
				ElementType: types.StringType,
				Computed:    true,
				Description: "Computed values from the GraphQL response.",
			},
			"create_only_fields": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "List of fields that should only be set during creation.",
			},
			"force_replace": schema.BoolAttribute{
				Optional:    true,
				Description: "Force replacement of the resource.",
			},
			"enable_remote_state_verification": schema.BoolAttribute{
				Optional:    true,
				Description: "Enable verification of remote state.",
			},
			"response": schema.StringAttribute{
				Computed:    true,
				Description: "The JSON response from the GraphQL operation.",
			},
			"current_remote_state": schema.StringAttribute{
				Computed:    true,
				Description: "The current remote state of the resource.",
			},
			"last_applied_variables": schema.StringAttribute{
				Computed:    true,
				Description: "The last applied variables for tracking changes.",
			},
		},
	}
}

// Configure configures the resource.
func (r *GraphqlMutationV2Resource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*graphqlProviderConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *graphqlProviderConfig, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.config = config
}

// Create creates the resource.
func (r *GraphqlMutationV2Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan GraphqlMutationV2ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read the create mutation from file
	createMutation, err := r.readQueryFile(plan.CreateMutationFile.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read create mutation file", err.Error())
		return
	}

	// Execute the create mutation
	createVarsJSON, diags := utils.DynamicToJSONString(ctx, plan.CreateVariables)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	var createVars map[string]interface{}
	if err := json.Unmarshal([]byte(createVarsJSON), &createVars); err != nil {
		resp.Diagnostics.AddError("Failed to parse create variables", err.Error())
		return
	}

	response, err := r.executeMutation(ctx, createMutation, createVars)
	if err != nil {
		resp.Diagnostics.AddError("Failed to execute create mutation", err.Error())
		return
	}

	// Extract computed values
	computedValues, err := r.extractComputedValues(response, plan.ComputeKeys)
	if err != nil {
		resp.Diagnostics.AddError("Failed to extract computed values", err.Error())
		return
	}

	// Set the ID and computed values
	computedValuesMap := make(map[string]string)
	if !computedValues.IsNull() {
		computedValues.ElementsAs(ctx, &computedValuesMap, false)
	}
	if id, exists := computedValuesMap["id"]; exists {
		plan.ID = types.StringValue(id)
	}
	plan.ComputedValues = computedValues
	plan.Response = types.StringValue(response)
	plan.CurrentRemoteState = types.StringValue(response)

	// Store the last applied variables
	lastAppliedVars, _ := json.Marshal(createVars)
	plan.LastAppliedVariables = types.StringValue(string(lastAppliedVars))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Read reads the resource.
func (r *GraphqlMutationV2Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state GraphqlMutationV2ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If no read query file is provided, skip the read operation
	if state.ReadQueryFile.IsNull() || state.ReadQueryFile.IsUnknown() {
		// Just return the current state without trying to read from the API
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}

	// Read the read query from file
	readQuery, err := r.readQueryFile(state.ReadQueryFile.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read query file", err.Error())
		return
	}

	// Build read variables using computed values
	readVars, err := r.buildReadVariables(ctx, &state)
	if err != nil {
		resp.Diagnostics.AddError("Failed to build read variables", err.Error())
		return
	}

	// Execute the read query
	response, err := r.executeQuery(ctx, readQuery, readVars)
	if err != nil {
		resp.Diagnostics.AddError("Failed to execute read query", err.Error())
		return
	}

	// Update the state
	state.Response = types.StringValue(response)
	state.CurrentRemoteState = types.StringValue(response)

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update updates the resource.
func (r *GraphqlMutationV2Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state GraphqlMutationV2ResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Check if we need to force replace
	if plan.ForceReplace.ValueBool() {
		// Delete the existing resource
		if err := r.deleteResource(ctx, &state); err != nil {
			resp.Diagnostics.AddError("Failed to delete resource for replacement", err.Error())
			return
		}

		// Create the new resource by calling the create logic directly
		// Read the create mutation from file
		createMutation, err := r.readQueryFile(plan.CreateMutationFile.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Failed to read create mutation file", err.Error())
			return
		}

		// Execute the create mutation
		createVarsJSON, diags := utils.DynamicToJSONString(ctx, plan.CreateVariables)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}

		var createVars map[string]interface{}
		if err := json.Unmarshal([]byte(createVarsJSON), &createVars); err != nil {
			resp.Diagnostics.AddError("Failed to parse create variables", err.Error())
			return
		}

		response, err := r.executeMutation(ctx, createMutation, createVars)
		if err != nil {
			resp.Diagnostics.AddError("Failed to execute create mutation", err.Error())
			return
		}

		// Extract computed values
		computedValues, err := r.extractComputedValues(response, plan.ComputeKeys)
		if err != nil {
			resp.Diagnostics.AddError("Failed to extract computed values", err.Error())
			return
		}

		// Set the ID and computed values
		computedValuesMap := make(map[string]string)
		if !computedValues.IsNull() {
			computedValues.ElementsAs(ctx, &computedValuesMap, false)
		}
		if id, exists := computedValuesMap["id"]; exists {
			plan.ID = types.StringValue(id)
		}
		plan.ComputedValues = computedValues
		plan.Response = types.StringValue(response)
		plan.CurrentRemoteState = types.StringValue(response)

		// Store the last applied variables
		lastAppliedVars, _ := json.Marshal(createVars)
		plan.LastAppliedVariables = types.StringValue(string(lastAppliedVars))

		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	// Read the update mutation from file
	updateMutation, err := r.readQueryFile(plan.UpdateMutationFile.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read update mutation file", err.Error())
		return
	}

	// Build update variables
	updateVars, err := r.buildUpdateVariables(ctx, &plan, &state)
	if err != nil {
		resp.Diagnostics.AddError("Failed to build update variables", err.Error())
		return
	}

	// Execute the update mutation
	response, err := r.executeMutation(ctx, updateMutation, updateVars)
	if err != nil {
		resp.Diagnostics.AddError("Failed to execute update mutation", err.Error())
		return
	}

	// Extract computed values from update response
	// For update responses, we need to use different paths than create responses
	updateComputeKeys := types.MapValueMust(types.StringType, map[string]attr.Value{
		"id":   types.StringValue("data.updateConnector.connector.id"),
		"name": types.StringValue("data.updateConnector.connector.name"),
	})

	computedValues, err := r.extractComputedValues(response, updateComputeKeys)
	if err != nil {
		resp.Diagnostics.AddError("Failed to extract computed values from update response", err.Error())
		return
	}

	// If computed values are empty, try to extract from the current response using create paths
	if computedValues.IsNull() || len(computedValues.Elements()) == 0 {
		computedValues, err = r.extractComputedValues(response, plan.ComputeKeys)
		if err != nil {
			resp.Diagnostics.AddError("Failed to extract computed values using create paths", err.Error())
			return
		}
	}

	// Update the state
	plan.Response = types.StringValue(response)
	plan.CurrentRemoteState = types.StringValue(response)
	plan.ComputedValues = computedValues

	// Store the last applied variables
	lastAppliedVars, _ := json.Marshal(updateVars)
	plan.LastAppliedVariables = types.StringValue(string(lastAppliedVars))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete deletes the resource.
func (r *GraphqlMutationV2Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state GraphqlMutationV2ResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.deleteResource(ctx, &state); err != nil {
		resp.Diagnostics.AddError("Failed to delete resource", err.Error())
		return
	}
}

// Helper methods

func (r *GraphqlMutationV2Resource) readQueryFile(filePath string) (string, error) {
	// Resolve relative paths relative to the Terraform configuration
	if !filepath.IsAbs(filePath) {
		// For now, assume the file is relative to the current working directory
		// In a real implementation, you might want to get the Terraform configuration directory
		absPath, err := filepath.Abs(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to resolve file path: %w", err)
		}
		filePath = absPath
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	return string(content), nil
}

func (r *GraphqlMutationV2Resource) executeMutation(ctx context.Context, mutation string, variables map[string]interface{}) (string, error) {
	// Convert variables to JSON string
	variablesJSON, err := json.Marshal(variables)
	if err != nil {
		return "", fmt.Errorf("failed to marshal variables: %w", err)
	}

	// Use the existing query execution framework
	queryResponse, resBytes, diags := queryExecuteFramework(ctx, r.config, mutation, string(variablesJSON), false)
	if diags.HasError() {
		return "", fmt.Errorf("query execution failed: %v", diags)
	}

	if len(queryResponse.Errors) > 0 {
		for _, gqlErr := range queryResponse.Errors {
			return "", fmt.Errorf("GraphQL error: %s", gqlErr.Message)
		}
	}

	return string(resBytes), nil
}

func (r *GraphqlMutationV2Resource) executeQuery(ctx context.Context, query string, variables map[string]interface{}) (string, error) {
	// Convert variables to JSON string
	variablesJSON, err := json.Marshal(variables)
	if err != nil {
		return "", fmt.Errorf("failed to marshal variables: %w", err)
	}

	// Use the existing query execution framework
	queryResponse, resBytes, diags := queryExecuteFramework(ctx, r.config, query, string(variablesJSON), false)
	if diags.HasError() {
		return "", fmt.Errorf("query execution failed: %v", diags)
	}

	if len(queryResponse.Errors) > 0 {
		for _, gqlErr := range queryResponse.Errors {
			return "", fmt.Errorf("GraphQL error: %s", gqlErr.Message)
		}
	}

	return string(resBytes), nil
}

func (r *GraphqlMutationV2Resource) extractComputedValues(response string, computeKeys types.Map) (types.Map, error) {
	var responseData map[string]interface{}
	if err := json.Unmarshal([]byte(response), &responseData); err != nil {
		return types.MapNull(types.StringType), fmt.Errorf("failed to unmarshal response: %w", err)
	}

	computeKeysMap := make(map[string]string)
	if !computeKeys.IsNull() {
		computeKeys.ElementsAs(context.Background(), &computeKeysMap, false)
	}

	computedValues := make(map[string]attr.Value)
	for key, path := range computeKeysMap {
		value, err := r.extractValueFromPath(responseData, path)
		if err != nil {
			return types.MapNull(types.StringType), fmt.Errorf("failed to extract value for key %s: %w", key, err)
		}
		computedValues[key] = types.StringValue(fmt.Sprintf("%v", value))
	}

	return types.MapValueMust(types.StringType, computedValues), nil
}

func (r *GraphqlMutationV2Resource) extractValueFromPath(data map[string]interface{}, path string) (interface{}, error) {
	// Simple path extraction - split by dots and traverse
	parts := strings.Split(path, ".")
	var current interface{} = data

	for _, part := range parts {
		if current == nil {
			return nil, fmt.Errorf("path %s not found", path)
		}

		if mapData, ok := current.(map[string]interface{}); ok {
			if val, exists := mapData[part]; exists {
				current = val
			} else {
				return nil, fmt.Errorf("path %s not found", path)
			}
		} else {
			return nil, fmt.Errorf("path %s not found", path)
		}
	}

	return current, nil
}

func (r *GraphqlMutationV2Resource) buildReadVariables(ctx context.Context, state *GraphqlMutationV2ResourceModel) (map[string]interface{}, error) {
	// Use the computed values to build read variables
	computedValuesMap := make(map[string]string)
	if !state.ComputedValues.IsNull() {
		state.ComputedValues.ElementsAs(ctx, &computedValuesMap, false)
	}

	// Debug: Print computed values
	fmt.Printf("DEBUG: Computed values: %+v\n", computedValuesMap)

	// If read variables are provided, use them as a template
	var readVars map[string]interface{}
	if !state.ReadVariables.IsNull() {
		readVarsJSON, _ := utils.DynamicToJSONString(ctx, state.ReadVariables)
		fmt.Printf("DEBUG: Read variables JSON: %s\n", readVarsJSON)
		json.Unmarshal([]byte(readVarsJSON), &readVars)
	} else {
		readVars = make(map[string]interface{})
	}

	// Replace any "self.id" references with actual computed values
	readVars = r.replaceSelfReferences(readVars, computedValuesMap)

	// If computed values are empty and we have a state ID, use it directly
	if len(computedValuesMap) == 0 && !state.ID.IsNull() && !state.ID.IsUnknown() {
		// Replace any remaining "self.id" references with the state ID
		readVars = r.replaceSelfReferences(readVars, map[string]string{
			"id": state.ID.ValueString(),
		})
	}

	// For read queries, check if the query expects an input parameter
	// Only extract from input wrapper if the query doesn't expect input
	readQuery, err := r.readQueryFile(state.ReadQueryFile.ValueString())
	if err == nil {
		// Simple check: if query contains "$input:" it expects input parameter
		if !strings.Contains(readQuery, "$input:") {
			// Query doesn't expect input, so extract from input wrapper if present
			if inputObj, ok := readVars["input"]; ok {
				if inputMap, ok := inputObj.(map[string]interface{}); ok {
					readVars = inputMap
				}
			}
		}
		// If query contains "$input:", keep the input wrapper as-is
	}

	// Debug: Print final read variables
	fmt.Printf("DEBUG: Final read variables: %+v\n", readVars)

	// Debug: Print the actual variables being sent to GraphQL
	variablesJSON, _ := json.Marshal(readVars)
	fmt.Printf("DEBUG: Variables JSON being sent: %s\n", string(variablesJSON))

	return readVars, nil
}

func (r *GraphqlMutationV2Resource) buildUpdateVariables(ctx context.Context, plan, state *GraphqlMutationV2ResourceModel) (map[string]interface{}, error) {
	// Get the current variables from the plan
	updateVarsJSON, diags := utils.DynamicToJSONString(ctx, plan.UpdateVariables)
	if diags.HasError() {
		return nil, fmt.Errorf("failed to convert update variables: %v", diags)
	}

	var updateVars map[string]interface{}
	if err := json.Unmarshal([]byte(updateVarsJSON), &updateVars); err != nil {
		return nil, fmt.Errorf("failed to parse update variables: %w", err)
	}

	// Get computed values from state
	computedValuesMap := make(map[string]string)
	if !state.ComputedValues.IsNull() {
		state.ComputedValues.ElementsAs(ctx, &computedValuesMap, false)
	}

	// Replace any "self.id" references with actual computed values
	updateVars = r.replaceSelfReferences(updateVars, computedValuesMap)

	return updateVars, nil
}

func (r *GraphqlMutationV2Resource) replaceSelfReferences(variables map[string]interface{}, computedValues map[string]string) map[string]interface{} {
	result := make(map[string]interface{})

	for key, value := range variables {
		switch v := value.(type) {
		case string:
			if strings.HasPrefix(v, "self.") {
				field := strings.TrimPrefix(v, "self.")
				if computedValue, exists := computedValues[field]; exists {
					result[key] = computedValue
				} else {
					result[key] = v // Keep original if not found
				}
			} else {
				result[key] = v
			}
		case map[string]interface{}:
			result[key] = r.replaceSelfReferences(v, computedValues)
		default:
			result[key] = v
		}
	}

	return result
}

func (r *GraphqlMutationV2Resource) deleteResource(ctx context.Context, state *GraphqlMutationV2ResourceModel) error {
	// Read the delete mutation from file
	deleteMutation, err := r.readQueryFile(state.DeleteMutationFile.ValueString())
	if err != nil {
		return fmt.Errorf("failed to read delete mutation file: %w", err)
	}

	// Build delete variables
	deleteVarsJSON, diags := utils.DynamicToJSONString(ctx, state.DeleteVariables)
	if diags.HasError() {
		return fmt.Errorf("failed to convert delete variables: %v", diags)
	}

	var deleteVars map[string]interface{}
	if err := json.Unmarshal([]byte(deleteVarsJSON), &deleteVars); err != nil {
		return fmt.Errorf("failed to parse delete variables: %w", err)
	}

	// Replace any "self.id" references
	computedValuesMap := make(map[string]string)
	if !state.ComputedValues.IsNull() {
		state.ComputedValues.ElementsAs(ctx, &computedValuesMap, false)
	}
	deleteVars = r.replaceSelfReferences(deleteVars, computedValuesMap)

	// Execute the delete mutation
	_, err = r.executeMutation(ctx, deleteMutation, deleteVars)
	return err
}

// NewGraphqlMutationV2Resource creates a new instance of the resource
func NewGraphqlMutationV2Resource() resource.Resource {
	return &GraphqlMutationV2Resource{}
}
