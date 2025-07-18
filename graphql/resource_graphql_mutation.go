package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
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
				Description:  "A JSON-encoded string representing the variables for the create and update operations.",
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
			"read_compute_keys": {
				Type: schema.TypeMap,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional:    true,
				Description: "A map of keys to paths for extracting values from the read query response. If not provided, defaults to compute_mutation_keys.",
			},
			"compute_from_read": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "If true, the provider will automatically generate compute keys from the read query response, saving the need to define read_compute_keys.",
			},
			"wrap_update_in_patch": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
				Description: "If true, for update operations, the provider will wrap the changed fields from mutation_variables inside a 'patch' object and inject the computed 'id'.",
			},
			"create_only_fields": {
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Optional:    true,
				Description: "A list of paths to fields in mutation_variables that should be ignored during update operations.",
			},
			"computed_values": {
				Type: schema.TypeMap,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Computed:    true,
				Description: "A map of values computed from the API response, used to populate variables for subsequent operations.",
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
	forceReplace := d.Get("force_replace").(bool)
	if forceReplace {
		deleteDiags := executeDeleteHook(ctx, d, m)
		if deleteDiags.HasError() {
			return deleteDiags
		}

		_, createDiags := executeCreateHook(ctx, d, m)
		if createDiags.HasError() {
			return createDiags
		}
	} else {
		if err := prepareUpdatePayload(d); err != nil {
			return diag.FromErr(err)
		}

		_, updateDiags := executeUpdateHook(ctx, d, m)
		if updateDiags.HasError() {
			return updateDiags
		}
	}

	return resourceGraphqlRead(ctx, d, m)
}

func resourceGraphqlRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	var queryResponse *GqlQueryResponse
	var resBytes []byte
	var err error

	err = resource.RetryContext(ctx, d.Timeout(schema.TimeoutRead), func() *resource.RetryError {
		var queryVariables map[string]interface{}
		readVarsStr := d.Get("read_query_variables").(string)
		if readVarsStr != "" {
			if err := json.Unmarshal([]byte(readVarsStr), &queryVariables); err != nil {
				return resource.NonRetryableError(fmt.Errorf("failed to unmarshal read_query_variables: %w", err))
			}
		} else {
			queryVariables = make(map[string]interface{})
		}

		computedValues := d.Get("computed_values").(map[string]interface{})
		computedVariables := make(map[string]interface{})

		for k, v := range queryVariables {
			if str, ok := v.(string); ok {
				computedVariables[k] = str
			} else {
				bytes, err := json.Marshal(v)
				if err != nil {
					return resource.NonRetryableError(fmt.Errorf("failed to marshal value for key %s in read_query_variables: %w", k, err))
				}
				computedVariables[k] = string(bytes)
			}
		}

		for k, v := range computedValues {
			computedVariables[k] = v
		}

		if err := d.Set("computed_read_operation_variables", computedVariables); err != nil {
			return resource.NonRetryableError(fmt.Errorf("unable to set computed_read_operation_variables: %w", err))
		}

		queryResponse, resBytes, err = queryExecute(ctx, d, m, "read_query", "computed_read_operation_variables", false)
		if err != nil {
			if strings.Contains(err.Error(), "Not Found") || strings.Contains(err.Error(), "Cannot return null for non-nullable field") {
				log.Printf("[WARN] Resource not found on remote, removing from state.")
				d.SetId("")
				return nil
			}
			log.Printf("[WARN] Retrying read due to transient error: %s", err)
			return resource.RetryableError(err)
		}

		if len(queryResponse.Errors) > 0 {
			log.Printf("[WARN] Retrying read due to GraphQL errors in response: %v", queryResponse.Errors)
			return resource.RetryableError(fmt.Errorf("graphql server returned errors: %v", queryResponse.Errors))
		}

		if dataMap, ok := queryResponse.Data["data"].(map[string]interface{}); ok {
			for key, value := range dataMap {
				if value == nil {
					log.Printf("[WARN] Retrying read because primary data object '%s' is null.", key)
					return resource.RetryableError(fmt.Errorf("resource data is not yet available ('%s' is null)", key))
				}
				break
			}
		}

		return nil
	})

	if err != nil {
		return diag.FromErr(fmt.Errorf("failed to read resource after multiple retries: %w", err))
	}

	if d.Id() == "" {
		return nil
	}

	if err := d.Set("query_response", string(resBytes)); err != nil {
		return diag.FromErr(err)
	}

	keysToUse := d.Get("compute_mutation_keys").(map[string]interface{})
	if readKeys, ok := d.GetOk("read_compute_keys"); ok && len(readKeys.(map[string]interface{})) > 0 {
		log.Printf("[DEBUG] Using user-defined read_compute_keys for parsing Read response.")
		keysToUse = readKeys.(map[string]interface{})
	} else if d.Get("compute_from_read").(bool) {
		log.Printf("[DEBUG] compute_from_read is true. Auto-generating keys from Read response.")
		autoGeneratedKeys, err := generateKeysFromResponse(resBytes)
		if err != nil {
			log.Printf("[WARN] Failed to auto-generate keys from read response: %s", err)
		} else if len(autoGeneratedKeys) > 0 {
			keysToUse = autoGeneratedKeys
		}
	}

	if err := computeMutationVariables(resBytes, d, keysToUse); err != nil {
		return diag.FromErr(fmt.Errorf("unable to compute keys from read response: %w", err))
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

	if len(queryResponse.Errors) > 0 {
		var diags diag.Diagnostics
		for _, gqlErr := range queryResponse.Errors {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "GraphQL server error",
				Detail:   gqlErr.Message,
			})
		}
		return nil, diags
	}

	existingHash := hash(resBytes)
	if err := d.Set("existing_hash", fmt.Sprint(existingHash)); err != nil {
		return nil, diag.FromErr(err)
	}

	keysToUse := d.Get("compute_mutation_keys").(map[string]interface{})
	if err := computeMutationVariables(resBytes, d, keysToUse); err != nil {
		return nil, diag.FromErr(fmt.Errorf("unable to compute keys from create response: %w", err))
	}

	return resBytes, nil
}

func executeUpdateHook(ctx context.Context, d *schema.ResourceData, m interface{}) ([]byte, diag.Diagnostics) {
	queryResponse, resBytes, err := queryExecute(ctx, d, m, "update_mutation", "computed_update_operation_variables", true)
	if err != nil {
		return nil, diag.FromErr(fmt.Errorf("unable to execute update query: %w", err))
	}

	if len(queryResponse.Errors) > 0 {
		var diags diag.Diagnostics
		for _, gqlErr := range queryResponse.Errors {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "GraphQL server error",
				Detail:   gqlErr.Message,
			})
		}
		return nil, diags
	}

	return resBytes, nil
}

func executeDeleteHook(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	queryResponse, _, err := queryExecute(ctx, d, m, "delete_mutation", "computed_delete_operation_variables", false)
	if err != nil {
		return diag.FromErr(fmt.Errorf("unable to execute delete query: %w", err))
	}

	if len(queryResponse.Errors) > 0 {
		var diags diag.Diagnostics
		for _, gqlErr := range queryResponse.Errors {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "GraphQL server error",
				Detail:   gqlErr.Message,
			})
		}
		return diags
	}

	return nil
}

func computeMutationVariables(queryResponseBytes []byte, d *schema.ResourceData, dataKeys map[string]interface{}) error {
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

	searchRoot := robj
	if data, ok := robj["data"].(map[string]interface{}); ok {
		log.Printf("[DEBUG] Found 'data' key in response, using it as the root for key computation.")
		searchRoot = data
	}

	mvks := make(map[string]string)
	log.Printf("[DEBUG] Starting manual key computation. Keys to find: %v", dataKeys)
	for key, path := range dataKeys {
		pathStr, ok := path.(string)
		if !ok {
			log.Printf("[WARN] Path for key '%s' is not a string, skipping", key)
			continue
		}

		log.Printf("[DEBUG] Searching for key '%s' at path '%s'", key, pathStr)
		pathParts := strings.Split(pathStr, ".")
		var currentData interface{} = searchRoot
		var found bool = true

		for i, part := range pathParts {
			log.Printf("[DEBUG] Traversing part %d: '%s'. Current data type: %T", i, part, currentData)
			var nextData interface{}
			var traversalOk bool

			if currentMap, isMap := currentData.(map[string]interface{}); isMap {
				nextData, traversalOk = currentMap[part]
				if !traversalOk {
					log.Printf("[WARN] Path part '%s' not found in current map level.", part)
				}
			} else if currentSlice, isSlice := currentData.([]interface{}); isSlice {
				index, err := strconv.Atoi(part)
				if err != nil {
					log.Printf("[WARN] Path part '%s' is not a valid integer index for the array.", part)
					traversalOk = false
				} else if index < 0 || index >= len(currentSlice) {
					log.Printf("[WARN] Index %d is out of bounds for the array (length %d).", index, len(currentSlice))
					traversalOk = false
				} else {
					nextData = currentSlice[index]
					traversalOk = true
				}
			} else {
				log.Printf("[WARN] Failed to traverse path. Part '%s' requires a map or array, but current data is not. Type is %T.", part, currentData)
				traversalOk = false
			}

			if !traversalOk {
				found = false
				break
			}

			if i == len(pathParts)-1 {
				if valStr, isStr := nextData.(string); isStr {
					log.Printf("[DEBUG] Found string value for key '%s': %s", key, valStr)
					mvks[key] = valStr
				} else if valBool, isBool := nextData.(bool); isBool {
					log.Printf("[DEBUG] Found boolean value for key '%s': %t", key, valBool)
					mvks[key] = fmt.Sprintf("%t", valBool)
				} else {
					log.Printf("[WARN] Value for key '%s' at path '%s' is not a string or bool. Type is %T.", key, pathStr, nextData)
					found = false
				}
			} else {
				currentData = nextData
			}
		}
		if !found {
			log.Printf("[ERROR] Could not find or access value for key '%s' at path '%s'", key, pathStr)
		}
	}

	if len(mvks) == 0 && len(dataKeys) > 0 {
		return fmt.Errorf("failed to compute any of the specified keys from the API response")
	} else if len(mvks) != len(dataKeys) {
		log.Printf("[WARN] Could not compute all mutation keys. Found %d of %d. This may be acceptable if some keys are optional.", len(mvks), len(dataKeys))
	}

	if err := d.Set("computed_values", mvks); err != nil {
		return fmt.Errorf("failed to set computed_values: %w", err)
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

	computedID, idExists := mvks["id"]

	// Populate Delete Variables
	dvks := make(map[string]interface{})
	for k, v := range deleteMutationVariables {
		dvks[k] = v
	}
	if idExists {
		if input, ok := dvks["input"].(map[string]interface{}); ok {
			log.Printf("[DEBUG] Injecting computed 'id' into existing 'input' object for delete.")
			input["id"] = computedID
			dvks["input"] = input
		} else {
			log.Printf("[DEBUG] No 'input' object found for delete, creating one for computed 'id'.")
			dvks["input"] = map[string]interface{}{"id": computedID}
		}
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

	return nil
}

func prepareUpdatePayload(d *schema.ResourceData) error {
	var mutationVariables map[string]interface{}
	mutVarsStr := d.Get("mutation_variables").(string)
	if mutVarsStr != "" {
		if err := json.Unmarshal([]byte(mutVarsStr), &mutationVariables); err != nil {
			return fmt.Errorf("failed to unmarshal mutation_variables: %w", err)
		}
	} else {
		mutationVariables = make(map[string]interface{})
	}

	computedValues := d.Get("computed_values").(map[string]interface{})
	computedID, idExists := computedValues["id"]

	uvks := make(map[string]interface{})
	if d.HasChange("mutation_variables") {
		log.Printf("[DEBUG] Change detected in 'mutation_variables', preparing update payload.")
		old, new := d.GetChange("mutation_variables")
		var oldVars, newVars map[string]interface{}
		if err := json.Unmarshal([]byte(old.(string)), &oldVars); err != nil {
			return fmt.Errorf("failed to unmarshal old mutation_variables: %w", err)
		}
		if err := json.Unmarshal([]byte(new.(string)), &newVars); err != nil {
			return fmt.Errorf("failed to unmarshal new mutation_variables: %w", err)
		}

		if newInput, ok := newVars["input"].(map[string]interface{}); ok {
			if oldInput, ok := oldVars["input"].(map[string]interface{}); ok {
				diff := deepDiff(oldInput, newInput)
				uvks["input"] = diff
			} else {
				uvks["input"] = newInput
			}
		}
	}

	if d.Get("wrap_update_in_patch").(bool) && idExists {
		log.Printf("[DEBUG] 'wrap_update_in_patch' is true. Restructuring update variables.")
		if originalInput, ok := uvks["input"].(map[string]interface{}); ok {
			newInput := make(map[string]interface{})
			newInput["id"] = computedID
			newInput["patch"] = originalInput
			uvks["input"] = newInput
		} else {
			log.Printf("[WARN] 'wrap_update_in_patch' is true, but there are no changes to wrap.")
			uvks["input"] = map[string]interface{}{"id": computedID, "patch": map[string]interface{}{}}
		}
	} else if idExists {
		if input, ok := uvks["input"].(map[string]interface{}); ok {
			log.Printf("[DEBUG] Injecting computed 'id' into 'input' object for update.")
			input["id"] = computedID
			uvks["input"] = input
		} else {
			log.Printf("[DEBUG] No changes detected, but injecting 'id' for potential use.")
			uvks["input"] = map[string]interface{}{"id": computedID}
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

func generateKeysFromResponse(responseBytes []byte) (map[string]interface{}, error) {
	var robj map[string]interface{}
	if err := json.Unmarshal(responseBytes, &robj); err != nil {
		return nil, err
	}

	data, ok := robj["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("response JSON does not contain a 'data' object")
	}

	generatedKeys := make(map[string]interface{})
	flattenRecursive("", data, generatedKeys)
	return generatedKeys, nil
}

func flattenRecursive(prefix string, data interface{}, keyMap map[string]interface{}) {
	switch v := data.(type) {
	case map[string]interface{}:
		for key, val := range v {
			newPrefix := key
			if prefix != "" {
				newPrefix = prefix + "." + key
			}
			flattenRecursive(newPrefix, val, keyMap)
		}
	case []interface{}:
		for i, val := range v {
			newPrefix := fmt.Sprintf("%s.%d", prefix, i)
			flattenRecursive(newPrefix, val, keyMap)
		}
	default:
		pathParts := strings.Split(prefix, ".")
		leafKey := pathParts[len(pathParts)-1]
		if _, exists := keyMap[leafKey]; !exists {
			keyMap[leafKey] = prefix
			log.Printf("[DEBUG] Auto-generated key: '%s' -> '%s'", leafKey, prefix)
		}
	}
}

func deleteNestedKey(data map[string]interface{}, path []string) {
	if len(path) == 0 {
		return
	}

	current := data
	for i, key := range path {
		if i == len(path)-1 {
			delete(current, key)
			return
		}

		next, ok := current[key].(map[string]interface{})
		if !ok {
			return
		}
		current = next
	}
}

func deepDiff(old, new map[string]interface{}) map[string]interface{} {
	diff := make(map[string]interface{})

	for key, newVal := range new {
		oldVal, ok := old[key]
		if !ok {
			diff[key] = newVal
			continue
		}

		if !reflect.DeepEqual(oldVal, newVal) {
			newMap, newIsMap := newVal.(map[string]interface{})
			oldMap, oldIsMap := oldVal.(map[string]interface{})

			if newIsMap && oldIsMap {
				nestedDiff := deepDiff(oldMap, newMap)
				if len(nestedDiff) > 0 {
					diff[key] = nestedDiff
				}
			} else {
				diff[key] = newVal
			}
		}
	}
	return diff
}
