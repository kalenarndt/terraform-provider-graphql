terraform {
  required_providers {
    graphql = {
      source = "kalenarndt/graphql"
    }
  }
}

provider "graphql" {
  url = "http://localhost:8080/graphql"
}

# Test with object mutation variables
resource "graphql_mutation" "test_object" {
  read_query = <<EOF
query getTest($id: ID!) {
  test(id: $id) {
    id
    name
    value
  }
}
EOF

  create_mutation = <<EOF
mutation createTest($input: CreateTestInput!) {
  createTest(input: $input) {
    test {
      id
      name
      value
    }
  }
}
EOF

  update_mutation = <<EOF
mutation updateTest($input: UpdateTestInput!) {
  updateTest(input: $input) {
    test {
      id
      name
      value
    }
  }
}
EOF

  delete_mutation = <<EOF
mutation deleteTest($input: DeleteTestInput!) {
  deleteTest(input: $input) {
    success
  }
}
EOF

  # Test with object mutation variables
  mutation_variables = {
    input = {
      name  = "test-object"
      value = 42
    }
  }

  # Test with object read query variables
  read_query_variables = {
    id = "test-123"
  }

  # Test with object delete mutation variables
  delete_mutation_variables = {
    input = {
      id = "test-123"
    }
  }

  compute_mutation_keys = {
    "id"    = "createTest.test.id"
    "name"  = "createTest.test.name"
    "value" = "createTest.test.value"
  }

  read_compute_keys = {
    "id"    = "test.id"
    "name"  = "test.name"
    "value" = "test.value"
  }
}

# Test with array mutation variables
resource "graphql_mutation" "test_array" {
  read_query = <<EOF
query getTests($ids: [ID!]!) {
  tests(ids: $ids) {
    id
    name
    value
  }
}
EOF

  create_mutation = <<EOF
mutation createTests($input: [CreateTestInput!]!) {
  createTests(input: $input) {
    tests {
      id
      name
      value
    }
  }
}
EOF

  update_mutation = <<EOF
mutation updateTests($input: [UpdateTestInput!]!) {
  updateTests(input: $input) {
    tests {
      id
      name
      value
    }
  }
}
EOF

  delete_mutation = <<EOF
mutation deleteTests($input: [ID!]!) {
  deleteTests(input: $input) {
    success
  }
}
EOF

  # Test with array mutation variables
  mutation_variables = [
    {
      name  = "test-1"
      value = 10
    },
    {
      name  = "test-2"
      value = 20
    }
  ]

  # Test with array read query variables
  read_query_variables = [
    "test-123",
    "test-456"
  ]

  # Test with array delete mutation variables
  delete_mutation_variables = [
    "test-123",
    "test-456"
  ]

  compute_mutation_keys = {
    "ids" = "createTests.tests[*].id"
  }

  read_compute_keys = {
    "ids" = "tests[*].id"
  }
}

# Test with primitive mutation variables
resource "graphql_mutation" "test_primitive" {
  read_query = <<EOF
query getTest($id: ID!) {
  test(id: $id) {
    id
    name
    value
  }
}
EOF

  create_mutation = <<EOF
mutation createTest($name: String!, $value: Int!) {
  createTest(name: $name, value: $value) {
    test {
      id
      name
      value
    }
  }
}
EOF

  update_mutation = <<EOF
mutation updateTest($id: ID!, $name: String!, $value: Int!) {
  updateTest(id: $id, name: $name, value: $value) {
    test {
      id
      name
      value
    }
  }
}
EOF

  delete_mutation = <<EOF
mutation deleteTest($id: ID!) {
  deleteTest(id: $id) {
    success
  }
}
EOF

  # Test with primitive mutation variables
  mutation_variables = {
    name  = "test-primitive"
    value = 100
  }

  # Test with primitive read query variables
  read_query_variables = "test-123"

  # Test with primitive delete mutation variables
  delete_mutation_variables = "test-123"

  compute_mutation_keys = {
    "id" = "createTest.test.id"
  }

  read_compute_keys = {
    "id" = "test.id"
  }
}