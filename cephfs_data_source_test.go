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

func TestAccCephFSDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := acctest.RandomWithPrefix("test-cephfs-ds")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCleanupCephFS(t, fsName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)

			if err := cephTestClusterCLI.CephFSVolumeCreate(t.Context(), fsName); err != nil {
				t.Fatalf("Failed to create CephFS volume: %v", err)
			}
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_cephfs" "test" {
					  name = %q
					}
				`, fsName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(fsName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs.test",
						tfjsonpath.New("metadata_pool_id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_cephfs.test",
						tfjsonpath.New("data_pool_ids"),
						knownvalue.NotNull(),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_cephfs.test", "name", fsName),
					resource.TestCheckResourceAttrSet("data.ceph_cephfs.test", "id"),
					resource.TestCheckResourceAttrSet("data.ceph_cephfs.test", "metadata_pool_id"),
				),
			},
		},
	})
}

func testAccCleanupCephFS(t *testing.T, fsName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if err := cephTestClusterCLI.CephFSVolumeDelete(t.Context(), fsName); err != nil {
			t.Logf("Warning: failed to cleanup CephFS volume %s: %v", fsName, err)
		}
		return nil
	}
}
