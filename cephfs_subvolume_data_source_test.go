package main

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephFSSubvolumeDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCleanupCephFSSubvolume(t, fsName, subvolName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)

			if err := cephTestClusterCLI.CephFSSubvolumeCreate(t.Context(), fsName, subvolName, ""); err != nil {
				t.Fatalf("Failed to create CephFS subvolume: %v", err)
			}
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_cephfs_subvolume" "test" {
					  name     = %q
					  vol_name = %q
					}
				`, subvolName, fsName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs_subvolume.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(subvolName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs_subvolume.test",
						tfjsonpath.New("vol_name"),
						knownvalue.StringExact(fsName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs_subvolume.test",
						tfjsonpath.New("path"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs_subvolume.test",
						tfjsonpath.New("state"),
						knownvalue.NotNull(),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_cephfs_subvolume.test", "name", subvolName),
					resource.TestCheckResourceAttr("data.ceph_cephfs_subvolume.test", "vol_name", fsName),
					resource.TestCheckResourceAttrSet("data.ceph_cephfs_subvolume.test", "path"),
					resource.TestCheckResourceAttrSet("data.ceph_cephfs_subvolume.test", "data_pool"),
					resource.TestCheckResourceAttrSet("data.ceph_cephfs_subvolume.test", "state"),
				),
			},
		},
	})
}

func TestAccCephFSSubvolumeDataSource_group(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	groupName := acctest.RandomWithPrefix("test-subvol-ds-group")
	subvolName := acctest.RandomWithPrefix("test-subvol-in-group")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)

			if err := cephTestClusterCLI.CephFSSubvolumeGroupCreate(t.Context(), fsName, groupName); err != nil {
				t.Fatalf("Failed to create CephFS subvolume group: %v", err)
			}
			testCleanup(t, func(ctx context.Context) {
				if err := cephTestClusterCLI.CephFSSubvolumeGroupDelete(ctx, fsName, groupName); err != nil {
					t.Logf("Warning: failed to cleanup CephFS subvolume group %s/%s: %v", fsName, groupName, err)
				}
			})

			if err := cephTestClusterCLI.CephFSSubvolumeCreate(t.Context(), fsName, subvolName, groupName); err != nil {
				t.Fatalf("Failed to create CephFS subvolume in group: %v", err)
			}
			testCleanup(t, func(ctx context.Context) {
				if err := cephTestClusterCLI.CephFSSubvolumeDelete(ctx, fsName, subvolName, groupName); err != nil {
					t.Logf("Warning: failed to cleanup CephFS subvolume %s/%s: %v", fsName, subvolName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_cephfs_subvolume" "test" {
					  name       = %q
					  vol_name   = %q
					  group_name = %q
					}
				`, subvolName, fsName, groupName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_cephfs_subvolume.test", "name", subvolName),
					resource.TestCheckResourceAttr("data.ceph_cephfs_subvolume.test", "vol_name", fsName),
					resource.TestCheckResourceAttr("data.ceph_cephfs_subvolume.test", "group_name", groupName),
					resource.TestMatchResourceAttr("data.ceph_cephfs_subvolume.test", "path", regexp.MustCompile(regexp.QuoteMeta("/volumes/"+groupName+"/"+subvolName))),
					resource.TestCheckResourceAttrSet("data.ceph_cephfs_subvolume.test", "data_pool"),
					resource.TestCheckResourceAttrSet("data.ceph_cephfs_subvolume.test", "state"),
				),
			},
		},
	})
}

func testAccCleanupCephFSSubvolume(t *testing.T, fsName string, subvolName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if err := cephTestClusterCLI.CephFSSubvolumeDelete(t.Context(), fsName, subvolName, ""); err != nil {
			t.Logf("Warning: failed to cleanup CephFS subvolume %s/%s: %v", fsName, subvolName, err)
		}
		return nil
	}
}
