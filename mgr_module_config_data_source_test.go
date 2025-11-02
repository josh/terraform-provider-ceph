package main

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephMgrModuleConfigDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_mgr_module_config" "dashboard" {
					  module_name = "dashboard"
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_mgr_module_config.dashboard",
						tfjsonpath.New("id"),
						knownvalue.StringExact("dashboard"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_mgr_module_config.dashboard",
						tfjsonpath.New("module_name"),
						knownvalue.StringExact("dashboard"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_mgr_module_config.dashboard",
						tfjsonpath.New("configs").AtMapKey("ssl"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_mgr_module_config.dashboard",
						tfjsonpath.New("configs").AtMapKey("server_port"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_mgr_module_config.dashboard",
						tfjsonpath.New("configs").AtMapKey("server_addr"),
						knownvalue.NotNull(),
					),
				},
			},
		},
	})
}
