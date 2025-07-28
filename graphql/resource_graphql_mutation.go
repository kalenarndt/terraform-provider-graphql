package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// GraphqlMutationResource represents the GraphQL mutation resource
type GraphqlMutationResource struct {
	config *graphqlProviderConfig
}

// GraphqlMutationResourceModel describes the resource data model
type GraphqlMutationResourceModel struct {
	ReadQuery               types.String `tfsdk:"read_query"`
	CreateMutation          types.String `tfsdk:"create_mutation"`
	DeleteMutation          types.String `tfsdk:"delete_mutation"`
	UpdateMutation          types.String `tfsdk:"update_mutation"`
	MutationVariables       types.String `tfsdk:"mutation_variables"`
	ReadQueryVariables      types.String `tfsdk:"read_query_variables"`
	DeleteMutationVariables types.String `tfsdk:"delete_mutation_variables"`
	ComputeMutationKeys     types.Map    `tfsdk:"compute_mutation_keys"`
	ReadComputeKeys         types.Map    `tfsdk:"read_compute_keys"`
	ComputeFromRead         types.Bool   `tfsdk:"compute_from_read"`
	WrapUpdateInPatch       types.Bool   `tfsdk:"wrap_update_in_patch"`
	CreateOnlyFields        types.List   `tfsdk:"create_only_fields"`
	ComputedValues          types.Map    `tfsdk:"computed_values"`
	ForceReplace            types.Bool   `tfsdk:"force_replace"`
	QueryResponse           types.String `tfsdk:"query_response"`
	ExistingHash            types.String `tfsdk:"existing_hash"`
	Id                      types.String `tfsdk:"id"`
}

// Metadata returns the resource type name.
func (r *GraphqlMutationResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mutation"
}

// Schema defines the schema for the resource.
func (r *GraphqlMutationResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A GraphQL mutation resource that can create, read, update, and delete resources via GraphQL mutations.",
		Attributes: map[string]schema.Attribute{
			"read_query": schema.StringAttribute{
				Required:    true,
				Description: "The GraphQL query to read the resource.",
			},
			"create_mutation": schema.StringAttribute{
				Required:    true,
				Description: "The GraphQL mutation to create the resource.",
			},
			"delete_mutation": schema.StringAttribute{
				Required:    true,
				Description: "The GraphQL mutation to delete the resource.",
			},
			"update_mutation": schema.StringAttribute{
				Required:    true,
				Description: "The GraphQL mutation to update the resource.",
			},
			"mutation_variables": schema.StringAttribute{
				Required:    true,
				Description: "A JSON-encoded string representing the variables for the create and update operations.",
			},
			"read_query_variables": schema.StringAttribute{
				Optional:    true,
				Description: "A JSON-encoded string representing the variables for the read query.",
			},
			"delete_mutation_variables": schema.StringAttribute{
				Optional:    true,
				Description: "A JSON-encoded string representing the variables for the delete mutation.",
			},
			"compute_mutation_keys": schema.MapAttribute{
				ElementType: types.StringType,
				Required:    true,
				Description: "A map of keys to paths for extracting values from the API response. Use JSON path syntax (e.g., 'createTodo.id' or 'data.user.id'). These extracted values become available in computed_values and are used for subsequent operations.",
			},
			"read_compute_keys": schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "A map of keys to paths for extracting values from the read query response. If not provided, defaults to compute_mutation_keys.",
			},
			"compute_from_read": schema.BoolAttribute{
				Optional:    true,
				Description: "If true, the provider will automatically generate compute keys from the read query response, saving the need to define read_compute_keys.",
			},
			"wrap_update_in_patch": schema.BoolAttribute{
				Optional:    true,
				Description: "If true, for update operations, the provider will wrap the changed fields from mutation_variables inside a 'patch' object and inject the computed 'id'.",
			},
			"create_only_fields": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "A list of paths to fields in mutation_variables that should be ignored during update operations.",
			},
			"computed_values": schema.MapAttribute{
				ElementType: types.StringType,
				Computed:    true,
				Description: "A map of values computed from the API response, used to populate variables for subsequent operations.",
			},
			"force_replace": schema.BoolAttribute{
				Optional:    true,
				Description: "If true, all updates will first delete the resource and recreate it.",
			},
			"query_response": schema.StringAttribute{
				Computed:    true,
				Description: "The raw body of the HTTP response from the last read of the object.",
			},
			"existing_hash": schema.StringAttribute{
				Computed:    true,
				Description: "Represents the state of existence of a mutation in order to support intelligent updates.",
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The ID of the resource.",
			},
		},
	}
}

// Configure adds the provider configured client to the resource.
func (r *GraphqlMutationResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// Create creates the resource and sets the initial Terraform state.
func (r *GraphqlMutationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	tflog.Debug(ctx, "Preparing to create GraphQL mutation resource")

	var data GraphqlMutationResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Execute create operation
	_, diags := r.executeCreateHook(ctx, &data, r.config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	// Read the resource to populate computed fields
	diags = r.readResource(ctx, &data, r.config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	// Set state to fully populated data
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	tflog.Debug(ctx, "Created GraphQL mutation resource", map[string]any{"success": true})
}

// Read refreshes the Terraform state with the latest data.
func (r *GraphqlMutationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	tflog.Debug(ctx, "Preparing to read GraphQL mutation resource")

	var data GraphqlMutationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Read the resource
	diags := r.readResource(ctx, &data, r.config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	// Set refreshed state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	tflog.Debug(ctx, "Finished reading GraphQL mutation resource", map[string]any{"success": true})
}

// Update updates the resource and sets the updated Terraform state on success.
func (r *GraphqlMutationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	tflog.Info(ctx, "UPDATE METHOD CALLED - PROVIDER IS WORKING!")

	var data GraphqlMutationResourceModel
	var state GraphqlMutationResourceModel

	// Get the plan data
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Get the previous state to ensure we have the ID
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Ensure the ID is set from the previous state
	if !state.Id.IsNull() && !state.Id.IsUnknown() {
		data.Id = state.Id
	}

	// Check if force replace is enabled
	if data.ForceReplace.ValueBool() {
		// Delete the resource first
		diags := r.executeDeleteHook(ctx, &data, r.config)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}

		// Create the resource again
		_, createDiags := r.executeCreateHook(ctx, &data, r.config)
		if createDiags.HasError() {
			resp.Diagnostics.Append(createDiags...)
			return
		}
	} else {
		// Read the resource first to populate computed values
		diags := r.readResource(ctx, &data, r.config)
		if diags.HasError() {
			resp.Diagnostics.Append(diags...)
			return
		}

		// Prepare update payload after computed values are populated
		if err := r.prepareUpdatePayload(ctx, &data, req); err != nil {
			resp.Diagnostics.AddError("Update Payload Error", err.Error())
			return
		}

		// Execute update operation
		_, updateDiags := r.executeUpdateHook(ctx, &data, r.config)
		if updateDiags.HasError() {
			resp.Diagnostics.Append(updateDiags...)
			return
		}

		// Read the resource again to populate computed fields after update
		readDiags := r.readResource(ctx, &data, r.config)
		if readDiags.HasError() {
			resp.Diagnostics.Append(readDiags...)
			return
		}
	}

	// Set state to fully populated data
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	tflog.Debug(ctx, "Updated GraphQL mutation resource", map[string]any{"success": true})
}

// Delete deletes the resource and removes the Terraform state on success.
func (r *GraphqlMutationResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	tflog.Debug(ctx, "Preparing to delete GraphQL mutation resource")

	var data GraphqlMutationResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Execute delete operation
	diags := r.executeDeleteHook(ctx, &data, r.config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	tflog.Debug(ctx, "Deleted GraphQL mutation resource", map[string]any{"success": true})
}

// ImportState imports the resource and sets the initial Terraform state.
func (r *GraphqlMutationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// For now, we'll use the import ID as the resource ID
	// In a more sophisticated implementation, you might want to parse the import ID
	// and set specific attributes based on the import format
	var data GraphqlMutationResourceModel
	data.Id = types.StringValue(req.ID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Helper methods for CRUD operations
func (r *GraphqlMutationResource) executeCreateHook(ctx context.Context, data *GraphqlMutationResourceModel, config *graphqlProviderConfig) ([]byte, diag.Diagnostics) {
	var diags diag.Diagnostics

	tflog.Debug(ctx, "Executing create hook", map[string]any{
		"createMutation":    data.CreateMutation.ValueString(),
		"mutationVariables": data.MutationVariables.ValueString(),
	})

	// Execute create mutation
	queryResponse, resBytes, diags := r.queryExecuteFramework(ctx, config, data.CreateMutation.ValueString(), data.MutationVariables.ValueString(), true)
	if diags.HasError() {
		return nil, diags
	}

	if len(queryResponse.Errors) > 0 {
		for _, gqlErr := range queryResponse.Errors {
			diags.AddError("GraphQL Server Error", gqlErr.Message)
		}
		return nil, diags
	}

	// Debug: Log the response for troubleshooting
	tflog.Debug(ctx, "Create mutation response", map[string]any{
		"responseLength":  len(resBytes),
		"responsePreview": string(resBytes[:min(1000, len(resBytes))]),
		"hasData":         queryResponse.Data != nil,
	})

	// Set existing hash
	existingHash := hash(resBytes)
	data.ExistingHash = types.StringValue(fmt.Sprint(existingHash))

	// Compute mutation variables
	keysToUse := make(map[string]interface{})
	if !data.ComputeMutationKeys.IsNull() && !data.ComputeMutationKeys.IsUnknown() {
		elements := make(map[string]types.String)
		diags.Append(data.ComputeMutationKeys.ElementsAs(ctx, &elements, false)...)
		if diags.HasError() {
			return nil, diags
		}
		for k, v := range elements {
			keysToUse[k] = v.ValueString()
		}
	}

	tflog.Debug(ctx, "Computing mutation variables", map[string]any{
		"keysToUse": keysToUse,
	})

	if err := r.computeMutationVariables(string(resBytes), data, keysToUse); err != nil {
		diags.AddError("Computation Error", fmt.Sprintf("Unable to compute keys from create response: %s", err))
		return nil, diags
	}

	return resBytes, diags
}

func (r *GraphqlMutationResource) executeUpdateHook(ctx context.Context, data *GraphqlMutationResourceModel, config *graphqlProviderConfig) ([]byte, diag.Diagnostics) {
	var diags diag.Diagnostics

	// Get update mutation variables
	var updateVariables map[string]interface{}
	updateVarsStr := data.MutationVariables.ValueString()
	if updateVarsStr != "" {
		if err := json.Unmarshal([]byte(updateVarsStr), &updateVariables); err != nil {
			diags.AddError("Update Variables Error", fmt.Sprintf("Failed to unmarshal mutation_variables: %s", err))
			return nil, diags
		}
	} else {
		updateVariables = make(map[string]interface{})
	}

	// Convert variables to JSON string
	var updateVarsJSON string
	if len(updateVariables) > 0 {
		updateVarsBytes, err := json.Marshal(updateVariables)
		if err != nil {
			diags.AddError("Update Variables Error", fmt.Sprintf("Failed to marshal update variables: %s", err))
			return nil, diags
		}
		updateVarsJSON = string(updateVarsBytes)
	}

	// Execute update query with fallback mechanism
	queryResponse, resBytes, diags := r.queryExecuteFramework(ctx, config, data.UpdateMutation.ValueString(), updateVarsJSON, false)

	// Check if the error is related to patch structure and we should retry without patch
	if diags.HasError() && data.WrapUpdateInPatch.ValueBool() {
		// Check if the error is related to invalid input type or patch structure
		errorMsg := ""
		for _, diag := range diags {
			errorMsg += diag.Detail() + " "
		}

		if strings.Contains(errorMsg, "invalid type for variable: 'input'") ||
			strings.Contains(errorMsg, "unknown field") ||
			strings.Contains(errorMsg, "422") {

			tflog.Debug(ctx, "Update with patch failed, retrying without patch wrapper", map[string]any{
				"error": errorMsg,
			})

			// Retry without patch wrapper
			originalMutationVariables := data.MutationVariables.ValueString()
			if originalMutationVariables != "" {
				// Use original mutation variables without patch wrapper
				queryResponse, resBytes, diags = r.queryExecuteFramework(ctx, config, data.UpdateMutation.ValueString(), originalMutationVariables, false)

				if !diags.HasError() {
					tflog.Debug(ctx, "Update succeeded without patch wrapper")
				} else {
					// Build error message from diagnostics
					errorMsg := ""
					for _, diag := range diags {
						errorMsg += diag.Detail() + " "
					}
					tflog.Debug(ctx, "Update failed even without patch wrapper", map[string]any{
						"error": errorMsg,
					})
				}
			}
		}
	}

	if diags.HasError() {
		return nil, diags
	}

	if len(queryResponse.Errors) > 0 {
		diags.AddError("GraphQL Update Error", fmt.Sprintf("GraphQL server returned errors: %v", queryResponse.Errors))
		return nil, diags
	}

	return resBytes, diags
}

func (r *GraphqlMutationResource) executeDeleteHook(ctx context.Context, data *GraphqlMutationResourceModel, config *graphqlProviderConfig) diag.Diagnostics {
	var diags diag.Diagnostics

	// Prepare delete variables
	deleteVarsStr := data.DeleteMutationVariables.ValueString()
	if data.ComputedValues.IsNull() || data.ComputedValues.IsUnknown() {
		diags.AddWarning("Missing Computed Values", "Cannot perform delete without computed values from a prior read or create.")
		return diags
	}

	computedVals := make(map[string]string)
	respDiags := data.ComputedValues.ElementsAs(ctx, &computedVals, false)
	diags.Append(respDiags...)
	if diags.HasError() {
		return diags
	}

	id, ok := computedVals["id"]
	if !ok {
		diags.AddError("Missing ID", "Computed values do not contain an 'id' for deletion.")
		return diags
	}

	var deleteVars map[string]interface{}
	if deleteVarsStr != "" {
		if err := json.Unmarshal([]byte(deleteVarsStr), &deleteVars); err != nil {
			diags.AddError("Delete Variables Error", fmt.Sprintf("Failed to unmarshal delete_mutation_variables: %s", err))
			return diags
		}
	} else {
		deleteVars = make(map[string]interface{})
	}

	// Ensure the delete variables have the proper structure with input wrapper
	if _, hasInput := deleteVars["input"]; !hasInput {
		deleteVars["input"] = make(map[string]interface{})
	}
	if inputMap, ok := deleteVars["input"].(map[string]interface{}); ok {
		inputMap["id"] = id
	} else {
		deleteVars["input"] = map[string]interface{}{
			"id": id,
		}
	}

	deleteVarsBytes, err := json.Marshal(deleteVars)
	if err != nil {
		diags.AddError("Delete Variables Error", fmt.Sprintf("Failed to marshal delete variables: %s", err))
		return diags
	}

	queryResponse, _, diags := r.queryExecuteFramework(ctx, config, data.DeleteMutation.ValueString(), string(deleteVarsBytes), false)
	if diags.HasError() {
		return diags
	}

	if len(queryResponse.Errors) > 0 {
		for _, gqlErr := range queryResponse.Errors {
			diags.AddError("GraphQL Server Error", gqlErr.Message)
		}
		return diags
	}

	return diags
}

func (r *GraphqlMutationResource) readResource(ctx context.Context, data *GraphqlMutationResourceModel, config *graphqlProviderConfig) diag.Diagnostics {
	var diags diag.Diagnostics

	// Prepare read variables
	var queryVariables map[string]interface{}
	readVarsStr := data.ReadQueryVariables.ValueString()
	if readVarsStr != "" {
		if err := json.Unmarshal([]byte(readVarsStr), &queryVariables); err != nil {
			diags.AddError("Read Variables Error", fmt.Sprintf("Failed to unmarshal read_query_variables: %s", err))
			return diags
		}
	} else {
		queryVariables = make(map[string]interface{})
	}

	// Add computed values to variables
	computedVariables := make(map[string]interface{})
	for k, v := range queryVariables {
		if str, ok := v.(string); ok {
			computedVariables[k] = str
		} else {
			bytes, err := json.Marshal(v)
			if err != nil {
				diags.AddError("Variable Marshaling Error", fmt.Sprintf("Failed to marshal value for key %s in read_query_variables: %s", k, err))
				return diags
			}
			computedVariables[k] = string(bytes)
		}
	}

	// Add computed values from previous operations
	if !data.ComputedValues.IsNull() && !data.ComputedValues.IsUnknown() {
		elements := make(map[string]types.String)
		diags.Append(data.ComputedValues.ElementsAs(ctx, &elements, false)...)
		if diags.HasError() {
			return diags
		}
		for k, v := range elements {
			computedVariables[k] = v.ValueString()
		}
	}

	// Add the resource ID to the variables if it exists
	if !data.Id.IsNull() && !data.Id.IsUnknown() {
		computedVariables["id"] = data.Id.ValueString()
	}

	// Set computed read operation variables
	computedVarsMap := make(map[string]attr.Value)
	for k, v := range computedVariables {
		if str, ok := v.(string); ok {
			computedVarsMap[k] = types.StringValue(str)
		} else {
			bytes, err := json.Marshal(v)
			if err != nil {
				diags.AddError("Variable Marshaling Error", fmt.Sprintf("Failed to marshal computed variable %s: %s", k, err))
				return diags
			}
			computedVarsMap[k] = types.StringValue(string(bytes))
		}
	}
	data.ComputedValues = types.MapValueMust(types.StringType, computedVarsMap)

	readVarsBytes, err := json.Marshal(computedVariables)
	if err != nil {
		diags.AddError("Read Variables Error", fmt.Sprintf("Failed to marshal read variables: %s", err))
		return diags
	}

	// Execute read query
	queryResponse, resBytes, diags := r.queryExecuteFramework(ctx, config, data.ReadQuery.ValueString(), string(readVarsBytes), false)
	if diags.HasError() {
		// Check if it's a "not found" error
		for _, diag := range diags {
			if strings.Contains(diag.Detail(), "Not Found") || strings.Contains(diag.Detail(), "Cannot return null for non-nullable field") {
				log.Printf("[WARN] Resource not found on remote, removing from state.")
				data.Id = types.StringNull()
				return diags
			}
		}
		return diags
	}

	if len(queryResponse.Errors) > 0 {
		diags.AddError("GraphQL Read Error", fmt.Sprintf("GraphQL server returned errors: %v", queryResponse.Errors))
		return diags
	}

	// Check for null data
	if dataMap, ok := queryResponse.Data["data"].(map[string]interface{}); ok {
		for key, value := range dataMap {
			if value == nil {
				log.Printf("[WARN] Primary data object '%s' is null.", key)
				diags.AddWarning("Null Data", fmt.Sprintf("Resource data is not yet available ('%s' is null)", key))
				return diags
			}
			break
		}
	}

	// Set query response
	data.QueryResponse = types.StringValue(string(resBytes))

	// Determine keys to use for computation
	keysToUse := make(map[string]interface{})
	if !data.ComputeMutationKeys.IsNull() && !data.ComputeMutationKeys.IsUnknown() {
		elements := make(map[string]types.String)
		diags.Append(data.ComputeMutationKeys.ElementsAs(ctx, &elements, false)...)
		if diags.HasError() {
			return diags
		}
		for k, v := range elements {
			keysToUse[k] = v.ValueString()
		}
	}

	// Use read compute keys if provided
	if !data.ReadComputeKeys.IsNull() && !data.ReadComputeKeys.IsUnknown() {
		log.Printf("[DEBUG] Using user-defined read_compute_keys for parsing Read response.")
		elements := make(map[string]types.String)
		diags.Append(data.ReadComputeKeys.ElementsAs(ctx, &elements, false)...)
		if diags.HasError() {
			return diags
		}
		keysToUse = make(map[string]interface{})
		for k, v := range elements {
			keysToUse[k] = v.ValueString()
		}
	} else if data.ComputeFromRead.ValueBool() {
		log.Printf("[DEBUG] compute_from_read is true. Auto-generating keys from Read response.")
		autoGeneratedKeys, err := r.generateKeysFromResponse(ctx, resBytes)
		if err != nil {
			log.Printf("[WARN] Failed to auto-generate keys from read response: %s", err)
		} else if len(autoGeneratedKeys) > 0 {
			keysToUse = autoGeneratedKeys
		}
	}

	// Compute mutation variables
	if err := r.computeMutationVariables(string(resBytes), data, keysToUse); err != nil {
		diags.AddError("Computation Error", fmt.Sprintf("Unable to compute keys from read response: %s", err))
		return diags
	}

	// Set the ID from computed values
	if !data.ComputedValues.IsNull() && !data.ComputedValues.IsUnknown() {
		elements := make(map[string]types.String)
		diags.Append(data.ComputedValues.ElementsAs(ctx, &elements, false)...)
		if diags.HasError() {
			return diags
		}

		// Look for an 'id' key in computed values
		if idValue, ok := elements["id"]; ok {
			data.Id = idValue
		} else {
			// If no 'id' key found, try to generate a hash-based ID
			existingHash := hash(resBytes)
			data.Id = types.StringValue(fmt.Sprintf("%d", existingHash))
		}
	} else {
		// Fallback: generate ID from response hash
		existingHash := hash(resBytes)
		data.Id = types.StringValue(fmt.Sprintf("%d", existingHash))
	}

	// Set existing hash
	existingHash := hash(resBytes)
	data.ExistingHash = types.StringValue(fmt.Sprintf("%d", existingHash))

	return diags
}

func (r *GraphqlMutationResource) queryExecuteFramework(ctx context.Context, config *graphqlProviderConfig, query string, variablesStr string, usePagination bool) (*GqlQueryResponse, []byte, diag.Diagnostics) {
	var diags diag.Diagnostics
	var variables map[string]interface{}
	if variablesStr != "" {
		if err := json.Unmarshal([]byte(variablesStr), &variables); err != nil {
			diags.AddError("Variable Parsing Error", fmt.Sprintf("failed to unmarshal variables: %v", err))
			return nil, nil, diags
		}
	}

	// Convert variables to JSON string for the framework function
	var variablesJSON string
	if len(variables) > 0 {
		variablesBytes, err := json.Marshal(variables)
		if err != nil {
			diags.AddError("Variable Marshaling Error", fmt.Sprintf("failed to marshal variables: %v", err))
			return nil, nil, diags
		}
		variablesJSON = string(variablesBytes)
	}

	return queryExecuteFramework(ctx, config, query, variablesJSON, usePagination)
}

func (r *GraphqlMutationResource) computeMutationVariables(queryResponse string, data *GraphqlMutationResourceModel, dataKeys map[string]interface{}) error {
	mvks, err := computeMutationVariableKeys(dataKeys, queryResponse)
	if err != nil {
		return err
	}

	// Set computed values
	computedValuesMap := make(map[string]attr.Value)
	for k, v := range mvks {
		computedValuesMap[k] = types.StringValue(v)
	}
	data.ComputedValues = types.MapValueMust(types.StringType, computedValuesMap)
	return nil
}

func (r *GraphqlMutationResource) prepareUpdatePayload(ctx context.Context, data *GraphqlMutationResourceModel, req resource.UpdateRequest) error {
	var mutationVariables map[string]interface{}
	mutVarsStr := data.MutationVariables.ValueString()
	if mutVarsStr != "" {
		if err := json.Unmarshal([]byte(mutVarsStr), &mutationVariables); err != nil {
			return fmt.Errorf("failed to unmarshal mutation_variables: %w", err)
		}
	} else {
		mutationVariables = make(map[string]interface{})
	}

	// Get computed values
	computedValues := make(map[string]string)
	if !data.ComputedValues.IsNull() && !data.ComputedValues.IsUnknown() {
		diags := data.ComputedValues.ElementsAs(ctx, &computedValues, false)
		if diags.HasError() {
			return fmt.Errorf("failed to get computed values: %w", diags)
		}
	}

	// Get previous state to compare changes
	var state GraphqlMutationResourceModel
	req.State.Get(ctx, &state)

	tflog.Debug(ctx, "DEBUG: Mutation variables and computed values", map[string]any{
		"mutationVariables": mutationVariables,
		"computedValues":    computedValues,
		"wrapUpdateInPatch": data.WrapUpdateInPatch.ValueBool(),
	})

	computedID, idExists := computedValues["id"]

	// For update operations, we need to restructure the variables
	uvks := mutationVariables

	// Always wrap in update structure for update operations
	if idExists {
		tflog.Debug(ctx, "Restructuring variables for update operation")
		patch := make(map[string]interface{})

		// Extract fields from the input object and only include changed fields
		if inputObj, ok := uvks["input"].(map[string]interface{}); ok {
			// Get previous state mutation variables for comparison
			var prevMutationVariables map[string]interface{}
			prevMutVarsStr := state.MutationVariables.ValueString()
			if prevMutVarsStr != "" {
				if err := json.Unmarshal([]byte(prevMutVarsStr), &prevMutationVariables); err == nil {
					if prevInputObj, ok := prevMutationVariables["input"].(map[string]interface{}); ok {
						// Only include fields that have changed
						for k, v := range inputObj {
							if prevVal, exists := prevInputObj[k]; !exists || !reflect.DeepEqual(v, prevVal) {
								patch[k] = v
								tflog.Debug(ctx, "Field changed, including in patch", map[string]any{
									"field":    k,
									"newValue": v,
									"oldValue": prevVal,
								})
							}
						}
					}
				}
			}

			// If no changes detected, include all fields (fallback)
			if len(patch) == 0 {
				tflog.Debug(ctx, "No changes detected, including all fields in patch")
				for k, v := range inputObj {
					patch[k] = v
				}
			}
		} else {
			// If there's no input wrapper, use the variables directly but filter
			var prevMutationVariables map[string]interface{}
			prevMutVarsStr := state.MutationVariables.ValueString()
			if prevMutVarsStr != "" {
				if err := json.Unmarshal([]byte(prevMutVarsStr), &prevMutationVariables); err == nil {
					// Only include fields that have changed
					for k, v := range uvks {
						if prevVal, exists := prevMutationVariables[k]; !exists || !reflect.DeepEqual(v, prevVal) {
							patch[k] = v
							tflog.Debug(ctx, "Field changed, including in patch", map[string]any{
								"field":    k,
								"newValue": v,
								"oldValue": prevVal,
							})
						}
					}
				}
			}

			// If no changes detected, include all fields (fallback)
			if len(patch) == 0 {
				tflog.Debug(ctx, "No changes detected, including all fields in patch")
				for k, v := range uvks {
					patch[k] = v
				}
			}
		}

		uvks = map[string]interface{}{
			"input": map[string]interface{}{
				"id":    computedID,
				"patch": patch,
			},
		}
		tflog.Debug(ctx, "DEBUG: Final wrapped payload structure", map[string]any{
			"finalPayload": uvks,
		})
	}

	updateVarsBytes, err := json.Marshal(uvks)
	if err != nil {
		return fmt.Errorf("failed to marshal update variables: %w", err)
	}

	tflog.Debug(ctx, "Update mutation payload", map[string]any{"payload": string(updateVarsBytes)})

	data.ComputedValues = types.MapValueMust(types.StringType, map[string]attr.Value{
		"variables": types.StringValue(string(updateVarsBytes)),
	})

	return nil
}

func (r *GraphqlMutationResource) generateKeysFromResponse(ctx context.Context, responseBytes []byte) (map[string]interface{}, error) {
	var robj map[string]interface{}
	if err := json.Unmarshal(responseBytes, &robj); err != nil {
		return nil, err
	}

	data, ok := robj["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("response JSON does not contain a 'data' object")
	}

	generatedKeys := make(map[string]interface{})
	r.flattenRecursive(ctx, "", data, generatedKeys)
	return generatedKeys, nil
}

func (r *GraphqlMutationResource) flattenRecursive(ctx context.Context, prefix string, data interface{}, keyMap map[string]interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, val := range v {
			newPrefix := key
			if prefix != "" {
				newPrefix = prefix + "." + key
			}
			r.flattenRecursive(ctx, newPrefix, val, keyMap)
		}
	case []interface{}:
		for i, val := range v {
			newPrefix := fmt.Sprintf("%s.%d", prefix, i)
			r.flattenRecursive(ctx, newPrefix, val, keyMap)
		}
	default:
		pathParts := strings.Split(prefix, ".")
		leafKey := pathParts[len(pathParts)-1]
		if _, exists := keyMap[leafKey]; !exists {
			keyMap[leafKey] = prefix
			tflog.Debug(ctx, fmt.Sprintf("Auto-generated key: '%s' -> '%s'", leafKey, prefix))
		}
	}
}
