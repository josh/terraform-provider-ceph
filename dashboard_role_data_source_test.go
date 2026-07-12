package main

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephDashboardRoleDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	roleName := acctest.RandomWithPrefix("test-dash-role")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccDashboardRoleCLIFixture(t, roleName)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_dashboard_role" "test" {
					  name = %q
					}

					data "ceph_dashboard_role" "system" {
					  name = "administrator"
					}
				`, roleName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_dashboard_role.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("Fixture role"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_dashboard_role.test",
						tfjsonpath.New("scopes_permissions"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"pool": knownvalue.SetExact([]knownvalue.Check{
								knownvalue.StringExact("read"),
							}),
						}),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_dashboard_role.test",
						tfjsonpath.New("system"),
						knownvalue.Bool(false),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_dashboard_role.system",
						tfjsonpath.New("system"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_dashboard_role.system",
						tfjsonpath.New("description"),
						knownvalue.NotNull(),
					),
				},
			},
		},
	})
}
