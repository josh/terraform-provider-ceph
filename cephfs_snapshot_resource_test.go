package main

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func testAccCephFSSnapshotConfig(fsName, dirName, snapName string) string {
	return testAccCephFSDirectoryConfig(fsName, "/volumes/"+dirName) + fmt.Sprintf(`
		resource "ceph_cephfs_snapshot" "test" {
		  vol_name = %q
		  path     = ceph_cephfs_directory.test.path
		  name     = %q
		}
	`, fsName, snapName)
}

func TestAccCephFSSnapshotResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	dirName := acctest.RandomWithPrefix("test-dir")
	snapName := acctest.RandomWithPrefix("test-snap")
	renamedSnapName := acctest.RandomWithPrefix("test-snap-renamed")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkCephFSSnapshotAbsentViaCLI(t, fsName, dirName, renamedSnapName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
			testAccEnsureCephFSVolumesRoot(t, fsName)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccCephFSSnapshotConfig(fsName, dirName, snapName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_cephfs_snapshot.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(snapName),
					),
					statecheck.ExpectKnownValue(
						"ceph_cephfs_snapshot.test",
						tfjsonpath.New("created"),
						knownvalue.NotNull(),
					),
				},
				Check: checkCephFSSnapshotViaCLI(t, fsName, dirName, snapName, true),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccCephFSSnapshotConfig(fsName, dirName, snapName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccCephFSSnapshotConfig(fsName, dirName, renamedSnapName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_cephfs_snapshot.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephFSSnapshotViaCLI(t, fsName, dirName, renamedSnapName, true),
					checkCephFSSnapshotViaCLI(t, fsName, dirName, snapName, false),
				),
			},
		},
	})
}

func TestAccCephFSSnapshotResource_OutOfBandRemoval(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	dirName := acctest.RandomWithPrefix("test-dir-oob")
	snapName := acctest.RandomWithPrefix("test-snap-oob")

	config := testAccCephFSSnapshotConfig(fsName, dirName, snapName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             checkCephFSSnapshotAbsentViaCLI(t, fsName, dirName, snapName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
			testAccEnsureCephFSVolumesRoot(t, fsName)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				Check:           checkCephFSSnapshotViaCLI(t, fsName, dirName, snapName, true),
			},
			{
				PreConfig: func() {
					if err := cephTestClusterCLI.CephFSSubvolumeGroupSnapshotDelete(t.Context(), fsName, dirName, snapName); err != nil {
						t.Fatalf("Failed to delete snapshot out of band: %v", err)
					}
				},
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_cephfs_snapshot.test", plancheck.ResourceActionCreate),
					},
				},
				Check: checkCephFSSnapshotViaCLI(t, fsName, dirName, snapName, true),
			},
		},
	})
}

func TestAccCephFSSnapshotResource_Import(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	dirName := acctest.RandomWithPrefix("test-dir")
	snapName := acctest.RandomWithPrefix("test-snap")
	dirPath := "/volumes/" + dirName

	config := testAccCephFSSnapshotConfig(fsName, dirName, snapName)

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
				Config:          config,
			},
			{
				ConfigVariables:                      testAccProviderConfig(),
				Config:                               config,
				ResourceName:                         "ceph_cephfs_snapshot.test",
				ImportState:                          true,
				ImportStateId:                        fsName + ":" + dirPath + ":" + snapName,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
			},
		},
	})
}

func TestAccCephFSSnapshotResource_DuplicateName(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	dirName := acctest.RandomWithPrefix("test-dir-dup")
	snapName := acctest.RandomWithPrefix("test-snap-dup")

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
				Config:          testAccCephFSSnapshotConfig(fsName, dirName, snapName),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccCephFSSnapshotConfig(fsName, dirName, snapName) + fmt.Sprintf(`
					resource "ceph_cephfs_snapshot" "dup" {
					  vol_name = %q
					  path     = ceph_cephfs_directory.test.path
					  name     = %q
					}
				`, fsName, snapName),
				ExpectError: regexp.MustCompile(`already in use`),
			},
		},
	})
}

func checkCephFSSnapshotViaCLI(t *testing.T, fsName, dirName, snapName string, want bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.CephFSSubvolumeGroupSnapshotExists(t.Context(), fsName, dirName, snapName)
		if err != nil {
			return fmt.Errorf("failed to check snapshot existence: %w", err)
		}
		if exists != want {
			return fmt.Errorf("snapshot %q of /volumes/%s: expected exists=%t, got %t", snapName, dirName, want, exists)
		}
		return nil
	}
}

func checkCephFSSnapshotAbsentViaCLI(t *testing.T, fsName, dirName, snapName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		// The directory is destroyed in the same sweep; a missing
		// directory means the snapshot is gone too.
		dirExists, err := cephTestClusterCLI.CephFSSubvolumeGroupExists(t.Context(), fsName, dirName)
		if err != nil {
			return fmt.Errorf("failed to check directory existence: %w", err)
		}
		if !dirExists {
			return nil
		}
		return checkCephFSSnapshotViaCLI(t, fsName, dirName, snapName, false)(s)
	}
}
