package main

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephFSDirectoryDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	dirName := acctest.RandomWithPrefix("test-dir")
	dirPath := "/volumes/" + dirName

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
				Config: testAccCephFSDirectoryConfig(fsName, dirPath) + fmt.Sprintf(`
					data "ceph_cephfs_directory" "test" {
					  vol_name = %q
					  path     = ceph_cephfs_directory.test.path
					}
				`, fsName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs_directory.test",
						tfjsonpath.New("path"),
						knownvalue.StringExact(dirPath),
					),
				},
			},
		},
	})
}

func TestAccCephFSDirectoryDataSource_NotFound(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	dirName := acctest.RandomWithPrefix("nonexistent")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_cephfs_directory" "test" {
					  vol_name = %q
					  path     = "/volumes/%s"
					}
				`, fsName, dirName),
				ExpectError: regexp.MustCompile(`does not exist`),
			},
		},
	})
}
