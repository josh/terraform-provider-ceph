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

func TestAccCephFSSubvolumeGroupResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	groupName := acctest.RandomWithPrefix("test-group")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSSubvolumeGroupDestroy(t, fsName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_cephfs_subvolume_group" "test" {
					  name     = %q
					  vol_name = %q

					  timeouts = {
					    create = "5m"
					    delete = "5m"
					  }
					}
				`, groupName, fsName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume_group.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(groupName),
					),
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume_group.test",
						tfjsonpath.New("vol_name"),
						knownvalue.StringExact(fsName),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephFSSubvolumeGroupExists(t, fsName, groupName),
					resource.TestCheckResourceAttr("ceph_cephfs_subvolume_group.test", "name", groupName),
					resource.TestCheckResourceAttr("ceph_cephfs_subvolume_group.test", "vol_name", fsName),
				),
			},
		},
	})
}

func TestAccCephFSSubvolumeGroupResource_Import(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	groupName := acctest.RandomWithPrefix("test-group")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSSubvolumeGroupDestroy(t, fsName),
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
					resource "ceph_cephfs_subvolume_group" "test" {
					  name     = %q
					  vol_name = %q

					  timeouts = {
					    create = "5m"
					    delete = "5m"
					  }
					}
				`, groupName, fsName),
				ResourceName:  "ceph_cephfs_subvolume_group.test",
				ImportState:   true,
				ImportStateId: fsName + "/" + groupName,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["name"] != groupName {
						return fmt.Errorf("expected name %q, got %q", groupName, attrs["name"])
					}
					if attrs["vol_name"] != fsName {
						return fmt.Errorf("expected vol_name %q, got %q", fsName, attrs["vol_name"])
					}
					return nil
				},
			},
		},
	})
}

func TestAccCephFSSubvolumeGroupResource_Size(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	groupName := acctest.RandomWithPrefix("test-group")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSSubvolumeGroupDestroy(t, fsName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_cephfs_subvolume_group" "test" {
					  name     = %q
					  vol_name = %q
					  size     = 1073741824

					  timeouts = {
					    create = "5m"
					    update = "5m"
					    delete = "5m"
					  }
					}
				`, groupName, fsName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume_group.test",
						tfjsonpath.New("size"),
						knownvalue.Int64Exact(1073741824),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephFSSubvolumeGroupExists(t, fsName, groupName),
					resource.TestCheckResourceAttr("ceph_cephfs_subvolume_group.test", "size", "1073741824"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_cephfs_subvolume_group" "test" {
					  name     = %q
					  vol_name = %q
					  size     = 2147483648

					  timeouts = {
					    create = "5m"
					    update = "5m"
					    delete = "5m"
					  }
					}
				`, groupName, fsName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_cephfs_subvolume_group.test",
						tfjsonpath.New("size"),
						knownvalue.Int64Exact(2147483648),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephFSSubvolumeGroupExists(t, fsName, groupName),
					resource.TestCheckResourceAttr("ceph_cephfs_subvolume_group.test", "size", "2147483648"),
				),
			},
		},
	})
}

func checkCephFSSubvolumeGroupExists(t *testing.T, fsName string, groupName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.CephFSSubvolumeGroupExists(t.Context(), fsName, groupName)
		if err != nil {
			return fmt.Errorf("failed to check CephFS subvolume group existence: %w", err)
		}

		if !exists {
			return fmt.Errorf("CephFS subvolume group %q not found in %q", groupName, fsName)
		}

		return nil
	}
}

func testAccCheckCephFSSubvolumeGroupDestroy(t *testing.T, fsName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_cephfs_subvolume_group" {
				continue
			}

			groupName := rs.Primary.Attributes["name"]

			exists, err := cephTestClusterCLI.CephFSSubvolumeGroupExists(ctx, fsName, groupName)
			if err != nil {
				return fmt.Errorf("failed to check CephFS subvolume group existence: %w", err)
			}

			if exists {
				return fmt.Errorf("CephFS subvolume group %q still exists in %q", groupName, fsName)
			}
		}

		return nil
	}
}

func TestAccCephFSSubvolumeGroupResource_OutOfBandDeletion(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	groupName := acctest.RandomWithPrefix("test-group-oob")

	config := testAccProviderConfigBlock + fmt.Sprintf(`
		resource "ceph_cephfs_subvolume_group" "test" {
		  name     = %q
		  vol_name = %q

		  timeouts = {
		    create = "5m"
		    delete = "5m"
		  }
		}
	`, groupName, fsName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSSubvolumeGroupDestroy(t, fsName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				Check:           checkCephFSSubvolumeGroupExists(t, fsName, groupName),
			},
			{
				PreConfig: func() {
					if err := cephTestClusterCLI.CephFSSubvolumeGroupDelete(t.Context(), fsName, groupName); err != nil {
						t.Fatalf("Failed to delete subvolume group out of band: %v", err)
					}
				},
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_cephfs_subvolume_group.test", plancheck.ResourceActionCreate),
					},
				},
				Check: checkCephFSSubvolumeGroupExists(t, fsName, groupName),
			},
		},
	})
}
