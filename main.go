package main

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

var (
	version string = "0.2.0"
)

func main() {
	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/josh/ceph",
	}

	err := providerserver.Serve(context.Background(), providerFunc, opts)

	if err != nil {
		log.Fatal(err.Error())
	}
}
