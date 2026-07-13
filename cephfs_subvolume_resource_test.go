package main

import (
	"context"
	"fmt"
	"regexp"
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

func TestAccCephFSSubvolumeResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSSubvolumeDestroy(t, fsName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_cephfs_subvolume" "test" {
					  name     = %q
					  vol_name = %q

					  timeouts = {
					    create = "5m"
					    delete = "5m"
					  }
					}
				`, subvolName, fsName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(subvolName),
					),
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume.test",
						tfjsonpath.New("vol_name"),
						knownvalue.StringExact(fsName),
					),
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume.test",
						tfjsonpath.New("path"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume.test",
						tfjsonpath.New("state"),
						knownvalue.NotNull(),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephFSSubvolumeExists(t, fsName, subvolName),
					resource.TestCheckResourceAttr("ceph_cephfs_subvolume.test", "name", subvolName),
					resource.TestCheckResourceAttr("ceph_cephfs_subvolume.test", "vol_name", fsName),
					resource.TestCheckResourceAttrSet("ceph_cephfs_subvolume.test", "path"),
					resource.TestCheckResourceAttrSet("ceph_cephfs_subvolume.test", "data_pool"),
					resource.TestCheckResourceAttrSet("ceph_cephfs_subvolume.test", "state"),
				),
			},
		},
	})
}

func TestAccCephFSSubvolumeResource_Import(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSSubvolumeDestroy(t, fsName),
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
					resource "ceph_cephfs_subvolume" "test" {
					  name     = %q
					  vol_name = %q

					  timeouts = {
					    create = "5m"
					    delete = "5m"
					  }
					}
				`, subvolName, fsName),
				ResourceName:  "ceph_cephfs_subvolume.test",
				ImportState:   true,
				ImportStateId: fsName + "/" + subvolName,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["name"] != subvolName {
						return fmt.Errorf("expected name %q, got %q", subvolName, attrs["name"])
					}
					if attrs["vol_name"] != fsName {
						return fmt.Errorf("expected vol_name %q, got %q", fsName, attrs["vol_name"])
					}
					if attrs["path"] == "" {
						return fmt.Errorf("expected path to be set")
					}
					if attrs["data_pool"] == "" {
						return fmt.Errorf("expected data_pool to be set")
					}
					if attrs["state"] == "" {
						return fmt.Errorf("expected state to be set")
					}
					return nil
				},
			},
		},
	})
}

func TestAccCephFSSubvolumeResource_Size(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSSubvolumeDestroy(t, fsName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_cephfs_subvolume" "test" {
					  name     = %q
					  vol_name = %q
					  size     = 1073741824

					  timeouts = {
					    create = "5m"
					    delete = "5m"
					  }
					}
				`, subvolName, fsName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume.test",
						tfjsonpath.New("size"),
						knownvalue.Int64Exact(1073741824),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephFSSubvolumeExists(t, fsName, subvolName),
					resource.TestCheckResourceAttr("ceph_cephfs_subvolume.test", "size", "1073741824"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_cephfs_subvolume" "test" {
					  name     = %q
					  vol_name = %q
					  size     = 2147483648

					  timeouts = {
					    create = "5m"
					    delete = "5m"
					  }
					}
				`, subvolName, fsName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_cephfs_subvolume.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume.test",
						tfjsonpath.New("size"),
						knownvalue.Int64Exact(2147483648),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephFSSubvolumeExists(t, fsName, subvolName),
					resource.TestCheckResourceAttr("ceph_cephfs_subvolume.test", "size", "2147483648"),
				),
			},
		},
	})
}

func TestAccCephFSSubvolumeResource_zeroSizeRejected(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol-zero")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_cephfs_subvolume" "test" {
					  name     = %q
					  vol_name = %q
					  size     = 0
					}
				`, subvolName, fsName),
				ExpectError: regexp.MustCompile(`(?s)size.*must be at least 1`),
			},
		},
	})
}

func checkCephFSSubvolumeExists(t *testing.T, fsName string, subvolName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.CephFSSubvolumeExists(t.Context(), fsName, subvolName, "")
		if err != nil {
			return fmt.Errorf("failed to check CephFS subvolume existence: %w", err)
		}

		if !exists {
			return fmt.Errorf("CephFS subvolume %q not found in %q", subvolName, fsName)
		}

		return nil
	}
}

func testAccCheckCephFSSubvolumeDestroy(t *testing.T, fsName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_cephfs_subvolume" {
				continue
			}

			subvolName := rs.Primary.Attributes["name"]

			exists, err := cephTestClusterCLI.CephFSSubvolumeExists(ctx, fsName, subvolName, "")
			if err != nil {
				return fmt.Errorf("failed to check CephFS subvolume existence: %w", err)
			}

			if exists {
				return fmt.Errorf("CephFS subvolume %q still exists in %q", subvolName, fsName)
			}
		}

		return nil
	}
}

func TestAccCephFSSubvolumeResource_OutOfBandDeletion(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol-oob")

	config := testAccProviderConfigBlock + fmt.Sprintf(`
		resource "ceph_cephfs_subvolume" "test" {
		  name     = %q
		  vol_name = %q

		  timeouts = {
		    create = "5m"
		    delete = "5m"
		  }
		}
	`, subvolName, fsName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSSubvolumeDestroy(t, fsName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				Check:           checkCephFSSubvolumeExists(t, fsName, subvolName),
			},
			{
				PreConfig: func() {
					if err := cephTestClusterCLI.CephFSSubvolumeDelete(t.Context(), fsName, subvolName, ""); err != nil {
						t.Fatalf("Failed to delete subvolume out of band: %v", err)
					}
				},
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_cephfs_subvolume.test", plancheck.ResourceActionCreate),
					},
				},
				Check: checkCephFSSubvolumeExists(t, fsName, subvolName),
			},
		},
	})
}

func TestAccCephFSSubvolumeResource_TimeoutsOnlyUpdate(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol-touts")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSSubvolumeDestroy(t, fsName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_cephfs_subvolume" "test" {
					  name     = %q
					  vol_name = %q
					}
				`, subvolName, fsName),
				Check: checkCephFSSubvolumeExists(t, fsName, subvolName),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_cephfs_subvolume" "test" {
					  name     = %q
					  vol_name = %q

					  timeouts = {
					    create = "10m"
					    delete = "10m"
					  }
					}
				`, subvolName, fsName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_cephfs_subvolume.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: checkCephFSSubvolumeExists(t, fsName, subvolName),
			},
		},
	})
}

func TestAccCephFSSubvolumeResource_InGroup(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	groupName := acctest.RandomWithPrefix("test-group")
	subvolName := acctest.RandomWithPrefix("test-subvol")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)

			if err := cephTestClusterCLI.CephFSSubvolumeGroupCreate(t.Context(), fsName, groupName); err != nil {
				t.Fatalf("Failed to create subvolume group: %v", err)
			}
			t.Cleanup(func() {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				if err := cephTestClusterCLI.CephFSSubvolumeGroupDelete(cleanupCtx, fsName, groupName); err != nil {
					t.Errorf("Failed to delete subvolume group: %v", err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_cephfs_subvolume" "test" {
					  name       = %q
					  vol_name   = %q
					  group_name = %q

					  timeouts = {
					    create = "5m"
					    delete = "5m"
					  }
					}
				`, subvolName, fsName, groupName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume.test",
						tfjsonpath.New("group_name"),
						knownvalue.StringExact(groupName),
					),
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume.test",
						tfjsonpath.New("path"),
						knownvalue.StringRegexp(regexp.MustCompile(`^/volumes/`+regexp.QuoteMeta(groupName)+`/`)),
					),
				},
				Check: func(s *terraform.State) error {
					exists, err := cephTestClusterCLI.CephFSSubvolumeExists(t.Context(), fsName, subvolName, groupName)
					if err != nil {
						return fmt.Errorf("failed to check subvolume existence: %w", err)
					}
					if !exists {
						return fmt.Errorf("subvolume %q not found in group %q", subvolName, groupName)
					}
					return nil
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_cephfs_subvolume" "test" {
					  name       = %q
					  vol_name   = %q
					  group_name = %q

					  timeouts = {
					    create = "5m"
					    delete = "5m"
					  }
					}
				`, subvolName, fsName, groupName),
				ResourceName:  "ceph_cephfs_subvolume.test",
				ImportState:   true,
				ImportStateId: fsName + "/" + groupName + "/" + subvolName,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["group_name"] != groupName {
						return fmt.Errorf("expected group_name %q, got %q", groupName, attrs["group_name"])
					}
					if attrs["name"] != subvolName {
						return fmt.Errorf("expected name %q, got %q", subvolName, attrs["name"])
					}
					return nil
				},
			},
		},
	})
}

func TestAccCephFSSubvolumeResource_ModeUidGid(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol")

	config := func(mode string) string {
		return testAccProviderConfigBlock + fmt.Sprintf(`
			resource "ceph_cephfs_subvolume" "test" {
			  name     = %q
			  vol_name = %q
			  mode     = %q
			  uid      = 1000
			  gid      = 1000

			  timeouts = {
			    create = "5m"
			    delete = "5m"
			  }
			}
		`, subvolName, fsName, mode)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSSubvolumeDestroy(t, fsName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config("750"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume.test",
						tfjsonpath.New("mode"),
						knownvalue.StringExact("750"),
					),
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume.test",
						tfjsonpath.New("uid"),
						knownvalue.Int64Exact(1000),
					),
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume.test",
						tfjsonpath.New("gid"),
						knownvalue.Int64Exact(1000),
					),
				},
				Check: func(s *terraform.State) error {
					info, err := cephTestClusterCLI.CephFSSubvolumeInfo(t.Context(), fsName, subvolName, "")
					if err != nil {
						return fmt.Errorf("failed to get subvolume info: %w", err)
					}
					if info.Mode&0o7777 != 0o750 {
						return fmt.Errorf("expected mode 750, got %o", info.Mode&0o7777)
					}
					if info.UID != 1000 || info.GID != 1000 {
						return fmt.Errorf("expected uid/gid 1000/1000, got %d/%d", info.UID, info.GID)
					}
					return nil
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config("750"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config("700"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_cephfs_subvolume.test", plancheck.ResourceActionReplace),
					},
				},
			},
		},
	})
}

func TestAccCephFSSubvolumeResource_ModeDefaults(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol")

	config := testAccProviderConfigBlock + fmt.Sprintf(`
		resource "ceph_cephfs_subvolume" "test" {
		  name     = %q
		  vol_name = %q
		}
	`, subvolName, fsName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSSubvolumeDestroy(t, fsName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume.test",
						tfjsonpath.New("mode"),
						knownvalue.StringExact("755"),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccCephFSSubvolumeResource_InvalidMode(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_cephfs_subvolume" "test" {
					  name     = "invalid-mode-test"
					  vol_name = %q
					  mode     = "0755"
					}
				`, testSharedCephFSName),
				ExpectError: regexp.MustCompile(`octal permission string`),
			},
		},
	})
}
