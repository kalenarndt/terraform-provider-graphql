package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/kalenarndt/terraform-provider-graphql/graphql"
)

// Run the docs generation tool, check out the Terraform docs site for more information: https://developer.hashicorp.com/terraform/tutorials/providers-plugin-framework/provider-documents
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs

func main() {
	var debugMode bool

	flag.BoolVar(&debugMode, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/kalenarndt/graphql",
		Debug:   debugMode,
	}

	err := providerserver.Serve(context.Background(), graphql.New("dev"), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
}
