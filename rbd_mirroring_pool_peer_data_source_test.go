package main

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephRBDMirroringPoolPeerDataSource(t *testing.T) {
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
				Config: testAccRBDMirroringPeerBaseConfig(poolName) + `
					resource "ceph_rbd_mirroring_pool_peer" "test" {
					  pool_name    = ceph_rbd_mirroring_pool_mode.test.pool_name
					  cluster_name = "remote-site"
					  client_id    = "rbd-remote"
					  mon_host     = "192.0.2.20:6789"
					}

					data "ceph_rbd_mirroring_pool_peer" "test" {
					  pool_name = ceph_rbd_mirroring_pool_peer.test.pool_name
					  uuid      = ceph_rbd_mirroring_pool_peer.test.uuid
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_mirroring_pool_peer.test",
						tfjsonpath.New("cluster_name"),
						knownvalue.StringExact("remote-site"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_mirroring_pool_peer.test",
						tfjsonpath.New("client_id"),
						knownvalue.StringExact("rbd-remote"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_mirroring_pool_peer.test",
						tfjsonpath.New("mon_host"),
						knownvalue.StringExact("192.0.2.20:6789"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rbd_mirroring_pool_peer.test",
						tfjsonpath.New("direction"),
						knownvalue.StringExact("rx-tx"),
					),
				},
			},
		},
	})
}
