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

func TestAccCephRBDSnapshotDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")
	imageName := acctest.RandomWithPrefix("test-image")
	snapName := acctest.RandomWithPrefix("test-snap")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
			testAccRBDSnapshotCLIFixture(t, poolName, imageName, snapName)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rbd_snapshot" "test" {
					  pool_name  = %q
					  image_name = %q
					  name       = %q
					}
				`, poolName, imageName, snapName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_snapshot.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(snapName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_snapshot.test",
						tfjsonpath.New("is_protected"),
						knownvalue.Bool(false),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_snapshot.test",
						tfjsonpath.New("size"),
						knownvalue.Int64Exact(8388608),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_snapshot.test",
						tfjsonpath.New("timestamp"),
						knownvalue.NotNull(),
					),
				},
			},
		},
	})
}
