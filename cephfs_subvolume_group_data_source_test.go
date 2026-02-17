package main

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephFSSubvolumeGroupDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	groupName := acctest.RandomWithPrefix("test-group")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCleanupCephFSSubvolumeGroup(t, fsName, groupName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)

			if err := cephTestClusterCLI.CephFSSubvolumeGroupCreate(t.Context(), fsName, groupName); err != nil {
				t.Fatalf("Failed to create CephFS subvolume group: %v", err)
			}
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_cephfs_subvolume_group" "test" {
					  name     = %q
					  vol_name = %q
					}
				`, groupName, fsName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs_subvolume_group.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(groupName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs_subvolume_group.test",
						tfjsonpath.New("vol_name"),
						knownvalue.StringExact(fsName),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_cephfs_subvolume_group.test", "name", groupName),
					resource.TestCheckResourceAttr("data.ceph_cephfs_subvolume_group.test", "vol_name", fsName),
				),
			},
		},
	})
}

func testAccCleanupCephFSSubvolumeGroup(t *testing.T, fsName string, groupName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if err := cephTestClusterCLI.CephFSSubvolumeGroupDelete(t.Context(), fsName, groupName); err != nil {
			t.Logf("Warning: failed to cleanup CephFS subvolume group %s/%s: %v", fsName, groupName, err)
		}
		return nil
	}
}
