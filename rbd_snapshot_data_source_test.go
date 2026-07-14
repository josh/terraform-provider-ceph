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

func TestAccCephRBDSnapshotDataSource_namespace(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")
	namespace := acctest.RandomWithPrefix("test-ns")
	imageName := acctest.RandomWithPrefix("test-image")
	snapName := acctest.RandomWithPrefix("test-snap")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + testAccRBDPoolConfig(poolName) + fmt.Sprintf(`
					resource "ceph_rbd_namespace" "test" {
					  pool_name = ceph_pool.test.name
					  name      = %q
					}

					resource "ceph_rbd_image" "test" {
					  pool_name = ceph_pool.test.name
					  namespace = ceph_rbd_namespace.test.name
					  name      = %q
					  size      = 8388608

					  timeouts = {
					    create = "5m"
					    update = "5m"
					    delete = "5m"
					  }
					}

					resource "ceph_rbd_snapshot" "test" {
					  pool_name  = ceph_pool.test.name
					  namespace  = ceph_rbd_image.test.namespace
					  image_name = ceph_rbd_image.test.name
					  name       = %q
					}

					data "ceph_rbd_snapshot" "test" {
					  pool_name  = ceph_pool.test.name
					  namespace  = ceph_rbd_snapshot.test.namespace
					  image_name = ceph_rbd_snapshot.test.image_name
					  name       = ceph_rbd_snapshot.test.name
					}
				`, namespace, imageName, snapName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_snapshot.test",
						tfjsonpath.New("namespace"),
						knownvalue.StringExact(namespace),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_snapshot.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(snapName),
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
