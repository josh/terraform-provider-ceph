package main

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephAuthDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"ceph": providerserver.NewProtocol6WithError(providerFunc()),
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					provider "ceph" {
					  endpoint = %q
					  username = "admin"
					  password = "password"
					}

					data "ceph_auth" "client_admin" {
					  entity = "client.admin"
					}
				`, testDashboardURL),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_auth.client_admin",
						tfjsonpath.New("entity"),
						knownvalue.StringExact("client.admin"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_auth.client_admin",
						tfjsonpath.New("key"),
						knownvalue.StringExact("AQB5m89objcKIxAAda2ULz/l3NH+mv9XzKePHQ=="),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_auth.client_admin",
						tfjsonpath.New("caps"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"mon": knownvalue.StringExact("allow *"),
							"mds": knownvalue.StringExact("allow *"),
							"osd": knownvalue.StringExact("allow *"),
							"mgr": knownvalue.StringExact("allow *"),
						}),
					),
				},
			},
		},
	})
}
