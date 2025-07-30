package graphql

import (
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/stretchr/testify/assert"
)

func TestInitializeRateLimiters(t *testing.T) {
	tests := []struct {
		name           string
		queryDelay     time.Duration
		mutationDelay  time.Duration
		expectQuery    bool
		expectMutation bool
	}{
		{
			name:           "both delays set",
			queryDelay:     100 * time.Millisecond,
			mutationDelay:  200 * time.Millisecond,
			expectQuery:    true,
			expectMutation: true,
		},
		{
			name:           "only query delay",
			queryDelay:     100 * time.Millisecond,
			mutationDelay:  0,
			expectQuery:    true,
			expectMutation: false,
		},
		{
			name:           "only mutation delay",
			queryDelay:     0,
			mutationDelay:  200 * time.Millisecond,
			expectQuery:    false,
			expectMutation: true,
		},
		{
			name:           "no delays",
			queryDelay:     0,
			mutationDelay:  0,
			expectQuery:    false,
			expectMutation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset global rate limiters
			queryRateLimiter = nil
			mutationRateLimiter = nil

			initializeRateLimiters(tt.queryDelay, tt.mutationDelay)

			if tt.expectQuery {
				assert.NotNil(t, queryRateLimiter)
			} else {
				assert.Nil(t, queryRateLimiter)
			}

			if tt.expectMutation {
				assert.NotNil(t, mutationRateLimiter)
			} else {
				assert.Nil(t, mutationRateLimiter)
			}
		})
	}
}

func TestPrepareQueryVariables(t *testing.T) {
	tests := []struct {
		name          string
		inputVars     map[string]interface{}
		cursor        string
		expectedVars  map[string]interface{}
		expectedCount int
	}{
		{
			name:          "nil input variables with cursor",
			inputVars:     nil,
			cursor:        "cursor123",
			expectedCount: 1,
		},
		{
			name:          "nil input variables without cursor",
			inputVars:     nil,
			cursor:        "",
			expectedCount: 0,
		},
		{
			name: "existing variables with cursor",
			inputVars: map[string]interface{}{
				"limit": 10,
				"filter": map[string]interface{}{
					"status": "active",
				},
			},
			cursor:        "cursor123",
			expectedCount: 3, // limit, filter, after
		},
		{
			name: "existing variables without cursor",
			inputVars: map[string]interface{}{
				"limit": 10,
			},
			cursor:        "",
			expectedCount: 1,
		},
		{
			name: "variables with JSON string values",
			inputVars: map[string]interface{}{
				"filter": `{"status": "active"}`,
			},
			cursor:        "",
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := prepareQueryVariables(tt.inputVars, tt.cursor)

			assert.Len(t, result, tt.expectedCount)

			if tt.cursor != "" {
				assert.Equal(t, tt.cursor, result["after"])
			} else {
				_, hasAfter := result["after"]
				assert.False(t, hasAfter)
			}

			// Check that original variables are preserved
			if tt.inputVars != nil {
				for k := range tt.inputVars {
					if k != "after" { // after is added by the function
						assert.Contains(t, result, k)
					}
				}
			}
		})
	}
}

func TestRecursivelyPrepareVariables(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "string input",
			input:    "test",
			expected: "test",
		},
		{
			name:     "number input",
			input:    42,
			expected: 42,
		},
		{
			name:     "boolean input",
			input:    true,
			expected: true,
		},
		{
			name: "simple map",
			input: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
			},
		},
		{
			name: "nested map",
			input: map[string]interface{}{
				"parent": map[string]interface{}{
					"child": "value",
				},
			},
			expected: map[string]interface{}{
				"parent": map[string]interface{}{
					"child": "value",
				},
			},
		},
		{
			name: "slice",
			input: []interface{}{
				"item1",
				"item2",
				map[string]interface{}{
					"nested": "value",
				},
			},
			expected: []interface{}{
				"item1",
				"item2",
				map[string]interface{}{
					"nested": "value",
				},
			},
		},
		{
			name: "JSON string in map",
			input: map[string]interface{}{
				"filter": `{"status": "active"}`,
			},
			expected: map[string]interface{}{
				"filter": `{"status": "active"}`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := recursivelyPrepareVariables(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsRateLimitError(t *testing.T) {
	tests := []struct {
		name     string
		diags    diag.Diagnostics
		expected bool
	}{
		{
			name:     "empty diagnostics",
			diags:    diag.Diagnostics{},
			expected: false,
		},
		{
			name: "rate limit error",
			diags: diag.Diagnostics{
				diag.NewErrorDiagnostic("Rate Limit", "HTTP 429 Too many requests"),
			},
			expected: true,
		},
		{
			name: "rate limit error with different message",
			diags: diag.Diagnostics{
				diag.NewErrorDiagnostic("Error", "Rate limit exceeded"),
			},
			expected: true,
		},
		{
			name: "other error",
			diags: diag.Diagnostics{
				diag.NewErrorDiagnostic("Other Error", "Something else"),
			},
			expected: false,
		},
		{
			name: "mixed diagnostics",
			diags: diag.Diagnostics{
				diag.NewErrorDiagnostic("Rate Limit", "HTTP 429 Too many requests"),
				diag.NewWarningDiagnostic("Warning", "Some warning"),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRateLimitError(tt.diags)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsBusinessLogicError(t *testing.T) {
	tests := []struct {
		name     string
		diags    diag.Diagnostics
		expected bool
	}{
		{
			name:     "empty diagnostics",
			diags:    diag.Diagnostics{},
			expected: false,
		},
		{
			name: "business logic error - multiple versions",
			diags: diag.Diagnostics{
				diag.NewErrorDiagnostic("Error", "Can't enable multiple versions"),
			},
			expected: true,
		},
		{
			name: "business logic error - already enabled",
			diags: diag.Diagnostics{
				diag.NewErrorDiagnostic("Error", "already enabled"),
			},
			expected: true,
		},
		{
			name: "business logic error - already exists",
			diags: diag.Diagnostics{
				diag.NewErrorDiagnostic("Error", "already exists"),
			},
			expected: true,
		},
		{
			name: "business logic error - conflict",
			diags: diag.Diagnostics{
				diag.NewErrorDiagnostic("Error", "conflict"),
			},
			expected: true,
		},
		{
			name: "other error",
			diags: diag.Diagnostics{
				diag.NewErrorDiagnostic("Other Error", "Something else"),
			},
			expected: false,
		},
		{
			name: "mixed diagnostics",
			diags: diag.Diagnostics{
				diag.NewErrorDiagnostic("Error", "already enabled"),
				diag.NewWarningDiagnostic("Warning", "Some warning"),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBusinessLogicError(tt.diags)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseRetryDelay(t *testing.T) {
	tests := []struct {
		name     string
		diags    diag.Diagnostics
		expected time.Duration
	}{
		{
			name:     "empty diagnostics",
			diags:    diag.Diagnostics{},
			expected: 0,
		},
		{
			name: "no retry delay in diagnostics",
			diags: diag.Diagnostics{
				diag.NewErrorDiagnostic("Error", "Some error"),
			},
			expected: 0,
		},
		{
			name: "with retry delay in nanoseconds",
			diags: diag.Diagnostics{
				diag.NewErrorDiagnostic("Error", `HTTP 429 error with {"retryAfterNS": 5000000000}`),
			},
			expected: 5 * time.Second,
		},
		{
			name: "invalid retry delay",
			diags: diag.Diagnostics{
				diag.NewErrorDiagnostic("Error", "Some error with retry delay: invalid"),
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRetryDelay(tt.diags)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractPaginatedData(t *testing.T) {
	tests := []struct {
		name            string
		data            map[string]interface{}
		expectedData    map[string]interface{}
		expectedHasMore bool
		expectedCursor  string
	}{
		{
			name: "no pagination",
			data: map[string]interface{}{
				"items": []interface{}{"item1", "item2"},
			},
			expectedData: map[string]interface{}{
				"items": []interface{}{"item1", "item2"},
			},
			expectedHasMore: false,
			expectedCursor:  "",
		},
		{
			name: "with pagination - edges structure",
			data: map[string]interface{}{
				"items": map[string]interface{}{
					"edges": []interface{}{"edge1", "edge2"},
					"pageInfo": map[string]interface{}{
						"hasNextPage": true,
						"endCursor":   "cursor123",
					},
				},
			},
			expectedData: map[string]interface{}{
				"edges": []interface{}{"edge1", "edge2"},
				"pageInfo": map[string]interface{}{
					"hasNextPage": true,
					"endCursor":   "cursor123",
				},
			},
			expectedHasMore: true,
			expectedCursor:  "cursor123",
		},
		{
			name: "no more pages",
			data: map[string]interface{}{
				"items": map[string]interface{}{
					"edges": []interface{}{"edge1", "edge2"},
					"pageInfo": map[string]interface{}{
						"hasNextPage": false,
						"endCursor":   "cursor123",
					},
				},
			},
			expectedData: map[string]interface{}{
				"edges": []interface{}{"edge1", "edge2"},
				"pageInfo": map[string]interface{}{
					"hasNextPage": false,
					"endCursor":   "cursor123",
				},
			},
			expectedHasMore: false,
			expectedCursor:  "cursor123",
		},
		{
			name:            "nil data",
			data:            nil,
			expectedData:    nil,
			expectedHasMore: false,
			expectedCursor:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, hasMore, cursor := extractPaginatedData(tt.data)

			assert.Equal(t, tt.expectedData, data)
			assert.Equal(t, tt.expectedHasMore, hasMore)
			assert.Equal(t, tt.expectedCursor, cursor)
		})
	}
}

func TestFindPageInfo(t *testing.T) {
	tests := []struct {
		name          string
		data          map[string]interface{}
		expectedInfo  map[string]interface{}
		expectedFound bool
	}{
		{
			name:          "no page info",
			data:          map[string]interface{}{},
			expectedInfo:  nil,
			expectedFound: false,
		},
		{
			name: "with page info in nested structure",
			data: map[string]interface{}{
				"items": map[string]interface{}{
					"pageInfo": map[string]interface{}{
						"hasNextPage": true,
						"endCursor":   "cursor123",
					},
				},
			},
			expectedInfo: map[string]interface{}{
				"pageInfo": map[string]interface{}{
					"hasNextPage": true,
					"endCursor":   "cursor123",
				},
			},
			expectedFound: true,
		},
		{
			name: "page info is not a map",
			data: map[string]interface{}{
				"pageInfo": "not a map",
			},
			expectedInfo:  nil,
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, found := findPageInfo(tt.data)

			assert.Equal(t, tt.expectedInfo, info)
			assert.Equal(t, tt.expectedFound, found)
		})
	}
}
