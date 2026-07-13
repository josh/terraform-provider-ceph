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

func testAccCephFSQuotaSubvolumeConfig(fsName, subvolName string) string {
	return testAccProviderConfigBlock + fmt.Sprintf(`
		resource "ceph_cephfs_subvolume" "test" {
		  name     = %q
		  vol_name = %q

		  timeouts = {
		    create = "5m"
		    delete = "5m"
		  }
		}
	`, subvolName, fsName)
}

func TestAccCephFSQuotaResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol")

	quotaConfig := func(maxBytes, maxFiles int64) string {
		return testAccCephFSQuotaSubvolumeConfig(fsName, subvolName) + fmt.Sprintf(`
			resource "ceph_cephfs_quota" "test" {
			  vol_name  = %q
			  path      = ceph_cephfs_subvolume.test.path
			  max_bytes = %d
			  max_files = %d
			}
		`, fsName, maxBytes, maxFiles)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          quotaConfig(10485760, 1000),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_cephfs_quota.test",
						tfjsonpath.New("max_bytes"),
						knownvalue.Int64Exact(10485760),
					),
					statecheck.ExpectKnownValue(
						"ceph_cephfs_quota.test",
						tfjsonpath.New("max_files"),
						knownvalue.Int64Exact(1000),
					),
				},
				Check: checkCephFSSubvolumeBytesQuota(t, fsName, subvolName, 10485760),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          quotaConfig(20971520, 2000),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_cephfs_quota.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: checkCephFSSubvolumeBytesQuota(t, fsName, subvolName, 20971520),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          quotaConfig(20971520, 2000),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccCephFSQuotaSubvolumeConfig(fsName, subvolName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_cephfs_quota.test", plancheck.ResourceActionDestroy),
					},
				},
				Check: checkCephFSSubvolumeBytesQuotaUnset(t, fsName, subvolName),
			},
		},
	})
}

func TestAccCephFSQuotaResource_Defaults(t *testing.T) {
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
					}
				`, fsName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_cephfs_quota.test",
						tfjsonpath.New("max_bytes"),
						knownvalue.Int64Exact(10485760),
					),
					statecheck.ExpectKnownValue(
						"ceph_cephfs_quota.test",
						tfjsonpath.New("max_files"),
						knownvalue.Int64Exact(0),
					),
				},
			},
		},
	})
}

func TestAccCephFSQuotaResource_Drift(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol")

	config := testAccCephFSQuotaSubvolumeConfig(fsName, subvolName) + fmt.Sprintf(`
		resource "ceph_cephfs_quota" "test" {
		  vol_name  = %q
		  path      = ceph_cephfs_subvolume.test.path
		  max_bytes = 10485760
		}
	`, fsName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				Check:           checkCephFSSubvolumeBytesQuota(t, fsName, subvolName, 10485760),
			},
			{
				PreConfig: func() {
					if err := cephTestClusterCLI.CephFSSubvolumeResize(t.Context(), fsName, subvolName, "31457280", ""); err != nil {
						t.Fatalf("Failed to resize subvolume out of band: %v", err)
					}
				},
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_cephfs_quota.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: checkCephFSSubvolumeBytesQuota(t, fsName, subvolName, 10485760),
			},
		},
	})
}

func TestAccCephFSQuotaResource_OutOfBandPathRemoval(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol-oob")

	config := testAccCephFSQuotaSubvolumeConfig(fsName, subvolName) + fmt.Sprintf(`
		resource "ceph_cephfs_quota" "test" {
		  vol_name  = %q
		  path      = ceph_cephfs_subvolume.test.path
		  max_bytes = 10485760
		}
	`, fsName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				Check:           checkCephFSSubvolumeBytesQuota(t, fsName, subvolName, 10485760),
			},
			{
				// Deleting the subvolume out of band removes the quota's
				// path; refresh must drop the quota from state (via the
				// 500-with-ObjectNotFound mapping) and plan re-creation of
				// both resources.
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
						plancheck.ExpectResourceAction("ceph_cephfs_quota.test", plancheck.ResourceActionCreate),
					},
				},
				Check: checkCephFSSubvolumeBytesQuota(t, fsName, subvolName, 10485760),
			},
		},
	})
}

func TestAccCephFSQuotaResource_Import(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := testSharedCephFSName
	subvolName := acctest.RandomWithPrefix("test-subvol")

	config := testAccCephFSQuotaSubvolumeConfig(fsName, subvolName) + fmt.Sprintf(`
		resource "ceph_cephfs_quota" "test" {
		  vol_name  = %q
		  path      = ceph_cephfs_subvolume.test.path
		  max_bytes = 10485760
		  max_files = 1000
		}
	`, fsName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ResourceName:    "ceph_cephfs_quota.test",
				ImportState:     true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["ceph_cephfs_quota.test"]
					if !ok {
						return "", fmt.Errorf("resource not found in state")
					}
					return fsName + ":" + rs.Primary.Attributes["path"], nil
				},
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["vol_name"] != fsName {
						return fmt.Errorf("expected vol_name %q, got %q", fsName, attrs["vol_name"])
					}
					if attrs["max_bytes"] != "10485760" {
						return fmt.Errorf("expected max_bytes 10485760, got %q", attrs["max_bytes"])
					}
					if attrs["max_files"] != "1000" {
						return fmt.Errorf("expected max_files 1000, got %q", attrs["max_files"])
					}
					return nil
				},
			},
		},
	})
}

func checkCephFSSubvolumeBytesQuota(t *testing.T, fsName, subvolName string, want int64) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		info, err := cephTestClusterCLI.CephFSSubvolumeInfo(t.Context(), fsName, subvolName, "")
		if err != nil {
			return fmt.Errorf("failed to get subvolume info: %w", err)
		}
		got, ok := info.BytesQuotaInt64()
		if !ok {
			return fmt.Errorf("subvolume %s/%s bytes_quota: expected %d, got %v", fsName, subvolName, want, info.BytesQuota)
		}
		if got != want {
			return fmt.Errorf("subvolume %s/%s bytes_quota: expected %d, got %d", fsName, subvolName, want, got)
		}
		return nil
	}
}

func checkCephFSSubvolumeBytesQuotaUnset(t *testing.T, fsName, subvolName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		info, err := cephTestClusterCLI.CephFSSubvolumeInfo(t.Context(), fsName, subvolName, "")
		if err != nil {
			return fmt.Errorf("failed to get subvolume info: %w", err)
		}
		// The MDS reports a cleared quota as "infinite" or 0 depending on
		// version; accept both.
		if got, ok := info.BytesQuotaInt64(); ok && got != 0 {
			return fmt.Errorf("subvolume %s/%s bytes_quota: expected unset, got %d", fsName, subvolName, got)
		}
		return nil
	}
}
