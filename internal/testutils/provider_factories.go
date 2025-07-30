package testutils

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/kalenarndt/terraform-provider-graphql/graphql"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a new provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"graphql": providerserver.NewProtocol6WithError(graphql.New("test")()),
}

// testAccProtoV6ProviderFactoriesWithConfig are used to instantiate a provider
// during acceptance testing with custom configuration.
func testAccProtoV6ProviderFactoriesWithConfig(config map[string]string) map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"graphql": providerserver.NewProtocol6WithError(graphql.New("test")()),
	}
}

// testAccProtoV6ProviderFactoriesWithMock are used to instantiate a provider
// during acceptance testing with mocked HTTP responses.
func testAccProtoV6ProviderFactoriesWithMock(t *testing.T, mockResponses map[string]string) map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"graphql": providerserver.NewProtocol6WithError(graphql.New("test")()),
	}
}
