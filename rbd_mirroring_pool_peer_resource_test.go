package main

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func testAccRBDMirroringPeerBaseConfig(poolName string) string {
	return testAccProviderConfigBlock + testAccRBDPoolConfig(poolName) + `
		resource "ceph_rbd_mirroring_pool_mode" "test" {
		  pool_name = ceph_pool.test.name
		  mode      = "image"
		}
	`
}

func TestAccCephRBDMirroringPoolPeerResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")

	peerConfig := func(clusterName, monHost string) string {
		return testAccRBDMirroringPeerBaseConfig(poolName) + fmt.Sprintf(`
			resource "ceph_rbd_mirroring_pool_peer" "test" {
			  pool_name    = ceph_rbd_mirroring_pool_mode.test.pool_name
			  cluster_name = %q
			  client_id    = "rbd-remote"
			  mon_host     = %q
			}
		`, clusterName, monHost)
	}

	uuidStable := statecheck.CompareValue(compare.ValuesSame())

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          peerConfig("remote-site", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rbd_mirroring_pool_peer.test",
						tfjsonpath.New("uuid"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_rbd_mirroring_pool_peer.test",
						tfjsonpath.New("direction"),
						knownvalue.StringExact("rx-tx"),
					),
					statecheck.ExpectKnownValue(
						"ceph_rbd_mirroring_pool_peer.test",
						tfjsonpath.New("mon_host"),
						knownvalue.StringExact(""),
					),
					uuidStable.AddStateValue(
						"ceph_rbd_mirroring_pool_peer.test",
						tfjsonpath.New("uuid"),
					),
				},
				Check: checkRBDMirrorPeer(t, poolName, "remote-site", "client.rbd-remote"),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          peerConfig("renamed-site", "192.0.2.10:6789"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rbd_mirroring_pool_peer.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					uuidStable.AddStateValue(
						"ceph_rbd_mirroring_pool_peer.test",
						tfjsonpath.New("uuid"),
					),
				},
				Check: checkRBDMirrorPeer(t, poolName, "renamed-site", "client.rbd-remote"),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          peerConfig("renamed-site", "192.0.2.10:6789"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccRBDMirroringPeerBaseConfig(poolName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rbd_mirroring_pool_peer.test", plancheck.ResourceActionDestroy),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkRBDMirrorPeerCount(t, poolName, 0),
					checkRBDMirrorPoolMode(t, poolName, "image"),
				),
			},
		},
	})
}

func TestAccCephRBDMirroringPoolPeerResource_MirroringDisabled(t *testing.T) {
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
				Config: testAccProviderConfigBlock + testAccRBDPoolConfig(poolName) + `
					resource "ceph_rbd_mirroring_pool_peer" "test" {
					  pool_name    = ceph_pool.test.name
					  cluster_name = "remote-site"
					  client_id    = "rbd-remote"
					}
				`,
				ExpectError: regexp.MustCompile(`mirroring must be enabled`),
			},
		},
	})
}

func TestAccCephRBDMirroringPoolPeerResource_OutOfBandRemoval(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")

	config := testAccRBDMirroringPeerBaseConfig(poolName) + `
		resource "ceph_rbd_mirroring_pool_peer" "test" {
		  pool_name    = ceph_rbd_mirroring_pool_mode.test.pool_name
		  cluster_name = "remote-site"
		  client_id    = "rbd-remote"
		}
	`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				Check:           checkRBDMirrorPeer(t, poolName, "remote-site", "client.rbd-remote"),
			},
			{
				PreConfig: func() {
					ctx := t.Context()
					info, err := cephTestClusterCLI.RBDMirrorPoolInfo(ctx, poolName)
					if err != nil {
						t.Fatalf("Failed to get mirror pool info: %v", err)
					}
					if len(info.Peers) != 1 {
						t.Fatalf("Expected 1 peer, got %d", len(info.Peers))
					}
					if err := cephTestClusterCLI.RBDMirrorPoolPeerRemove(ctx, poolName, info.Peers[0].UUID); err != nil {
						t.Fatalf("Failed to remove peer out of band: %v", err)
					}
				},
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rbd_mirroring_pool_peer.test", plancheck.ResourceActionCreate),
					},
				},
				Check: checkRBDMirrorPeer(t, poolName, "remote-site", "client.rbd-remote"),
			},
		},
	})
}

func TestAccCephRBDMirroringPoolPeerResource_Import(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")

	var peerUUID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
			testAccRBDMirroringCLIFixture(t, poolName)

			uuid, err := cephTestClusterCLI.RBDMirrorPoolPeerAdd(t.Context(), poolName, "client.rbd-remote", "remote-site")
			if err != nil {
				t.Fatalf("Failed to add rbd mirror peer: %v", err)
			}
			peerUUID = uuid
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rbd_mirroring_pool_peer" "test" {
					  pool_name    = %q
					  cluster_name = "remote-site"
					  client_id    = "rbd-remote"
					}
				`, poolName),
				ResourceName: "ceph_rbd_mirroring_pool_peer.test",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return poolName + "/" + peerUUID, nil
				},
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["pool_name"] != poolName {
						return fmt.Errorf("expected pool_name %q, got %q", poolName, attrs["pool_name"])
					}
					if attrs["uuid"] != peerUUID {
						return fmt.Errorf("expected uuid %q, got %q", peerUUID, attrs["uuid"])
					}
					if attrs["cluster_name"] != "remote-site" {
						return fmt.Errorf("expected cluster_name remote-site, got %q", attrs["cluster_name"])
					}
					if attrs["client_id"] != "rbd-remote" {
						return fmt.Errorf("expected client_id rbd-remote, got %q", attrs["client_id"])
					}
					if attrs["mon_host"] != "" {
						return fmt.Errorf("expected empty mon_host, got %q", attrs["mon_host"])
					}
					return nil
				},
			},
		},
	})
}

func checkRBDMirrorPeer(t *testing.T, poolName, siteName, clientName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		info, err := cephTestClusterCLI.RBDMirrorPoolInfo(t.Context(), poolName)
		if err != nil {
			return fmt.Errorf("failed to get mirror pool info: %w", err)
		}
		for _, peer := range info.Peers {
			if peer.SiteName == siteName && peer.ClientName == clientName {
				return nil
			}
		}
		return fmt.Errorf("peer %s@%s not found on pool %q, peers: %v", clientName, siteName, poolName, info.Peers)
	}
}

func checkRBDMirrorPeerCount(t *testing.T, poolName string, want int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		info, err := cephTestClusterCLI.RBDMirrorPoolInfo(t.Context(), poolName)
		if err != nil {
			return fmt.Errorf("failed to get mirror pool info: %w", err)
		}
		if len(info.Peers) != want {
			return fmt.Errorf("expected %d peers on pool %q, got %d: %v", want, poolName, len(info.Peers), info.Peers)
		}
		return nil
	}
}
