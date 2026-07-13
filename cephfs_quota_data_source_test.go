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

func TestAccCephFSQuotaDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccCephFSQuotaSubvolumeConfig(fsName, subvolName) + fmt.Sprintf(`
					resource "ceph_cephfs_quota" "test" {
					  vol_name  = %q
					  path      = ceph_cephfs_subvolume.test.path
					  max_bytes = 10485760
					  max_files = 1000
					}

					data "ceph_cephfs_quota" "test" {
					  vol_name   = %q
					  path       = ceph_cephfs_subvolume.test.path
					  depends_on = [ceph_cephfs_quota.test]
					}
				`, fsName, fsName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs_quota.test",
						tfjsonpath.New("max_bytes"),
						knownvalue.Int64Exact(10485760),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs_quota.test",
						tfjsonpath.New("max_files"),
						knownvalue.Int64Exact(1000),
					),
				},
			},
		},
	})
}
