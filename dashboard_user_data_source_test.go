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

func TestAccCephDashboardUserDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	username := acctest.RandomWithPrefix("test-dash-user")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccDashboardUserCLIFixture(t, username)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_dashboard_user" "test" {
					  username = %q
					}
				`, username),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_dashboard_user.test",
						tfjsonpath.New("username"),
						knownvalue.StringExact(username),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_dashboard_user.test",
						tfjsonpath.New("roles"),
						knownvalue.SetExact([]knownvalue.Check{
							knownvalue.StringExact("read-only"),
						}),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_dashboard_user.test",
						tfjsonpath.New("enabled"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_dashboard_user.test",
						tfjsonpath.New("pwd_update_required"),
						knownvalue.Bool(false),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_dashboard_user.test",
						tfjsonpath.New("last_update"),
						knownvalue.NotNull(),
					),
				},
			},
		},
	})
}
