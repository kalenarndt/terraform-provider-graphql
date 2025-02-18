# <resource name> Query

This data source is appropriate when you need to read/create any resources without managing its full lifecycle as you would with the `graphql_mutation` resource. 

## Example Usage

```hcl
data "graphql_query" "basic_query" {
  query_variables = {}
  query     = file("./path/to/query/file")
}
```

### Example with pagination (using the Github repo pull requests query as an example): 
```hcl
data "graphql_query" "basic_query" {
  query = file("../../testdata/readQueryPaginated")
  query_variables = {
    owner = "repo_owner"
    name = "repo_name"
    after = ""
  }
  paginated = true
}
```

## Argument Reference

* `query_variables` - (Required) A map of any variables that will be used in your query. Each variable's value is interpreted as JSON when possible.

> NOTE: If a query variable is a number that must be interpreted as a string, it should be wrapped in quotations. For example `"marVar" = "\"123\""`.

* `query` - (Required) The graphql query. (See basic example below for what that looks like.)

## Attribute Reference

* `query_response` - A computed json encoded http response object received from the query.
    - To use properties from this response, leverage Terraform's built in [jsondecode](https://www.terraform.io/docs/configuration/functions/jsondecode.html) function.


->**Note** For a full guide on using this provider, see the full documentation site located at [https://sullivtr.github.io/terraform-provider-graphql/docs/provider.html](https://sullivtr.github.io/terraform-provider-graphql/docs/provider.html)
