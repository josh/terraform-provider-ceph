package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephFSSubvolumeSnapshotResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol")
	snapName := acctest.RandomWithPrefix("test-snap")
	renamedSnapName := acctest.RandomWithPrefix("test-snap-renamed")

	subvolConfig := testAccProviderConfigBlock + fmt.Sprintf(`
		resource "ceph_cephfs_subvolume" "test" {
		  name     = %q
		  vol_name = %q

		  timeouts = {
		    create = "5m"
		    delete = "5m"
		  }
		}
	`, subvolName, fsName)

	snapshotConfig := func(name string) string {
		return subvolConfig + fmt.Sprintf(`
			resource "ceph_cephfs_subvolume_snapshot" "test" {
			  vol_name    = ceph_cephfs_subvolume.test.vol_name
			  subvol_name = ceph_cephfs_subvolume.test.name
			  name        = %q
			}
		`, name)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSSubvolumeSnapshotDestroy(t, fsName, subvolName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          snapshotConfig(snapName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume_snapshot.test",
						tfjsonpath.New("vol_name"),
						knownvalue.StringExact(fsName),
					),
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume_snapshot.test",
						tfjsonpath.New("subvol_name"),
						knownvalue.StringExact(subvolName),
					),
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume_snapshot.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(snapName),
					),
				},
				Check: checkCephFSSubvolumeSnapshotExists(t, fsName, subvolName, snapName),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          snapshotConfig(snapName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          snapshotConfig(renamedSnapName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_cephfs_subvolume_snapshot.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephFSSubvolumeSnapshotExists(t, fsName, subvolName, renamedSnapName),
					checkCephFSSubvolumeSnapshotAbsent(t, fsName, subvolName, snapName),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          subvolConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_cephfs_subvolume_snapshot.test", plancheck.ResourceActionDestroy),
					},
				},
				Check: checkCephFSSubvolumeSnapshotAbsent(t, fsName, subvolName, renamedSnapName),
			},
		},
	})
}

func TestAccCephFSSubvolumeSnapshotResource_Import(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol")
	snapName := acctest.RandomWithPrefix("test-snap")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
			testAccCephFSSubvolumeSnapshotCLIFixture(t, fsName, subvolName, snapName)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_cephfs_subvolume_snapshot" "test" {
					  vol_name    = %q
					  subvol_name = %q
					  name        = %q
					}
				`, fsName, subvolName, snapName),
				ResourceName:  "ceph_cephfs_subvolume_snapshot.test",
				ImportState:   true,
				ImportStateId: fsName + "/" + subvolName + "/" + snapName,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["vol_name"] != fsName {
						return fmt.Errorf("expected vol_name %q, got %q", fsName, attrs["vol_name"])
					}
					if attrs["subvol_name"] != subvolName {
						return fmt.Errorf("expected subvol_name %q, got %q", subvolName, attrs["subvol_name"])
					}
					if attrs["name"] != snapName {
						return fmt.Errorf("expected name %q, got %q", snapName, attrs["name"])
					}
					return nil
				},
			},
		},
	})
}

func TestAccCephFSSubvolumeSnapshotResource_OutOfBandDeletion(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol-oob")
	snapName := acctest.RandomWithPrefix("test-snap-oob")

	config := testAccProviderConfigBlock + fmt.Sprintf(`
		resource "ceph_cephfs_subvolume" "test" {
		  name     = %q
		  vol_name = %q

		  timeouts = {
		    create = "5m"
		    delete = "5m"
		  }
		}

		resource "ceph_cephfs_subvolume_snapshot" "test" {
		  vol_name    = ceph_cephfs_subvolume.test.vol_name
		  subvol_name = ceph_cephfs_subvolume.test.name
		  name        = %q
		}
	`, subvolName, fsName, snapName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSSubvolumeSnapshotDestroy(t, fsName, subvolName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				Check:           checkCephFSSubvolumeSnapshotExists(t, fsName, subvolName, snapName),
			},
			{
				PreConfig: func() {
					if err := cephTestClusterCLI.CephFSSubvolumeSnapshotDelete(t.Context(), fsName, subvolName, snapName); err != nil {
						t.Fatalf("Failed to delete snapshot out of band: %v", err)
					}
				},
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_cephfs_subvolume_snapshot.test", plancheck.ResourceActionCreate),
					},
				},
				Check: checkCephFSSubvolumeSnapshotExists(t, fsName, subvolName, snapName),
			},
		},
	})
}

func testAccCephFSSubvolumeSnapshotCLIFixture(t *testing.T, fsName, subvolName, snapName string) {
	ctx := t.Context()
	if err := cephTestClusterCLI.CephFSSubvolumeCreate(ctx, fsName, subvolName); err != nil {
		t.Fatalf("Failed to create CephFS subvolume: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := cephTestClusterCLI.CephFSSubvolumeSnapshotDelete(cleanupCtx, fsName, subvolName, snapName); err != nil {
			t.Errorf("Failed to delete CephFS subvolume snapshot: %v", err)
		}
		if err := cephTestClusterCLI.CephFSSubvolumeDelete(cleanupCtx, fsName, subvolName); err != nil {
			t.Errorf("Failed to delete CephFS subvolume: %v", err)
		}
	})
	if err := cephTestClusterCLI.CephFSSubvolumeSnapshotCreate(ctx, fsName, subvolName, snapName); err != nil {
		t.Fatalf("Failed to create CephFS subvolume snapshot: %v", err)
	}
}

func checkCephFSSubvolumeSnapshotExists(t *testing.T, fsName, subvolName, snapName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.CephFSSubvolumeSnapshotExists(t.Context(), fsName, subvolName, snapName)
		if err != nil {
			return fmt.Errorf("failed to check CephFS subvolume snapshot existence: %w", err)
		}

		if !exists {
			return fmt.Errorf("CephFS subvolume snapshot %q not found in %s/%s", snapName, fsName, subvolName)
		}

		return nil
	}
}

func checkCephFSSubvolumeSnapshotAbsent(t *testing.T, fsName, subvolName, snapName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.CephFSSubvolumeSnapshotExists(t.Context(), fsName, subvolName, snapName)
		if err != nil {
			return fmt.Errorf("failed to check CephFS subvolume snapshot existence: %w", err)
		}

		if exists {
			return fmt.Errorf("CephFS subvolume snapshot %q still exists in %s/%s", snapName, fsName, subvolName)
		}

		return nil
	}
}

func testAccCheckCephFSSubvolumeSnapshotDestroy(t *testing.T, fsName, subvolName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		subvolExists, err := cephTestClusterCLI.CephFSSubvolumeExists(ctx, fsName, subvolName)
		if err != nil {
			return fmt.Errorf("failed to check CephFS subvolume existence: %w", err)
		}
		if !subvolExists {
			return nil
		}

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_cephfs_subvolume_snapshot" {
				continue
			}

			snapName := rs.Primary.Attributes["name"]

			exists, err := cephTestClusterCLI.CephFSSubvolumeSnapshotExists(ctx, fsName, subvolName, snapName)
			if err != nil {
				return fmt.Errorf("failed to check CephFS subvolume snapshot existence: %w", err)
			}

			if exists {
				return fmt.Errorf("CephFS subvolume snapshot %q still exists in %s/%s", snapName, fsName, subvolName)
			}
		}

		return nil
	}
}
