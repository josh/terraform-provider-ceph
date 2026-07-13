package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// The dashboard's cephfs client can only create directories where it
// has POSIX write permission. On a fresh cluster /volumes does not
// exist and the filesystem root is not writable for it, so materialize
// /volumes through the volumes plugin with a throwaway group.
func testAccEnsureCephFSVolumesRoot(t *testing.T, fsName string) {
	ctx := t.Context()
	name := acctest.RandomWithPrefix("bootstrap")
	if err := cephTestClusterCLI.CephFSSubvolumeGroupCreate(ctx, fsName, name); err != nil {
		t.Fatalf("Failed to create bootstrap subvolume group: %v", err)
	}
	if err := cephTestClusterCLI.CephFSSubvolumeGroupDelete(ctx, fsName, name); err != nil {
		t.Fatalf("Failed to delete bootstrap subvolume group: %v", err)
	}
}

func testAccCephFSDirectoryConfig(fsName, dirPath string) string {
	return testAccProviderConfigBlock + fmt.Sprintf(`
		resource "ceph_cephfs_directory" "test" {
		  vol_name = %q
		  path     = %q
		}
	`, fsName, dirPath)
}

func TestAccCephFSDirectoryResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	dirName := acctest.RandomWithPrefix("test-dir")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSDirectoryDestroy(t, fsName, dirName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
			testAccEnsureCephFSVolumesRoot(t, fsName)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccCephFSDirectoryConfig(fsName, "/volumes/"+dirName),
				Check:           checkCephFSDirectoryExistsAsGroup(t, fsName, dirName, true),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccCephFSDirectoryConfig(fsName, "/volumes/"+dirName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccCephFSDirectoryResource_AdoptExisting(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	dirName := acctest.RandomWithPrefix("test-dir")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSDirectoryDestroy(t, fsName, dirName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
			testAccEnsureCephFSVolumesRoot(t, fsName)

			if err := cephTestClusterCLI.CephFSSubvolumeGroupCreate(t.Context(), fsName, dirName); err != nil {
				t.Fatalf("Failed to create directory out of band: %v", err)
			}
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccCephFSDirectoryConfig(fsName, "/volumes/"+dirName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_cephfs_directory.test", plancheck.ResourceActionCreate),
					},
				},
				Check: checkCephFSDirectoryExistsAsGroup(t, fsName, dirName, true),
			},
		},
	})
}

func TestAccCephFSDirectoryResource_OutOfBandRemoval(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	dirName := acctest.RandomWithPrefix("test-dir-oob")

	config := testAccCephFSDirectoryConfig(fsName, "/volumes/"+dirName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSDirectoryDestroy(t, fsName, dirName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
			testAccEnsureCephFSVolumesRoot(t, fsName)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				Check:           checkCephFSDirectoryExistsAsGroup(t, fsName, dirName, true),
			},
			{
				PreConfig: func() {
					if err := cephTestClusterCLI.CephFSSubvolumeGroupDelete(t.Context(), fsName, dirName); err != nil {
						t.Fatalf("Failed to delete directory out of band: %v", err)
					}
				},
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_cephfs_directory.test", plancheck.ResourceActionCreate),
					},
				},
				Check: checkCephFSDirectoryExistsAsGroup(t, fsName, dirName, true),
			},
		},
	})
}

func TestAccCephFSDirectoryResource_NestedPath(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	parentName := acctest.RandomWithPrefix("test-dir-parent")

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := cephTestClusterCLI.CephFSSubvolumeGroupDelete(cleanupCtx, fsName, parentName); err != nil {
			t.Errorf("Failed to delete parent directory: %v", err)
		}
	})

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		// The destroy removes only the leaf; the auto-created parent
		// remains, which the final check pins.
		CheckDestroy: checkCephFSDirectoryExistsAsGroup(t, fsName, parentName, true),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
			testAccEnsureCephFSVolumesRoot(t, fsName)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccCephFSDirectoryConfig(fsName, "/volumes/"+parentName+"/nested"),
				Check:           checkCephFSDirectoryExistsAsGroup(t, fsName, parentName, true),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccCephFSDirectoryConfig(fsName, "/volumes/"+parentName+"/nested"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccCephFSDirectoryResource_Import(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	dirName := acctest.RandomWithPrefix("test-dir")
	dirPath := "/volumes/" + dirName

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSDirectoryDestroy(t, fsName, dirName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
			testAccEnsureCephFSVolumesRoot(t, fsName)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccCephFSDirectoryConfig(fsName, dirPath),
			},
			{
				ConfigVariables:                      testAccProviderConfig(),
				Config:                               testAccCephFSDirectoryConfig(fsName, dirPath),
				ResourceName:                         "ceph_cephfs_directory.test",
				ImportState:                          true,
				ImportStateId:                        fsName + ":" + dirPath,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "path",
			},
		},
	})
}

func checkCephFSDirectoryExistsAsGroup(t *testing.T, fsName, dirName string, want bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.CephFSSubvolumeGroupExists(t.Context(), fsName, dirName)
		if err != nil {
			return fmt.Errorf("failed to check directory existence: %w", err)
		}
		if exists != want {
			return fmt.Errorf("directory /volumes/%s in %q: expected exists=%t, got %t", dirName, fsName, want, exists)
		}
		return nil
	}
}

func testAccCheckCephFSDirectoryDestroy(t *testing.T, fsName, dirName string) resource.TestCheckFunc {
	return checkCephFSDirectoryExistsAsGroup(t, fsName, dirName, false)
}
