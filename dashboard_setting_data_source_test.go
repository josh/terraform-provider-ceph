package main

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephDashboardSettingDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_dashboard_setting" "test" {
					  name = "GRAFANA_API_USERNAME"
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_dashboard_setting.test",
						tfjsonpath.New("value"),
						knownvalue.StringExact("admin"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_dashboard_setting.test",
						tfjsonpath.New("type"),
						knownvalue.StringExact("str"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_dashboard_setting.test",
						tfjsonpath.New("default"),
						knownvalue.StringExact("admin"),
					),
				},
			},
		},
	})
}
