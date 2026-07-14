package main

import (
	"context"
	"errors"
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
	"github.com/josh/terraform-provider-ceph/internal/cephcli"
)

const testAccRGWRoleAssumePolicy = `jsonencode({
  Version = "2012-10-17"
  Statement = [{
    Effect    = "Allow"
    Principal = { AWS = "arn:aws:iam:::user/%s" }
    Action    = "sts:AssumeRole"
  }]
})`

func TestAccCephRGWRoleResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	roleName := acctest.RandomWithPrefix("test-role")
	principal := acctest.RandomWithPrefix("test-principal")
	assumePolicy := fmt.Sprintf(testAccRGWRoleAssumePolicy, principal)
	otherPolicy := fmt.Sprintf(testAccRGWRoleAssumePolicy, principal+"-other")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWRoleDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_role" "test" {
					  name                        = %q
					  assume_role_policy_document = %s
					}
				`, roleName, assumePolicy),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_role.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(roleName),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_role.test",
						tfjsonpath.New("path"),
						knownvalue.StringExact("/"),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_role.test",
						tfjsonpath.New("max_session_duration"),
						knownvalue.Int64Exact(3600),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_role.test",
						tfjsonpath.New("arn"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_role.test",
						tfjsonpath.New("role_id"),
						knownvalue.NotNull(),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWRoleExists(t, roleName),
					resource.TestCheckResourceAttr("ceph_rgw_role.test", "name", roleName),
					resource.TestCheckResourceAttr("ceph_rgw_role.test", "id", roleName),
					resource.TestCheckResourceAttrSet("ceph_rgw_role.test", "arn"),
					checkCephRGWRoleMaxSessionDuration(t, roleName, 3600),
				),
			},
			// Update the only mutable attribute in place.
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_role" "test" {
					  name                        = %q
					  assume_role_policy_document = %s
					  max_session_duration        = 7200
					}
				`, roleName, assumePolicy),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rgw_role.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_role.test",
						tfjsonpath.New("max_session_duration"),
						knownvalue.Int64Exact(7200),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWRoleExists(t, roleName),
					checkCephRGWRoleMaxSessionDuration(t, roleName, 7200),
				),
			},
			// Re-applying the same config must be a no-op (guards against JSON drift).
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_role" "test" {
					  name                        = %q
					  assume_role_policy_document = %s
					  max_session_duration        = 7200
					}
				`, roleName, assumePolicy),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			// Changing the trust policy forces replacement.
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_role" "test" {
					  name                        = %q
					  assume_role_policy_document = %s
					  max_session_duration        = 7200
					}
				`, roleName, otherPolicy),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rgw_role.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWRoleExists(t, roleName),
				),
			},
		},
	})
}

func TestAccCephRGWRoleResource_customPath(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	roleName := acctest.RandomWithPrefix("test-role-path")
	assumePolicy := fmt.Sprintf(testAccRGWRoleAssumePolicy, "someuser")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWRoleDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_role" "test" {
					  name                        = %q
					  path                        = "/application/"
					  assume_role_policy_document = %s
					}
				`, roleName, assumePolicy),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_role.test",
						tfjsonpath.New("path"),
						knownvalue.StringExact("/application/"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWRoleExists(t, roleName),
					resource.TestCheckResourceAttr("ceph_rgw_role.test", "path", "/application/"),
				),
			},
		},
	})
}

func TestAccCephRGWRoleResource_nonHourMaxSessionDuration(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	roleName := acctest.RandomWithPrefix("test-role")
	principal := acctest.RandomWithPrefix("test-principal")
	assumePolicy := fmt.Sprintf(testAccRGWRoleAssumePolicy, principal)

	config := testAccProviderConfigBlock + fmt.Sprintf(`
		resource "ceph_rgw_role" "test" {
		  name                        = %q
		  assume_role_policy_document = %s
		  max_session_duration        = 3603
		}
	`, roleName, assumePolicy)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWRoleDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_role.test",
						tfjsonpath.New("max_session_duration"),
						knownvalue.Int64Exact(3603),
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

func TestAccCephRGWRoleResource_invalidMaxSessionDuration(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	roleName := acctest.RandomWithPrefix("test-role-invalid")
	assumePolicy := fmt.Sprintf(testAccRGWRoleAssumePolicy, "someuser")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_role" "test" {
					  name                        = %q
					  assume_role_policy_document = %s
					  max_session_duration        = 60
					}
				`, roleName, assumePolicy),
				ExpectError: regexp.MustCompile(`(?i)max_session_duration`),
			},
		},
	})
}

func TestAccCephRGWRoleResourceImport(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	roleName := acctest.RandomWithPrefix("test-role-import")
	assumePolicy := fmt.Sprintf(testAccRGWRoleAssumePolicy, "someuser")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWRoleDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_role" "test" {
					  name                        = %q
					  assume_role_policy_document = %s
					}
				`, roleName, assumePolicy),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_role" "test" {
					  name                        = %q
					  assume_role_policy_document = %s
					}
				`, roleName, assumePolicy),
				ResourceName:                         "ceph_rgw_role.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateId:                        roleName,
				// Ceph may re-serialize the stored trust policy; the managed
				// resource tolerates that via semantic comparison, but the
				// import verifier compares raw strings.
				ImportStateVerifyIgnore: []string{"assume_role_policy_document"},
			},
		},
	})
}

func testAccCheckCephRGWRoleDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_rgw_role" {
				continue
			}

			exists, err := cephTestClusterCLI.RGWRoleExists(ctx, rs.Primary.Attributes["name"])
			if err != nil {
				return err
			}
			if exists {
				return fmt.Errorf("ceph_rgw_role resource %s still exists", rs.Primary.Attributes["name"])
			}
		}
		return nil
	}
}

func checkCephRGWRoleExists(t *testing.T, name string) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.RGWRoleExists(t.Context(), name)
		if err != nil {
			return fmt.Errorf("RGW role %s existence check failed: %w", name, err)
		}
		if !exists {
			return fmt.Errorf("RGW role %s does not exist", name)
		}
		return nil
	}
}

func checkCephRGWRoleMaxSessionDuration(t *testing.T, name string, expected int64) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		role, err := cephTestClusterCLI.RGWRoleGet(t.Context(), name)
		if err != nil {
			return fmt.Errorf("radosgw-admin failed to get role %s: %w", name, err)
		}
		if role.MaxSessionDuration != expected {
			return fmt.Errorf("expected role %s max_session_duration=%d, got %d", name, expected, role.MaxSessionDuration)
		}
		return nil
	}
}

func createTestRGWRoleDirectly(t *testing.T, name, path, assumePolicyDoc string) {
	t.Helper()

	if err := cephTestClusterCLI.RGWRoleCreate(t.Context(), name, path, assumePolicyDoc); err != nil {
		t.Fatalf("Failed to pre-create test RGW role: %v", err)
	}

	t.Logf("Pre-created test RGW role: %s", name)

	testCleanup(t, func(ctx context.Context) {
		if err := cephTestClusterCLI.RGWRoleDelete(ctx, name); err != nil && !errors.Is(err, cephcli.ErrRGWRoleNotFound) {
			t.Fatalf("Failed to cleanup RGW role %s: %v", name, err)
		}
	})
}
