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

func TestAccCephRBDImageDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")
	imageName := acctest.RandomWithPrefix("test-image")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
			testAccRBDImageCLIFixture(t, poolName, imageName, true)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rbd_image" "test" {
					  pool_name = %q
					  name      = %q
					}
				`, poolName, imageName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_image.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(imageName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_image.test",
						tfjsonpath.New("pool_name"),
						knownvalue.StringExact(poolName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_image.test",
						tfjsonpath.New("size"),
						knownvalue.Int64Exact(8388608),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_image.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_image.test",
						tfjsonpath.New("block_name_prefix"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_image.test",
						tfjsonpath.New("object_size"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_image.test",
						tfjsonpath.New("features_name"),
						knownvalue.NotNull(),
					),
				},
			},
		},
	})
}
