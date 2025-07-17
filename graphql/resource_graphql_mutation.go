package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/validation"
)

func resourceGraphqlMutation() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"read_query": {
				Type:     schema.TypeString,
				Required: true,
			},
			"create_mutation": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"delete_mutation": {
				Type:     schema.TypeString,
				Required: true,
			},
			"update_mutation": {
				Type:     schema.TypeString,
				Required: true,
			},
			"mutation_variables": {
				Type:         schema.TypeString,
				Required:     true,
				ValidateFunc: validation.StringIsJSON,
				Description:  "A JSON-encoded string representing the variables for the create and update mutations.",
			},
			"read_query_variables": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validation.StringIsJSON,
				Description:  "A JSON-encoded string representing the variables for the read query.",
			},
			"delete_mutation_variables": {
				Type:         schema.TypeString,
				Optional:     true,
				ValidateFunc: validation.StringIsJSON,
				Description:  "A JSON-encoded string representing the variables for the delete mutation.",
			},
			"compute_mutation_keys": {
				Type: schema.TypeMap,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Required: true,
			},
			"compute_from_create": {
				Type:     schema.TypeBool,
				Optional: true,
			},
			"force_replace": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "If true, all updates will first delete the resource and recreate it.",
			},
			"computed_read_operation_variables": {
				Type: schema.TypeMap,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Computed: true,
			},
			"computed_update_operation_variables": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"computed_create_operation_variables": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"computed_delete_operation_variables": {
				Type: schema.TypeMap,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Computed: true,
			},
			"query_response": {
				Type:        schema.TypeString,
				Description: "The raw body of the HTTP response from the last read of the object.",
				Computed:    true,
			},
			"query_response_input_key_map": {
				Type: schema.TypeMap,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Computed: true,
			},
			"enable_remote_state_verification": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "A pre v2.4.0 backward-compatibility flag. Set to `false` to disable resource remote state verification during reads.",
			},
			"existing_hash": {
				Type:        schema.TypeString,
				Description: "Represents the state of existence of a mutation in order to support intelligent updates.",
				Computed:    true,
			},
		},
		CreateContext: resourceGraphqlMutationCreate,
		UpdateContext: resourceGraphqlMutationUpdate,
		ReadContext:   resourceGraphqlRead,
		DeleteContext: resourceGraphqlMutationDelete,
		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
	}
}

func resourceGraphqlMutationCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var resBytes []byte
	var errDiags diag.Diagnostics

	if resBytes, errDiags = executeCreateHook(ctx, d, m); errDiags.HasError() {
		return errDiags
	}

	objID := hash(resBytes)
	d.SetId(fmt.Sprint(objID))

	return resourceGraphqlRead(ctx, d, m)
}

func resourceGraphqlMutationUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var errDiags diag.Diagnostics

	forceReplace := d.Get("force_replace").(bool)
	if forceReplace {
		if errDiags = executeDeleteHook(ctx, d, m); errDiags.HasError() {
			return errDiags
		}

		if _, errDiags = executeCreateHook(ctx, d, m); errDiags.HasError() {
			return errDiags
		}
	} else {
		if _, errDiags = executeUpdateHook(ctx, d, m); errDiags.HasError() {
			return errDiags
		}
	}

	return resourceGraphqlRead(ctx, d, m)
}

func resourceGraphqlRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var queryVariables map[string]interface{}
	readVarsStr := d.Get("read_query_variables").(string)
	if readVarsStr != "" {
		if err := json.Unmarshal([]byte(readVarsStr), &queryVariables); err != nil {
			return diag.FromErr(fmt.Errorf("failed to unmarshal read_query_variables: %w", err))
		}
	} else {
		queryVariables = make(map[string]interface{})
	}

	// This computed field is a map[string]string, so we need to handle type conversion.
	computedVariables := make(map[string]interface{})
	for k, v := range queryVariables {
		if str, ok := v.(string); ok {
			computedVariables[k] = str
		} else {
			bytes, err := json.Marshal(v)
			if err != nil {
				return diag.FromErr(fmt.Errorf("failed to marshal value for key %s in read_query_variables: %w", k, err))
			}
			computedVariables[k] = string(bytes)
		}
	}

	if err := d.Set("computed_read_operation_variables", computedVariables); err != nil {
		return diag.FromErr(fmt.Errorf("unable to set computed_read_operation_variables: %w", err))
	}

	queryResponse, resBytes, err := queryExecute(ctx, d, m, "read_query", "computed_read_operation_variables", false)
	if err != nil {
		if strings.Contains(err.Error(), "Not Found") || strings.Contains(err.Error(), "Cannot return null for non-nullable field") {
			log.Printf("[WARN] Resource not found on remote, removing from state.")
			d.SetId("")
			return nil
		}
		return diag.FromErr(fmt.Errorf("unable to execute read query: %w", err))
	}

	if queryErrors := queryResponse.ProcessErrors(); queryErrors.HasError() {
		return *queryErrors
	}

	if err := d.Set("query_response", string(resBytes)); err != nil {
		return diag.FromErr(err)
	}

	computeFromCreate := d.Get("compute_from_create").(bool)
	if !computeFromCreate {
		if err := computeMutationVariables(resBytes, d); err != nil {
			return diag.FromErr(fmt.Errorf("unable to compute keys: %w", err))
		}
	}

	return nil
}

func resourceGraphqlMutationDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	if errDiags := executeDeleteHook(ctx, d, m); errDiags.HasError() {
		return errDiags
	}
	d.SetId("")
	return nil
}

func executeCreateHook(ctx context.Context, d *schema.ResourceData, m interface{}) ([]byte, diag.Diagnostics) {
	varsStr := d.Get("mutation_variables").(string)
	if err := d.Set("computed_create_operation_variables", varsStr); err != nil {
		return nil, diag.FromErr(fmt.Errorf("unable to set computed_create_operation_variables: %w", err))
	}

	queryResponse, resBytes, err := queryExecute(ctx, d, m, "create_mutation", "computed_create_operation_variables", true)
	if err != nil {
		return nil, diag.FromErr(fmt.Errorf("unable to execute create query: %w", err))
	}

	if queryErrors := queryResponse.ProcessErrors(); queryErrors.HasError() {
		return nil, *queryErrors
	}

	existingHash := hash(resBytes)
	if err := d.Set("existing_hash", fmt.Sprint(existingHash)); err != nil {
		return nil, diag.FromErr(err)
	}

	computeFromCreate := d.Get("compute_from_create").(bool)
	if computeFromCreate {
		if err := computeMutationVariables(resBytes, d); err != nil {
			return nil, diag.FromErr(fmt.Errorf("unable to compute keys: %w", err))
		}
	}
	return resBytes, nil
}

func executeUpdateHook(ctx context.Context, d *schema.ResourceData, m interface{}) ([]byte, diag.Diagnostics) {
	queryResponse, resBytes, err := queryExecute(ctx, d, m, "update_mutation", "computed_update_operation_variables", true)
	if err != nil {
		return nil, diag.FromErr(fmt.Errorf("unable to execute update query: %w", err))
	}

	if queryErrors := queryResponse.ProcessErrors(); queryErrors.HasError() {
		return nil, *queryErrors
	}
	return resBytes, nil
}

func executeDeleteHook(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	queryResponse, _, err := queryExecute(ctx, d, m, "delete_mutation", "computed_delete_operation_variables", false)
	if err != nil {
		return diag.FromErr(fmt.Errorf("unable to execute delete query: %w", err))
	}

	if queryErrors := queryResponse.ProcessErrors(); queryErrors.HasError() {
		return *queryErrors
	}

	return nil
}

func computeMutationVariables(queryResponseBytes []byte, d *schema.ResourceData) error {
	dataKeys := d.Get("compute_mutation_keys").(map[string]interface{})

	var mutationVariables map[string]interface{}
	mutVarsStr := d.Get("mutation_variables").(string)
	if mutVarsStr != "" {
		if err := json.Unmarshal([]byte(mutVarsStr), &mutationVariables); err != nil {
			return fmt.Errorf("failed to unmarshal mutation_variables: %w", err)
		}
	} else {
		mutationVariables = make(map[string]interface{})
	}

	var readQueryVariables map[string]interface{}
	readVarsStr := d.Get("read_query_variables").(string)
	if readVarsStr != "" {
		if err := json.Unmarshal([]byte(readVarsStr), &readQueryVariables); err != nil {
			return fmt.Errorf("failed to unmarshal read_query_variables: %w", err)
		}
	} else {
		readQueryVariables = make(map[string]interface{})
	}

	var deleteMutationVariables map[string]interface{}
	delVarsStr := d.Get("delete_mutation_variables").(string)
	if delVarsStr != "" {
		if err := json.Unmarshal([]byte(delVarsStr), &deleteMutationVariables); err != nil {
			return fmt.Errorf("failed to unmarshal delete_mutation_variables: %w", err)
		}
	} else {
		deleteMutationVariables = make(map[string]interface{})
	}

	var robj map[string]interface{}
	if err := json.Unmarshal(queryResponseBytes, &robj); err != nil {
		return err
	}

	mvks, err := computeMutationVariableKeys(dataKeys, robj)
	if err != nil {
		log.Printf("[ERROR] Unable to compute mutation variable keys: %s ", err)
		mvks = make(map[string]string)
	}

	rvks := make(map[string]string)
	for k, v := range readQueryVariables {
		if str, ok := v.(string); ok {
			rvks[k] = str
		} else {
			bytes, err := json.Marshal(v)
			if err != nil {
				return err
			}
			rvks[k] = string(bytes)
		}
	}
	for k, v := range mvks {
		rvks[k] = v
	}
	if err := d.Set("computed_read_operation_variables", rvks); err != nil {
		return err
	}

	dvks := make(map[string]interface{})
	for k, v := range deleteMutationVariables {
		dvks[k] = v
	}
	for k, v := range mvks {
		dvks[k] = v
	}
	finalDvks := make(map[string]string)
	for k, v := range dvks {
		if str, ok := v.(string); ok {
			finalDvks[k] = str
		} else {
			bytes, err := json.Marshal(v)
			if err != nil {
				return err
			}
			finalDvks[k] = string(bytes)
		}
	}
	if err := d.Set("computed_delete_operation_variables", finalDvks); err != nil {
		return err
	}

	uvks := make(map[string]interface{})
	for k, v := range mutationVariables {
		uvks[k] = v
	}
	if input, ok := uvks["input"].(map[string]interface{}); ok {
		log.Printf("[DEBUG] Merging computed keys into 'input' object for update.")
		for k, v := range mvks {
			input[k] = v
		}
		uvks["input"] = input
	} else {
		log.Printf("[DEBUG] No 'input' object found, merging computed keys at top level for update.")
		for k, v := range mvks {
			uvks[k] = v
		}
	}
	updateVarsBytes, err := json.Marshal(uvks)
	if err != nil {
		return fmt.Errorf("failed to marshal update variables: %w", err)
	}
	if err := d.Set("computed_update_operation_variables", string(updateVarsBytes)); err != nil {
		return err
	}

	return nil
}
