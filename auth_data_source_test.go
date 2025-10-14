package main

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/config"
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
				ConfigVariables: config.Variables{
					"endpoint": config.StringVariable(testDashboardURL),
					"username": config.StringVariable("admin"),
					"password": config.StringVariable("password"),
				},
				Config: `
					variable "endpoint" {
					  type = string
					}

					variable "username" {
					  type = string
					}

					variable "password" {
					  type = string
					}

					provider "ceph" {
					  endpoint = var.endpoint
					  username = var.username
					  password = var.password
					}

					data "ceph_auth" "client_admin" {
					  entity = "client.admin"
					}
				`,
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
