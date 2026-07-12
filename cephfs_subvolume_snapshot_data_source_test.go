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

func TestAccCephFSSubvolumeSnapshotDataSource(t *testing.T) {
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
					data "ceph_cephfs_subvolume_snapshot" "test" {
					  vol_name    = %q
					  subvol_name = %q
					  name        = %q
					}
				`, fsName, subvolName, snapName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs_subvolume_snapshot.test",
						tfjsonpath.New("vol_name"),
						knownvalue.StringExact(fsName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs_subvolume_snapshot.test",
						tfjsonpath.New("subvol_name"),
						knownvalue.StringExact(subvolName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs_subvolume_snapshot.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(snapName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs_subvolume_snapshot.test",
						tfjsonpath.New("created_at"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs_subvolume_snapshot.test",
						tfjsonpath.New("data_pool"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs_subvolume_snapshot.test",
						tfjsonpath.New("has_pending_clones"),
						knownvalue.Bool(false),
					),
				},
			},
		},
	})
}
