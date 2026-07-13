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

func TestAccCephRBDMirroringPoolModeDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccRBDMirroringPoolModeConfig(poolName, "pool") + fmt.Sprintf(`
					data "ceph_rbd_mirroring_pool_mode" "test" {
					  pool_name  = %q
					  depends_on = [ceph_rbd_mirroring_pool_mode.test]
					}
				`, poolName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_mirroring_pool_mode.test",
						tfjsonpath.New("mode"),
						knownvalue.StringExact("pool"),
					),
				},
			},
		},
	})
}
