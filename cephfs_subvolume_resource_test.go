package main

import (
	"fmt"
	"testing"

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

			if err := cephTestClusterCLI.CephFSSubvolumeCreate(t.Context(), fsName, subvolName); err != nil {
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

func checkCephFSSubvolumeExists(t *testing.T, fsName string, subvolName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.CephFSSubvolumeExists(t.Context(), fsName, subvolName)
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

			exists, err := cephTestClusterCLI.CephFSSubvolumeExists(ctx, fsName, subvolName)
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
					if err := cephTestClusterCLI.CephFSSubvolumeDelete(t.Context(), fsName, subvolName); err != nil {
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
