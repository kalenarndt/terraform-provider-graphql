package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/kalenarndt/terraform-provider-graphql/internal/utils"
)

// GraphqlMutationResource represents the GraphQL mutation resource
type GraphqlMutationResource struct {
	config *graphqlProviderConfig
}

// GraphqlMutationResourceModel describes the resource data model
type GraphqlMutationResourceModel struct {
	ReadQuery                        types.String  `tfsdk:"read_query"`
	CreateMutation                   types.String  `tfsdk:"create_mutation"`
	DeleteMutation                   types.String  `tfsdk:"delete_mutation"`
	UpdateMutation                   types.String  `tfsdk:"update_mutation"`
	MutationVariables                types.Dynamic `tfsdk:"mutation_variables"`
	ReadQueryVariables               types.Dynamic `tfsdk:"read_query_variables"`
	DeleteMutationVariables          types.Dynamic `tfsdk:"delete_mutation_variables"`
	ComputeMutationKeys              types.Map     `tfsdk:"compute_mutation_keys"`
	ReadComputeKeys                  types.Map     `tfsdk:"read_compute_keys"`
	ComputeFromRead                  types.Bool    `tfsdk:"compute_from_read"`
	WrapUpdateInPatch                types.Bool    `tfsdk:"wrap_update_in_patch"`
	CreateOnlyFields                 types.List    `tfsdk:"create_only_fields"`
	ComputedValues                   types.Map     `tfsdk:"computed_values"`
	ForceReplace                     types.Bool    `tfsdk:"force_replace"`
	EnableRemoteStateVerification    types.Bool    `tfsdk:"enable_remote_state_verification"`
	ComputedReadOperationVariables   types.Map     `tfsdk:"computed_read_operation_variables"`
	ComputedUpdateOperationVariables types.String  `tfsdk:"computed_update_operation_variables"`
	ComputedCreateOperationVariables types.String  `tfsdk:"computed_create_operation_variables"`
	ComputedDeleteOperationVariables types.Map     `tfsdk:"computed_delete_operation_variables"`
	QueryResponse                    types.String  `tfsdk:"query_response"`
	ExistingHash                     types.String  `tfsdk:"existing_hash"`
	CurrentRemoteState               types.String  `tfsdk:"current_remote_state"`
	Id                               types.String  `tfsdk:"id"`
}

// Add this helper function at file scope:
func deepDiff(desired, remote map[string]interface{}) map[string]interface{} {
	diff := make(map[string]interface{})
	for k, v := range desired {
		remoteVal, ok := remote[k]
		switch vTyped := v.(type) {
		case map[string]interface{}:
			if ok {
				if remoteMap, ok2 := remoteVal.(map[string]interface{}); ok2 {
					subDiff := deepDiff(vTyped, remoteMap)
					if len(subDiff) > 0 {
						diff[k] = subDiff
					}
				} else {
					diff[k] = v
				}
			} else {
				diff[k] = v
			}
		default:
			if !ok || !reflect.DeepEqual(v, remoteVal) {
				diff[k] = v
			}
		}
	}
	return diff
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
			"mutation_variables": schema.DynamicAttribute{
				Required:    true,
				Description: "Variables for the create and update operations. Can be any valid JSON value (object, array, string, number, boolean, null).",
			},
			"read_query_variables": schema.DynamicAttribute{
				Optional:    true,
				Description: "Variables for the read query. Can be any valid JSON value (object, array, string, number, boolean, null).",
			},
			"delete_mutation_variables": schema.DynamicAttribute{
				Optional:    true,
				Description: "Variables for the delete mutation. Can be any valid JSON value (object, array, string, number, boolean, null).",
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
				Description: "If true, update mutations will wrap changed fields in a 'patch' object under 'input'. Use this for APIs that require patch-style updates.",
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
			"enable_remote_state_verification": schema.BoolAttribute{
				Optional:    true,
				Description: "A pre v2.4.0 backward-compatibility flag. Set to false to disable resource remote state verification during reads. Defaults to true.",
			},
			"computed_read_operation_variables": schema.MapAttribute{
				ElementType: types.StringType,
				Computed:    true,
				Description: "Computed variables for read operations.",
			},
			"computed_update_operation_variables": schema.StringAttribute{
				Computed:    true,
				Description: "Computed variables for update operations. This shows the actual mutation payload that will be sent to the GraphQL API.",
			},
			"computed_create_operation_variables": schema.StringAttribute{
				Computed:    true,
				Description: "Computed variables for create operations.",
			},
			"computed_delete_operation_variables": schema.MapAttribute{
				ElementType: types.StringType,
				Computed:    true,
				Description: "Computed variables for delete operations.",
			},
			"query_response": schema.StringAttribute{
				Computed:    true,
				Description: "The raw body of the HTTP response from the last read of the object.",
			},
			"existing_hash": schema.StringAttribute{
				Computed:    true,
				Description: "Represents the state of existence of a mutation in order to support intelligent updates.",
			},
			"current_remote_state": schema.StringAttribute{
				Computed:    true,
				Description: "The current remote state of the resource, used for drift detection. This field is automatically populated during read operations.",
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
	createBytes, diags := r.executeCreateHook(ctx, &data, r.config)
	if diags.HasError() {
		resp.Diagnostics.Append(diags...)
		return
	}

	// Set the query response from the create operation
	data.QueryResponse = types.StringValue(string(createBytes))

	// Try to read the resource to populate computed fields, but don't fail if it doesn't work
	readDiags := r.readResource(ctx, &data, r.config)
	if readDiags.HasError() {
		tflog.Debug(ctx, "Read operation failed after create, but continuing", map[string]any{
			"errors": readDiags,
		})
		// Don't fail the create operation if read fails
		// The resource was created successfully, we just couldn't read it back
		// This can happen if the resource takes time to become available
	}

	// Ensure we have an ID set
	if data.Id.IsNull() || data.Id.IsUnknown() {
		// Generate a hash-based ID from the create response
		existingHash := hash(createBytes)
		data.Id = types.StringValue(fmt.Sprintf("%d", existingHash))
	}

	// Ensure computed values are set even if read failed
	if data.ComputedValues.IsNull() || data.ComputedValues.IsUnknown() {
		// Set empty computed values map
		data.ComputedValues = types.MapValueMust(types.StringType, make(map[string]attr.Value))
	}

	// Ensure computed read operation variables are set
	if data.ComputedReadOperationVariables.IsNull() || data.ComputedReadOperationVariables.IsUnknown() {
		// Set empty computed read operation variables
		data.ComputedReadOperationVariables = types.MapValueMust(types.StringType, make(map[string]attr.Value))
	}

	// Ensure existing hash is set
	if data.ExistingHash.IsNull() || data.ExistingHash.IsUnknown() {
		existingHash := hash(createBytes)
		data.ExistingHash = types.StringValue(fmt.Sprintf("%d", existingHash))
	}

	// Ensure computed update operation variables are set
	if data.ComputedUpdateOperationVariables.IsNull() || data.ComputedUpdateOperationVariables.IsUnknown() {
		data.ComputedUpdateOperationVariables = types.StringValue("")
	}

	// Ensure computed delete operation variables are set
	if data.ComputedDeleteOperationVariables.IsNull() || data.ComputedDeleteOperationVariables.IsUnknown() {
		data.ComputedDeleteOperationVariables = types.MapValueMust(types.StringType, make(map[string]attr.Value))
	}

	// Ensure current remote state is set
	if data.CurrentRemoteState.IsNull() || data.CurrentRemoteState.IsUnknown() {
		data.CurrentRemoteState = types.StringValue("")
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

	// CRITICAL: Preserve the original mutation_variables from the state
	// This ensures we don't modify the user's intended configuration
	originalMutationVariables := data.MutationVariables

	// Read the resource
	diags := r.readResource(ctx, &data, r.config)
	if diags.HasError() {
		// Check if the error indicates the resource was deleted
		resourceDeleted := false
		for _, diag := range diags {
			errorMsg := strings.ToLower(diag.Detail())
			if strings.Contains(errorMsg, "not found") ||
				strings.Contains(errorMsg, "deleted") ||
				strings.Contains(errorMsg, "does not exist") ||
				strings.Contains(errorMsg, "was deleted") ||
				strings.Contains(errorMsg, "deployment not found") ||
				strings.Contains(errorMsg, "connector was deleted") ||
				strings.Contains(errorMsg, "cannot return null for non-nullable field") {
				resourceDeleted = true
				break
			}
		}

		if resourceDeleted {
			tflog.Info(ctx, "Resource not found on remote (transport error indicates deletion), removing from state")
			resp.State.RemoveResource(ctx)
			return
		}

		resp.Diagnostics.Append(diags...)
		return
	}

	// Check if the resource was marked as not found (ID is null)
	if data.Id.IsNull() {
		tflog.Info(ctx, "Resource not found, removing from state")
		// Explicitly remove the resource from state
		resp.State.RemoveResource(ctx)
		return
	}

	// CRITICAL: Restore the original mutation_variables to preserve user's configuration
	// This prevents the provider from storing a different value than what's in the config
	data.MutationVariables = originalMutationVariables

	// Add debug logging to see what the response contains
	if !data.QueryResponse.IsNull() && !data.QueryResponse.IsUnknown() {
		responseStr := data.QueryResponse.ValueString()
		tflog.Debug(ctx, "GraphQL response content", map[string]any{
			"responseLength": len(responseStr),
			"response":       responseStr,
		})
	}

	// CAPTURE ALL STATE: According to Plugin Framework best practices,
	// the Read method should capture the complete current remote state
	// This allows Terraform's plan phase to detect differences between
	// desired state (configuration) and current state (from Read)
	if !data.QueryResponse.IsNull() && !data.QueryResponse.IsUnknown() {
		queryResponseStr := data.QueryResponse.ValueString()
		if queryResponseStr != "" {
			var queryResponse map[string]interface{}
			if err := json.Unmarshal([]byte(queryResponseStr), &queryResponse); err == nil {
				// Extract current remote state
				currentRemoteState := r.extractCurrentStateFromQueryResponse(ctx, queryResponse)

				// Parse desired state from mutation variables
				var desiredState map[string]interface{}
				if !data.MutationVariables.IsNull() && !data.MutationVariables.IsUnknown() {
					mutVarsStr, diags := utils.DynamicToJSONString(ctx, data.MutationVariables)
					if !diags.HasError() && mutVarsStr != "" {
						if err := json.Unmarshal([]byte(mutVarsStr), &desiredState); err == nil {
							// Extract desired fields
							var desiredFields map[string]interface{}
							if inputObj, ok := desiredState["input"].(map[string]interface{}); ok {
								desiredFields = inputObj
							} else {
								desiredFields = desiredState
							}

							// Check for drift for logging purposes only
							changedFields := r.findChangedFields(ctx, desiredFields, currentRemoteState)
							hasDrift := len(changedFields) > 0

							tflog.Debug(ctx, "State comparison in Read", map[string]any{
								"desiredFields":      desiredFields,
								"currentRemoteState": currentRemoteState,
								"changedFields":      changedFields,
								"hasDrift":           hasDrift,
							})

							// Store current remote state for drift detection
							currentStateBytes, _ := json.Marshal(currentRemoteState)
							data.CurrentRemoteState = types.StringValue(string(currentStateBytes))

							if hasDrift {
								tflog.Info(ctx, "DRIFT DETECTED - Resource state differs from desired configuration", map[string]any{
									"changedFields": changedFields,
								})

								// CRITICAL: Signal drift to Terraform by modifying the mutation_variables
								// to reflect the current remote state, so Terraform can detect the difference
								if !data.WrapUpdateInPatch.IsNull() && !data.WrapUpdateInPatch.IsUnknown() && data.WrapUpdateInPatch.ValueBool() {
									// For patch updates, update the patch field to reflect current state
									updatedMutationVars := map[string]interface{}{
										"input": map[string]interface{}{
											"id":    desiredFields["id"],
											"patch": currentRemoteState,
										},
									}
									updatedMutationVarsBytes, _ := json.Marshal(updatedMutationVars)
									data.MutationVariables = types.DynamicValue(types.StringValue(string(updatedMutationVarsBytes)))
								} else {
									// For direct updates, update the input field to reflect current state
									updatedMutationVars := map[string]interface{}{
										"input": currentRemoteState,
									}
									updatedMutationVarsBytes, _ := json.Marshal(updatedMutationVars)
									data.MutationVariables = types.DynamicValue(types.StringValue(string(updatedMutationVarsBytes)))
								}

								// Compute minimal patch or input for update
								wrapPatch := false
								if !data.WrapUpdateInPatch.IsNull() && !data.WrapUpdateInPatch.IsUnknown() {
									wrapPatch = data.WrapUpdateInPatch.ValueBool()
								}
								if wrapPatch {
									// Only put changed fields in patch using findChangedFields
									patch := r.findChangedFields(ctx, desiredFields, currentRemoteState)
									updateVars := map[string]interface{}{
										"input": map[string]interface{}{
											"patch": patch,
										},
									}
									// Get the ID from computed values
									if !data.ComputedValues.IsNull() && !data.ComputedValues.IsUnknown() {
										computedValues := make(map[string]string)
										if diags := data.ComputedValues.ElementsAs(ctx, &computedValues, false); !diags.HasError() {
											if id, hasID := computedValues["id"]; hasID {
												updateVars["input"].(map[string]interface{})["id"] = id
											}
										}
									}
									updateVarsBytes, _ := json.Marshal(updateVars)
									data.ComputedUpdateOperationVariables = types.StringValue(string(updateVarsBytes))
									tflog.Info(ctx, "Set ComputedUpdateOperationVariables for patch update (deep diff)", map[string]any{
										"updateVars": updateVars,
									})
								} else {
									// No patch, update input directly
									updateVars := map[string]interface{}{
										"input": changedFields,
									}
									updateVarsBytes, _ := json.Marshal(updateVars)
									data.ComputedUpdateOperationVariables = types.StringValue(string(updateVarsBytes))
									tflog.Info(ctx, "Set ComputedUpdateOperationVariables for direct update", map[string]any{
										"updateVars": updateVars,
									})
								}
							} else {
								tflog.Debug(ctx, "No drift detected")
							}
						}
					}
				}
			}
		}
	}

	// Set refreshed state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)

	tflog.Debug(ctx, "Final state before commit", map[string]any{
		"currentRemoteState": data.CurrentRemoteState.ValueString(),
		"queryResponse":      data.QueryResponse.ValueString(),
		"computedValues":     data.ComputedValues,
		"success":            true,
	})

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

	// CRITICAL: Preserve the original mutation_variables from the plan
	// This ensures we don't modify the user's intended configuration
	originalMutationVariables := data.MutationVariables

	// Ensure the ID is set from the previous state
	if !state.Id.IsNull() && !state.Id.IsUnknown() {
		data.Id = state.Id
	}

	// Check if force replace is enabled
	if data.ForceReplace.ValueBool() {
		tflog.Debug(ctx, "Force replace enabled, deleting and recreating resource")

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
		tflog.Debug(ctx, "Performing patch update")

		// Check if remote state verification is enabled (defaults to true)
		enableRemoteStateVerification := true
		if !data.EnableRemoteStateVerification.IsNull() && !data.EnableRemoteStateVerification.IsUnknown() {
			enableRemoteStateVerification = data.EnableRemoteStateVerification.ValueBool()
		}

		if enableRemoteStateVerification {
			tflog.Debug(ctx, "Remote state verification enabled, reading current state")
			// Read the resource first to populate computed values and verify current state
			diags := r.readResource(ctx, &data, r.config)
			if diags.HasError() {
				resp.Diagnostics.Append(diags...)
				return
			}

			// CRITICAL: Read the actual remote state and compare with desired state
			// This ensures we detect drift by comparing live remote state with desired configuration
			if !data.QueryResponse.IsNull() && !data.QueryResponse.IsUnknown() {
				var queryResponse map[string]interface{}
				if err := json.Unmarshal([]byte(data.QueryResponse.ValueString()), &queryResponse); err == nil {
					currentRemoteState := r.extractCurrentStateFromQueryResponse(ctx, queryResponse)

					// Get desired state from mutation variables
					var desiredFields map[string]interface{}
					if !data.MutationVariables.IsNull() && !data.MutationVariables.IsUnknown() {
						mutVarsStr, diags := utils.DynamicToJSONString(ctx, data.MutationVariables)
						if !diags.HasError() && mutVarsStr != "" {
							if err := json.Unmarshal([]byte(mutVarsStr), &desiredFields); err == nil {
								// Extract fields from desired state, handling patch structure
								if patch, hasPatch := desiredFields["patch"].(map[string]interface{}); hasPatch {
									desiredFields = patch
								}

								// Compare current remote state with desired state
								changedFields := r.findChangedFields(ctx, desiredFields, currentRemoteState)
								hasDrift := len(changedFields) > 0

								tflog.Debug(ctx, "Drift detection in Update", map[string]any{
									"currentRemoteState": currentRemoteState,
									"desiredFields":      desiredFields,
									"changedFields":      changedFields,
									"hasDrift":           hasDrift,
								})

								if hasDrift {
									tflog.Info(ctx, "DRIFT DETECTED in Update - Resource state differs from desired configuration", map[string]any{
										"changedFields": changedFields,
									})
								} else {
									tflog.Debug(ctx, "No drift detected - resource state matches desired configuration")
								}
							}
						}
					}
				}
			}
		} else {
			tflog.Debug(ctx, "Remote state verification disabled, skipping read operation")
		}

		// Prepare update payload to create patch operations
		if err := r.prepareUpdatePayload(ctx, &data, req); err != nil {
			resp.Diagnostics.AddError("Update Payload Error", err.Error())
			return
		}

		// Log the computed update variables for debugging
		if !data.ComputedUpdateOperationVariables.IsNull() && !data.ComputedUpdateOperationVariables.IsUnknown() {
			tflog.Debug(ctx, "Computed update variables", map[string]any{
				"updateVariables": data.ComputedUpdateOperationVariables.ValueString(),
			})
		} else {
			tflog.Debug(ctx, "No computed update variables found, skipping update")
		}

		// Check if we actually need to perform an update
		updateNeeded := true
		if !data.ComputedUpdateOperationVariables.IsNull() && !data.ComputedUpdateOperationVariables.IsUnknown() {
			updateVarsStr := data.ComputedUpdateOperationVariables.ValueString()
			if updateVarsStr != "" {
				var updateVars map[string]interface{}
				if err := json.Unmarshal([]byte(updateVarsStr), &updateVars); err == nil {
					if input, ok := updateVars["input"].(map[string]interface{}); ok {
						// If there's no patch or the patch is empty, no update is needed
						if patch, hasPatch := input["patch"]; !hasPatch || patch == nil {
							updateNeeded = false
							tflog.Debug(ctx, "No update needed - no changes detected")
						}
					}
				}
			}
		}

		if updateNeeded {
			var updatePayload string
			if !data.ComputedUpdateOperationVariables.IsNull() && data.ComputedUpdateOperationVariables.ValueString() != "" {
				updatePayload = data.ComputedUpdateOperationVariables.ValueString()
				tflog.Info(ctx, "Using ComputedUpdateOperationVariables as update payload", map[string]any{
					"payload": updatePayload,
				})
			} else {
				// fallback to original mutation_variables
				mutVarsStr, diags := utils.DynamicToJSONString(ctx, data.MutationVariables)
				if !diags.HasError() {
					updatePayload = mutVarsStr
				} else {
					updatePayload = "<error>"
				}
				tflog.Info(ctx, "Using original mutation_variables as update payload", map[string]any{
					"payload": updatePayload,
				})
			}
			// Execute update operation using computed update variables (patch)
			_, updateDiags := r.executeUpdateHook(ctx, &data, r.config)
			if updateDiags.HasError() {
				resp.Diagnostics.Append(updateDiags...)
				return
			}
		} else {
			tflog.Debug(ctx, "Skipping update operation - no changes detected")
		}

		// Read the resource again to populate computed fields after update
		readDiags := r.readResource(ctx, &data, r.config)
		if readDiags.HasError() {
			resp.Diagnostics.Append(readDiags...)
			return
		}

		// CRITICAL: Ensure CurrentRemoteState is set to a known value after update
		// This prevents the provider from returning an unknown value after apply
		if !data.QueryResponse.IsNull() && !data.QueryResponse.IsUnknown() {
			var queryResponse map[string]interface{}
			if err := json.Unmarshal([]byte(data.QueryResponse.ValueString()), &queryResponse); err == nil {
				currentRemoteState := r.extractCurrentStateFromQueryResponse(ctx, queryResponse)
				currentStateBytes, _ := json.Marshal(currentRemoteState)
				data.CurrentRemoteState = types.StringValue(string(currentStateBytes))

				tflog.Debug(ctx, "Set CurrentRemoteState after update", map[string]any{
					"currentRemoteState": currentRemoteState,
					"currentStateBytes":  string(currentStateBytes),
				})
			}
		}
	}

	// CRITICAL: Restore the original mutation_variables to preserve user's configuration
	// This prevents the provider from storing a different value than what's in the config
	data.MutationVariables = originalMutationVariables

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

	// Convert mutation variables to JSON for logging
	mutVarsStr, _ := utils.DynamicToJSONString(ctx, data.MutationVariables)
	tflog.Debug(ctx, "Executing create hook", map[string]any{
		"createMutation":    data.CreateMutation.ValueString(),
		"mutationVariables": mutVarsStr,
	})

	// Set computed create operation variables
	// Convert mutation variables to JSON string
	mutationVarsStr, diags := utils.DynamicToJSONString(ctx, data.MutationVariables)
	if diags.HasError() {
		return nil, diags
	}
	data.ComputedCreateOperationVariables = types.StringValue(mutationVarsStr)

	// Set computed update operation variables (empty for create)
	data.ComputedUpdateOperationVariables = types.StringValue("")

	// Set computed delete operation variables as JSON string
	deleteVars := map[string]interface{}{
		"input": map[string]interface{}{
			"id": "", // Will be populated after create
		},
	}
	deleteVarsBytes, err := json.Marshal(deleteVars)
	if err != nil {
		diags.AddError("Delete Variables Error", fmt.Sprintf("Failed to marshal delete variables: %s", err))
		return nil, diags
	}
	data.ComputedDeleteOperationVariables = types.MapValueMust(types.StringType, map[string]attr.Value{
		"variables": types.StringValue(string(deleteVarsBytes)),
	})

	// Execute create mutation
	queryResponse, resBytes, diags := r.queryExecuteFramework(ctx, config, data.CreateMutation.ValueString(), data.ComputedCreateOperationVariables.ValueString(), true)
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

	// Check if we have computed update variables (patch) available
	updateVarsStr := data.ComputedUpdateOperationVariables.ValueString()
	var variablesToUse string
	var usePatch bool

	if updateVarsStr != "" {
		tflog.Debug(ctx, "Using computed update variables (patch)", map[string]any{
			"computedUpdateVariables": updateVarsStr,
		})
		variablesToUse = updateVarsStr
		usePatch = true
	} else {
		// Fallback to original mutation variables
		mutationVarsStr, diags := utils.DynamicToJSONString(ctx, data.MutationVariables)
		if diags.HasError() {
			return nil, diags
		}
		tflog.Debug(ctx, "Using original mutation variables (full payload)", map[string]any{
			"mutationVariables": mutationVarsStr,
		})
		variablesToUse = mutationVarsStr
		usePatch = false
	}

	// Execute update query
	_, resBytes, diags := r.queryExecuteFramework(ctx, config, data.UpdateMutation.ValueString(), variablesToUse, false)

	// Check if the error is related to patch structure and we should retry without patch
	if diags.HasError() && usePatch {
		// Check if the error suggests patch format issues
		errorMsg := ""
		for _, diag := range diags {
			errorMsg += diag.Detail() + " "
		}

		if strings.Contains(errorMsg, "422") || strings.Contains(errorMsg, "unknown field") || strings.Contains(errorMsg, "Unprocessable Entity") {
			tflog.Debug(ctx, "Patch update failed, falling back to full payload", map[string]any{
				"error": errorMsg,
			})

			// Fallback to original mutation variables
			mutationVarsStr, fallbackDiags := utils.DynamicToJSONString(ctx, data.MutationVariables)
			if fallbackDiags.HasError() {
				return nil, fallbackDiags
			}

			tflog.Debug(ctx, "Retrying with full payload", map[string]any{
				"mutationVariables": mutationVarsStr,
			})

			// Retry with full payload
			_, resBytes, diags = r.queryExecuteFramework(ctx, config, data.UpdateMutation.ValueString(), mutationVarsStr, false)
		}
	}

	if diags.HasError() {
		return nil, diags
	}

	// Ensure computed fields are set to avoid unknown state
	// The computed update variables should already be set by prepareUpdatePayload
	// but we ensure they're not unknown here
	if data.ComputedUpdateOperationVariables.IsNull() || data.ComputedUpdateOperationVariables.IsUnknown() {
		// Fallback: set to the variables that were actually used
		data.ComputedUpdateOperationVariables = types.StringValue(variablesToUse)
	}

	return resBytes, nil
}

func (r *GraphqlMutationResource) executeDeleteHook(ctx context.Context, data *GraphqlMutationResourceModel, config *graphqlProviderConfig) diag.Diagnostics {
	var diags diag.Diagnostics

	// Prepare delete variables
	deleteVarsStr, diags := utils.DynamicToJSONString(ctx, data.DeleteMutationVariables)
	if diags.HasError() {
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

	// Try to get ID from delete mutation variables first
	var id string
	if inputMap, ok := deleteVars["input"].(map[string]interface{}); ok {
		if idVal, hasID := inputMap["id"]; hasID {
			if idStr, ok := idVal.(string); ok && idStr != "" {
				id = idStr
			}
		}
	}

	// If ID not found in delete variables, try to get it from computed values
	if id == "" {
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

		if idVal, ok := computedVals["id"]; ok {
			id = idVal
		}
	}

	// If still no ID, try to get it from the resource's Id field
	if id == "" {
		if !data.Id.IsNull() && !data.Id.IsUnknown() {
			id = data.Id.ValueString()
		}
	}

	if id == "" {
		diags.AddError("Missing ID", "No ID found in delete_mutation_variables, computed values, or resource ID for deletion.")
		return diags
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

	// Set computed fields to ensure they are never unknown
	data.ComputedCreateOperationVariables = types.StringValue("")
	data.ComputedUpdateOperationVariables = types.StringValue("")
	data.ComputedDeleteOperationVariables = types.MapValueMust(types.StringType, map[string]attr.Value{
		"variables": types.StringValue(string(deleteVarsBytes)),
	})

	return diags
}

func (r *GraphqlMutationResource) readResource(ctx context.Context, data *GraphqlMutationResourceModel, config *graphqlProviderConfig) diag.Diagnostics {
	var diags diag.Diagnostics

	// Prepare read variables
	var queryVariables map[string]interface{}
	readVarsStr, diags := utils.DynamicToJSONString(ctx, data.ReadQueryVariables)
	if diags.HasError() {
		return diags
	}
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
		computedVariables[k] = v // Keep the original value, don't re-marshal
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
	data.ComputedReadOperationVariables = types.MapValueMust(types.StringType, computedVarsMap)

	readVarsBytes, err := json.Marshal(computedVariables)
	if err != nil {
		diags.AddError("Read Variables Error", fmt.Sprintf("Failed to marshal read variables: %s", err))
		return diags
	}

	// Execute read query
	queryResponse, resBytes, diags := r.queryExecuteFramework(ctx, config, data.ReadQuery.ValueString(), string(readVarsBytes), false)
	if diags.HasError() {
		// Check if it's a "not found" error or other deletion indicators
		for _, diag := range diags {
			errorMsg := strings.ToLower(diag.Detail())
			if strings.Contains(errorMsg, "not found") ||
				strings.Contains(errorMsg, "cannot return null for non-nullable field") ||
				strings.Contains(errorMsg, "deleted") ||
				strings.Contains(errorMsg, "does not exist") ||
				strings.Contains(errorMsg, "was deleted") ||
				strings.Contains(errorMsg, "deployment not found") ||
				strings.Contains(errorMsg, "connector was deleted") ||
				strings.Contains(errorMsg, "404") ||
				strings.Contains(errorMsg, "resource not found") {
				tflog.Info(ctx, "Resource not found on remote (transport error indicates deletion), marking for removal", map[string]any{
					"error": diag.Detail(),
				})
				r.markResourceAsDeleted(data)
				return nil // Return nil to indicate success (resource was deleted)
			}
		}
		return diags
	}

	if len(queryResponse.Errors) > 0 {
		// Check if any of the GraphQL errors indicate the resource was deleted or not found
		resourceNotFound := false
		for _, gqlErr := range queryResponse.Errors {
			errorMsg := strings.ToLower(gqlErr.Message)
			tflog.Debug(ctx, "GraphQL error", map[string]any{
				"error": gqlErr.Message,
			})
			if strings.Contains(errorMsg, "deleted") ||
				strings.Contains(errorMsg, "not found") ||
				strings.Contains(errorMsg, "does not exist") ||
				strings.Contains(errorMsg, "was deleted") ||
				strings.Contains(errorMsg, "deployment not found") ||
				strings.Contains(errorMsg, "connector was deleted") ||
				strings.Contains(errorMsg, "resource not found") ||
				strings.Contains(errorMsg, "cannot return null") ||
				strings.Contains(errorMsg, "null for non-nullable") {
				resourceNotFound = true
				break
			}
		}

		if resourceNotFound {
			tflog.Info(ctx, "Resource not found on remote (GraphQL errors indicate deletion), marking for removal")
			r.markResourceAsDeleted(data)
			return nil
		}

		diags.AddError("GraphQL Read Error", fmt.Sprintf("GraphQL server returned errors: %v", queryResponse.Errors))
		return diags
	}

	// Check for null data or empty results
	if dataMap, ok := queryResponse.Data["data"].(map[string]interface{}); ok {
		hasValidData := false
		for key, value := range dataMap {
			if value == nil {
				tflog.Debug(ctx, "Primary data object is null", map[string]any{
					"key": key,
				})
			} else {
				// Check if the value is an empty array or empty object
				if arr, isArray := value.([]interface{}); isArray && len(arr) == 0 {
					tflog.Debug(ctx, "Primary data object is an empty array", map[string]any{
						"key": key,
					})
				} else if obj, isMap := value.(map[string]interface{}); isMap && len(obj) == 0 {
					tflog.Debug(ctx, "Primary data object is an empty object", map[string]any{
						"key": key,
					})
				} else {
					hasValidData = true
				}
			}
		}

		if !hasValidData {
			tflog.Info(ctx, "No valid data found in response, resource may have been deleted")
			r.markResourceAsDeleted(data)
			return nil
		}
	}

	// Check if the entire response data is null or empty
	if queryResponse.Data == nil || len(queryResponse.Data) == 0 {
		tflog.Info(ctx, "Response data is null or empty, resource may have been deleted")
		r.markResourceAsDeleted(data)
		return nil
	}

	// Debug: Log the response data structure
	tflog.Debug(ctx, "Response data keys", map[string]any{
		"keys": utils.GetMapKeys(queryResponse.Data),
	})
	if dataMap, ok := queryResponse.Data["data"].(map[string]interface{}); ok {
		tflog.Debug(ctx, "Data object keys", map[string]any{
			"keys": utils.GetMapKeys(dataMap),
		})
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
		tflog.Debug(ctx, "Using user-defined read_compute_keys for parsing Read response")
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
		tflog.Debug(ctx, "compute_from_read is true. Auto-generating keys from Read response")
		autoGeneratedKeys, err := utils.GenerateKeysFromResponse(ctx, resBytes)
		if err != nil {
			tflog.Warn(ctx, "Failed to auto-generate keys from read response", map[string]any{
				"error": err.Error(),
			})
		} else if len(autoGeneratedKeys) > 0 {
			keysToUse = autoGeneratedKeys
		}
	}

	// Compute mutation variables
	if err := r.computeMutationVariables(string(resBytes), data, keysToUse); err != nil {
		// Check if the error indicates that the resource was not found
		errorMsg := strings.ToLower(err.Error())
		if strings.Contains(errorMsg, "does not exist") ||
			strings.Contains(errorMsg, "not found") ||
			strings.Contains(errorMsg, "path") ||
			strings.Contains(errorMsg, "deleted") {
			tflog.Info(ctx, "Resource paths not found in response, resource may have been deleted", map[string]any{
				"error": err.Error(),
			})
			r.markResourceAsDeleted(data)
			return nil
		}
		diags.AddError("Computation Error", fmt.Sprintf("Unable to compute keys from read response: %s", err))
		return diags
	}

	// Check if we got any meaningful computed values
	if !data.ComputedValues.IsNull() && !data.ComputedValues.IsUnknown() {
		elements := make(map[string]types.String)
		diags.Append(data.ComputedValues.ElementsAs(ctx, &elements, false)...)
		if diags.HasError() {
			return diags
		}

		// If we have no computed values or all values are empty, the resource may be deleted
		if len(elements) == 0 {
			tflog.Info(ctx, "No computed values found, resource may have been deleted")
			r.markResourceAsDeleted(data)
			return nil
		}

		// Check if all computed values are empty strings
		allEmpty := true
		for _, value := range elements {
			if value.ValueString() != "" {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			tflog.Info(ctx, "All computed values are empty, resource may have been deleted")
			r.markResourceAsDeleted(data)
			return nil
		}
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

	// Set computed create operation variables (empty for read operations)
	data.ComputedCreateOperationVariables = types.StringValue("")

	// Set computed update operation variables (empty for read operations)
	data.ComputedUpdateOperationVariables = types.StringValue("")

	// Set computed delete operation variables (empty for read operations)
	data.ComputedDeleteOperationVariables = types.MapValueMust(types.StringType, make(map[string]attr.Value))

	// CRITICAL: Never modify mutation_variables - it should always contain the user's intended configuration
	// The mutation_variables field represents the desired state, not the current state
	// This ensures that Terraform always compares against the user's configuration, not the remote state

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

	// Use the existing query execution framework
	return queryExecuteFramework(ctx, config, query, variablesStr, usePagination)
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
	tflog.Debug(ctx, "prepareUpdatePayload called")

	// Get computed values
	computedValues := make(map[string]string)
	if !data.ComputedValues.IsNull() && !data.ComputedValues.IsUnknown() {
		diags := data.ComputedValues.ElementsAs(ctx, &computedValues, false)
		if diags.HasError() {
			return fmt.Errorf("failed to get computed values: %s", utils.DiagnosticsToString(diags))
		}
	}

	computedID, idExists := computedValues["id"]

	tflog.Debug(ctx, "prepareUpdatePayload called", map[string]any{
		"hasComputedID":     idExists,
		"computedID":        computedID,
		"hasUpdateMutation": !data.UpdateMutation.IsNull() && !data.UpdateMutation.IsUnknown(),
	})

	// Parse the current mutation variables (desired state)
	var desiredMutationVars map[string]interface{}
	if !data.MutationVariables.IsNull() && !data.MutationVariables.IsUnknown() {
		mutVarsStr, diags := utils.DynamicToJSONString(ctx, data.MutationVariables)
		if diags.HasError() {
			return fmt.Errorf("failed to convert mutation_variables to JSON: %s", utils.DiagnosticsToString(diags))
		}
		if mutVarsStr != "" {
			if err := json.Unmarshal([]byte(mutVarsStr), &desiredMutationVars); err != nil {
				return fmt.Errorf("failed to unmarshal mutation_variables: %w", err)
			}
		}
	}

	tflog.Debug(ctx, "Desired mutation variables", map[string]any{
		"desiredMutationVars": desiredMutationVars,
	})

	// Check if the mutation_variables already contains a patch structure
	if inputObj, ok := desiredMutationVars["input"].(map[string]interface{}); ok {
		if _, hasPatch := inputObj["patch"]; hasPatch {
			tflog.Debug(ctx, "Mutation variables already contain patch structure, using as-is")
			// Already in patch format, no conversion needed
			// Still need to set computed update variables to avoid unknown state
			updateVarsBytes, err := json.Marshal(desiredMutationVars)
			if err != nil {
				return fmt.Errorf("failed to marshal existing mutation variables: %w", err)
			}
			data.ComputedUpdateOperationVariables = types.StringValue(string(updateVarsBytes))
			return nil
		}
	}

	// Check if the mutation_variables already contains the correct structure for the API
	// If the user has provided the complete structure, use it as-is
	if inputObj, ok := desiredMutationVars["input"].(map[string]interface{}); ok {
		// If the input already has an ID and the structure looks complete, use it as-is
		if _, hasID := inputObj["id"]; hasID {
			tflog.Debug(ctx, "Mutation variables already contain complete input structure, using as-is")
			updateVarsBytes, err := json.Marshal(desiredMutationVars)
			if err != nil {
				return fmt.Errorf("failed to marshal existing mutation variables: %w", err)
			}
			data.ComputedUpdateOperationVariables = types.StringValue(string(updateVarsBytes))
			return nil
		}
	}

	// Only create patch operations if we have both an ID and an update mutation
	if idExists && computedID != "" && !data.UpdateMutation.IsNull() && !data.UpdateMutation.IsUnknown() {
		tflog.Debug(ctx, "Creating update format with patch", map[string]any{
			"computedID": computedID,
		})

		// Extract the desired fields from the mutation_variables
		var desiredFields map[string]interface{}
		if inputObj, ok := desiredMutationVars["input"].(map[string]interface{}); ok {
			desiredFields = inputObj
		} else {
			desiredFields = desiredMutationVars
		}

		// Get the current remote state from the query response
		var currentRemoteState map[string]interface{}
		if !data.QueryResponse.IsNull() && !data.QueryResponse.IsUnknown() {
			queryResponseStr := data.QueryResponse.ValueString()
			if queryResponseStr != "" {
				var queryResponse map[string]interface{}
				if err := json.Unmarshal([]byte(queryResponseStr), &queryResponse); err != nil {
					tflog.Debug(ctx, "Failed to parse query response for current state comparison", map[string]any{
						"error": err.Error(),
					})
				} else {
					// Extract current state from the query response
					currentRemoteState = r.extractCurrentStateFromQueryResponse(ctx, queryResponse)
					tflog.Debug(ctx, "Extracted current remote state", map[string]any{
						"currentRemoteState": currentRemoteState,
					})
				}
			}
		}

		// Compare desired state with current remote state
		changedFields := r.findChangedFields(ctx, desiredFields, currentRemoteState)
		updateNeeded := r.isUpdateNeeded(ctx, desiredFields, currentRemoteState)

		tflog.Debug(ctx, "Changed fields (desired vs remote)", map[string]any{
			"changedFields":      changedFields,
			"desiredFields":      desiredFields,
			"currentRemoteState": currentRemoteState,
			"updateNeeded":       updateNeeded,
		})

		// Always create update variables, even if no changes detected
		// This ensures the computed field is never unknown
		if updateNeeded && len(changedFields) > 0 {
			// Create the update format with patch - include the ID in the input
			updateVariables := map[string]interface{}{
				"input": map[string]interface{}{
					"id":    computedID,
					"patch": changedFields,
				},
			}

			// Convert to JSON and store in computed update operation variables
			updateVarsBytes, err := json.Marshal(updateVariables)
			if err != nil {
				return fmt.Errorf("failed to marshal update variables: %w", err)
			}

			data.ComputedUpdateOperationVariables = types.StringValue(string(updateVarsBytes))
			tflog.Debug(ctx, "Set ComputedUpdateOperationVariables with changes", map[string]any{
				"updateVariables": string(updateVarsBytes),
			})
		} else {
			// No changes detected between desired and remote state
			// Still create update variables to ensure consistency
			updateVariables := map[string]interface{}{
				"input": map[string]interface{}{
					"id": computedID,
				},
			}

			// Add any non-input fields from desired mutation variables
			for k, v := range desiredMutationVars {
				if k != "input" {
					updateVariables[k] = v
				}
			}

			updateVarsBytes, err := json.Marshal(updateVariables)
			if err != nil {
				return fmt.Errorf("failed to marshal update variables: %w", err)
			}

			data.ComputedUpdateOperationVariables = types.StringValue(string(updateVarsBytes))
			tflog.Debug(ctx, "Set ComputedUpdateOperationVariables without changes", map[string]any{
				"updateVariables": string(updateVarsBytes),
			})
		}
	} else {
		tflog.Debug(ctx, "No computed ID or update mutation available, will use original mutation variables", map[string]any{
			"hasComputedID":     idExists,
			"hasUpdateMutation": !data.UpdateMutation.IsNull() && !data.UpdateMutation.IsUnknown(),
		})

		// Set computed update variables to the original mutation variables to avoid unknown state
		updateVarsBytes, err := json.Marshal(desiredMutationVars)
		if err != nil {
			return fmt.Errorf("failed to marshal original mutation variables: %w", err)
		}
		data.ComputedUpdateOperationVariables = types.StringValue(string(updateVarsBytes))
	}

	return nil
}

// extractCurrentStateFromQueryResponse extracts the current state from the GraphQL query response
func (r *GraphqlMutationResource) extractCurrentStateFromQueryResponse(ctx context.Context, queryResponse map[string]interface{}) map[string]interface{} {
	extractor := &utils.ResponseExtraction{}
	return extractor.ExtractCurrentStateFromQueryResponse(ctx, queryResponse)
}

// findChangedFields compares desired state with current remote state and returns only the changed ones
func (r *GraphqlMutationResource) findChangedFields(ctx context.Context, desired, current map[string]interface{}) map[string]interface{} {
	changedFields := make(map[string]interface{})

	// Fields that should not be updated
	excludedFields := map[string]bool{
		"type": true, // Connector type cannot be changed after creation
	}

	// Use the state comparison helper
	comparison := utils.NewStateComparison()

	// Extract fields from desired state, handling patch structure
	desiredFields := desired
	if patch, hasPatch := desired["patch"].(map[string]interface{}); hasPatch {
		// If desired state has a patch structure, use the patch fields for comparison
		desiredFields = patch
		tflog.Debug(ctx, "Extracted fields from patch structure", map[string]any{
			"patchFields": desiredFields,
		})
	}

	// Only include fields that have actually changed
	for key, desiredValue := range desiredFields {
		if excludedFields[key] {
			continue // Skip excluded fields
		}

		currentValue, exists := current[key]
		if !exists {
			// Field doesn't exist in current state, it's a new field
			changedFields[key] = desiredValue
			tflog.Debug(ctx, "Field added", map[string]any{
				"field":        key,
				"desiredValue": desiredValue,
			})
			continue
		}

		// Compare values (handle different types)
		if !comparison.ValuesEqual(desiredValue, currentValue) {
			changedFields[key] = desiredValue
			tflog.Debug(ctx, "Field changed", map[string]any{
				"field":        key,
				"desiredValue": desiredValue,
				"currentValue": currentValue,
			})
		} else {
			tflog.Debug(ctx, "Field unchanged", map[string]any{
				"field": key,
			})
		}
	}

	// Don't check for fields that were removed from desired state
	// If a field exists in remote but not in desired config, we ignore it
	// This prevents the provider from trying to "remove" fields that the user didn't configure

	return changedFields
}

// isUpdateNeeded determines if an update operation is actually required
func (r *GraphqlMutationResource) isUpdateNeeded(ctx context.Context, desired, current map[string]interface{}) bool {
	changedFields := r.findChangedFields(ctx, desired, current)

	tflog.Debug(ctx, "Update need assessment", map[string]any{
		"hasChanges":    len(changedFields) > 0,
		"changedFields": changedFields,
		"desiredFields": desired,
		"currentFields": current,
	})

	return len(changedFields) > 0
}

// generateKeysFromResponse uses the helper to generate keys from the response
func (r *GraphqlMutationResource) generateKeysFromResponse(ctx context.Context, responseBytes []byte) (map[string]interface{}, error) {
	return utils.GenerateKeysFromResponse(ctx, responseBytes)
}

// flattenRecursive is now in utils
// detectFieldChanges is now in utils.StateComparison.DetectFieldChanges
// valuesEqual is now in utils.StateComparison.ValuesEqual
// mapsEqual is now in utils.StateComparison.MapsEqual
// slicesEqual is now in utils.StateComparison.SlicesEqual
// extractCurrentStateFromQueryResponse is now in utils.StateComparison.ExtractCurrentStateFromQueryResponse
// parseGraphQLQueryFields is now in utils.ParseGraphQLQueryFields
// genericExtractCurrentState is now in utils.StateComparison.GenericExtractCurrentState
// hasConfigurationChanges is now in utils.StateComparison.HasConfigurationChanges
// diagnosticsToString is now in utils.DiagnosticsToString

// markResourceAsDeleted sets all the necessary fields to indicate the resource has been deleted
func (r *GraphqlMutationResource) markResourceAsDeleted(data *GraphqlMutationResourceModel) {
	data.Id = types.StringNull()
	data.ComputedValues = types.MapValueMust(types.StringType, make(map[string]attr.Value))
	data.ComputedReadOperationVariables = types.MapValueMust(types.StringType, make(map[string]attr.Value))
	data.ComputedUpdateOperationVariables = types.StringValue("")
	data.ComputedCreateOperationVariables = types.StringValue("")
	data.ComputedDeleteOperationVariables = types.MapValueMust(types.StringType, make(map[string]attr.Value))
	data.QueryResponse = types.StringValue("")
	data.ExistingHash = types.StringValue("")
	data.CurrentRemoteState = types.StringValue("")
}
