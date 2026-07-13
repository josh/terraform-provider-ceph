package main

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephFSSnapshotDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	dirName := acctest.RandomWithPrefix("test-dir")
	snapName := acctest.RandomWithPrefix("test-snap")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
			testAccEnsureCephFSVolumesRoot(t, fsName)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccCephFSSnapshotConfig(fsName, dirName, snapName) + fmt.Sprintf(`
					data "ceph_cephfs_snapshot" "test" {
					  vol_name = %q
					  path     = ceph_cephfs_directory.test.path
					  name     = ceph_cephfs_snapshot.test.name
					}
				`, fsName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs(
						"data.ceph_cephfs_snapshot.test",
						tfjsonpath.New("created"),
						"ceph_cephfs_snapshot.test",
						tfjsonpath.New("created"),
						compare.ValuesSame(),
					),
				},
			},
		},
	})
}
