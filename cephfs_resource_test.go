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

func TestAccCephFSResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := acctest.RandomWithPrefix("test-cephfs")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSDestroy(t),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_cephfs" "test" {
					  name = %q

					  timeouts = {
					    create = "5m"
					    update = "5m"
					    delete = "5m"
					  }
					}
				`, fsName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_cephfs.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(fsName),
					),
					statecheck.ExpectKnownValue(
						"ceph_cephfs.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_cephfs.test",
						tfjsonpath.New("metadata_pool_id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_cephfs.test",
						tfjsonpath.New("data_pool_ids"),
						knownvalue.NotNull(),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephFSExists(t, fsName),
					resource.TestCheckResourceAttr("ceph_cephfs.test", "name", fsName),
					resource.TestCheckResourceAttrSet("ceph_cephfs.test", "id"),
					resource.TestCheckResourceAttrSet("ceph_cephfs.test", "metadata_pool_id"),
				),
			},
			{
				ConfigVariables:                      testAccProviderConfig(),
				ResourceName:                         "ceph_cephfs.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        fsName,
				ImportStateVerifyIdentifierAttribute: "name",
			},
		},
	})
}

func TestAccCephFSResource_NameRequiresReplace(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	fsName := acctest.RandomWithPrefix("test-cephfs-name")
	fsNameUpdated := acctest.RandomWithPrefix("test-cephfs-newname")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephFSDestroy(t),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_cephfs" "test" {
					  name = %q

					  timeouts = {
					    create = "5m"
					    update = "5m"
					    delete = "5m"
					  }
					}
				`, fsName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_cephfs.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(fsName),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephFSExists(t, fsName),
					resource.TestCheckResourceAttr("ceph_cephfs.test", "name", fsName),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_cephfs" "test" {
					  name = %q

					  timeouts = {
					    create = "5m"
					    update = "5m"
					    delete = "5m"
					  }
					}
				`, fsNameUpdated),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_cephfs.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_cephfs.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(fsNameUpdated),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephFSExists(t, fsNameUpdated),
					resource.TestCheckResourceAttr("ceph_cephfs.test", "name", fsNameUpdated),
				),
			},
		},
	})
}

func checkCephFSExists(t *testing.T, fsName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.CephFSExists(t.Context(), fsName)
		if err != nil {
			return fmt.Errorf("failed to check CephFS existence: %w", err)
		}

		if !exists {
			return fmt.Errorf("CephFS %q not found in Ceph", fsName)
		}

		return nil
	}
}

func testAccCheckCephFSDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_cephfs" {
				continue
			}

			fsName := rs.Primary.Attributes["name"]

			exists, err := cephTestClusterCLI.CephFSExists(ctx, fsName)
			if err != nil {
				return fmt.Errorf("failed to check CephFS existence: %w", err)
			}

			if exists {
				return fmt.Errorf("CephFS %q still exists in Ceph", fsName)
			}
		}

		testAccPostCheckWaitForTasks(t)

		return nil
	}
}
