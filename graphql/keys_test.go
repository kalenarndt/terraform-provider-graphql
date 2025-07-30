package graphql

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

var datablob = `{"data": {"someField": "someValue", "items": ["itemValueOne", "itemValueTwo"], "otherItems": [{"field1": "value1", "field2": "value2"}, {"field1": "value3", "field2": "value4"}, {"nestedList": ["nestedListValue"]}]}}`

var complexTest = `{"data": {"virtualHost": {"id": "vhostID","customer": {"id": "customerIDValue"},"dataProtectionPolicy": { "id": "dataProtectionPolicyValue"},"networkInterfaceList": [ {	"id": "fake",	"network": {	 "id": "networkIDValue"	} }],"tier": { "id": "tierIDValue"}}}}`

func TestComputeMutationVariableKeys(t *testing.T) {
	cases := []struct {
		body             string
		computeKeys      map[string]interface{}
		expectedValues   map[string]interface{}
		expectedErrorMsg string
	}{
		{
			body:           `{"data": {"todo": {"id": "computed_id", "otherComputedValue": "computed_value_two"}}}`,
			computeKeys:    map[string]interface{}{"id_key": "todo.id", "other_computed_value": "todo.otherComputedValue"},
			expectedValues: map[string]interface{}{"id_key": "computed_id", "other_computed_value": "computed_value_two"},
		},
		{
			body:           `{"data": {"todos": [{"id": "computed_id"}, {"id": "second_id"}]}}`,
			computeKeys:    map[string]interface{}{"id_key": "todos.1.id"},
			expectedValues: map[string]interface{}{"id_key": "second_id"},
		},
		{
			body:           `{"data": {"todos": ["stringval"]}}`,
			computeKeys:    map[string]interface{}{"id_key": "todos.0"},
			expectedValues: map[string]interface{}{"id_key": "stringval"},
		},
		{
			body:           `{"data": {"todos": ["notanobject", "another"]}}`,
			computeKeys:    map[string]interface{}{"id_key": "todos.1"},
			expectedValues: map[string]interface{}{"id_key": "another"},
		},
		{
			body:           `{"data": {"id": 1}}`,
			computeKeys:    map[string]interface{}{"id_key": "id"},
			expectedValues: map[string]interface{}{"id_key": "1"},
		},
		{
			body:           `{"data": {"pi": 3.14159}}`,
			computeKeys:    map[string]interface{}{"id_key": "pi"},
			expectedValues: map[string]interface{}{"id_key": "3.14159"},
		},
		{
			body:           `{"data": {"ready": false}}`,
			computeKeys:    map[string]interface{}{"id_key": "ready"},
			expectedValues: map[string]interface{}{"id_key": "false"},
		},
		{
			body:             `{"data": {"todos": [{"id": "computed_id"}, {"id": "second_id"}]}}`,
			computeKeys:      map[string]interface{}{"id_key": "todos.3.id"},
			expectedErrorMsg: "the path 'todos.3.id' does not exist in the response (tried: 'data.todos.3.id', 'data.paginatedData.0.todos.3.id', 'todos.3.id', 'paginatedData.0.todos.3.id'). Available top-level keys: [data]",
		},
		{
			body:             `{"data": {"todo": {"id": "computed_id"}}}`,
			computeKeys:      map[string]interface{}{"id_key": "notreal.id"},
			expectedErrorMsg: "the path 'notreal.id' does not exist in the response (tried: 'data.notreal.id', 'data.paginatedData.0.notreal.id', 'notreal.id', 'paginatedData.0.notreal.id'). Available top-level keys: [data]",
		},
	}

	for i, c := range cases {
		m, err := computeMutationVariableKeys(c.computeKeys, c.body)
		if c.expectedErrorMsg != "" {
			assert.Error(t, err, fmt.Sprintf("test case: %d", i))
			assert.EqualError(t, err, c.expectedErrorMsg, fmt.Sprintf("test case: %d", i))
		} else {
			assert.NoError(t, err, fmt.Sprintf("test case: %d", i))
			for k, v := range c.expectedValues {
				assert.Equal(t, v, m[k], fmt.Sprintf("test case: %d", i))
			}
		}
	}
}

// This test is illustrative of how one might find a key path from a value.
// It's not used directly by the provider logic but can be useful for debugging.
func TestFindPathForValue(t *testing.T) {
	cases := []struct {
		json      string
		value     string
		expectKey string
	}{
		{
			value:     "value1",
			json:      datablob,
			expectKey: "data.otherItems.0.field1",
		},
		{
			value:     "itemValueOne",
			json:      datablob,
			expectKey: "data.items.0",
		},
		{
			value:     "someValue",
			json:      datablob,
			expectKey: "data.someField",
		},
		{
			value:     "nestedListValue",
			json:      datablob,
			expectKey: "data.otherItems.2.nestedList.0",
		},
		{
			value:     "networkIDValue",
			json:      complexTest,
			expectKey: "data.virtualHost.networkInterfaceList.0.network.id",
		},
	}

	for i, c := range cases {
		foundPath := findPathForValue(c.json, c.value)
		assert.Equal(t, c.expectKey, foundPath, "test case %d", i)
	}
}
