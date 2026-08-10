package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/josh/terraform-provider-ceph/internal/cephcli"
	"github.com/josh/terraform-provider-ceph/internal/restapi"
)

var (
	testAccRGWRoleContractOnce  sync.Once
	testAccRGWRoleAccountScoped bool
	testAccRGWRoleContractErr   error
)

const testAccRGWRoleAssumePolicy = `jsonencode({
  Version = "2012-10-17"
  Statement = [{
    Effect    = "Allow"
    Principal = { AWS = "arn:aws:iam::%s:user/%s" }
    Action    = "sts:AssumeRole"
  }]
})`

func testAccRGWRoleUsesAccountScope(t *testing.T) bool {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests require TF_ACC")
	}

	testAccRGWRoleContractOnce.Do(func() {
		endpoint, err := url.Parse(testDashboardURL)
		if err != nil {
			testAccRGWRoleContractErr = fmt.Errorf("failed to parse dashboard URL: %w", err)
			return
		}

		client := &restapi.Client{}
		if err := client.Configure(
			t.Context(),
			[]*url.URL{endpoint},
			"admin",
			"password",
			"",
			"",
			"",
			time.Hour,
		); err != nil {
			testAccRGWRoleContractErr = fmt.Errorf("failed to configure dashboard client: %w", err)
			return
		}

		_, err = client.RGWListRoles(t.Context(), "")
		switch {
		case err == nil:
		case errors.Is(err, restapi.ErrRGWRoleAccountIDRequired):
			testAccRGWRoleAccountScoped = true
		default:
			testAccRGWRoleContractErr = fmt.Errorf("failed to detect RGW role API contract: %w", err)
		}
	})
	if testAccRGWRoleContractErr != nil {
		t.Fatalf("Failed to detect RGW role API contract: %v", testAccRGWRoleContractErr)
	}

	return testAccRGWRoleAccountScoped
}

func testAccRGWRoleAccount(t *testing.T) string {
	t.Helper()

	if !testAccRGWRoleUsesAccountScope(t) {
		return ""
	}

	accountID := "RGW" + acctest.RandStringFromCharSet(17, "0123456789")
	accountName := acctest.RandomWithPrefix("test-role-account")
	if err := cephTestClusterCLI.RGWAccountCreate(t.Context(), accountID, accountName); err != nil {
		t.Fatalf("Failed to create test RGW account: %v", err)
	}

	testCleanup(t, func(ctx context.Context) {
		if err := cephTestClusterCLI.RGWAccountDelete(ctx, accountID); err != nil {
			t.Fatalf("Failed to cleanup RGW account %s: %v", accountID, err)
		}
	})

	return accountID
}

func testAccRGWRoleAccountAttribute(accountID string) string {
	if accountID == "" {
		return ""
	}

	return fmt.Sprintf("  account_id = %q\n", accountID)
}

func testAccRGWRoleImportID(accountID, roleName string) string {
	if accountID == "" {
		return roleName
	}

	return accountID + "/" + roleName
}

func TestAccCephRGWRoleResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	accountID := testAccRGWRoleAccount(t)
	roleName := acctest.RandomWithPrefix("test-role")
	principal := acctest.RandomWithPrefix("test-principal")
	assumePolicy := fmt.Sprintf(testAccRGWRoleAssumePolicy, accountID, principal)
	otherPolicy := fmt.Sprintf(testAccRGWRoleAssumePolicy, accountID, principal+"-other")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWRoleDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_role" "test" {
					  name                        = %q
					%s
					  assume_role_policy_document = %s
					}
				`, roleName, testAccRGWRoleAccountAttribute(accountID), assumePolicy),
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
					checkCephRGWRoleExists(t, accountID, roleName),
					resource.TestCheckResourceAttr("ceph_rgw_role.test", "name", roleName),
					resource.TestCheckResourceAttr("ceph_rgw_role.test", "account_id", accountID),
					resource.TestCheckResourceAttr("ceph_rgw_role.test", "id", roleName),
					resource.TestCheckResourceAttrSet("ceph_rgw_role.test", "arn"),
					checkCephRGWRoleMaxSessionDuration(t, accountID, roleName, 3600),
				),
			},
			// Update the only mutable attribute in place.
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_role" "test" {
					  name                        = %q
					%s
					  assume_role_policy_document = %s
					  max_session_duration        = 7200
					}
				`, roleName, testAccRGWRoleAccountAttribute(accountID), assumePolicy),
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
					checkCephRGWRoleExists(t, accountID, roleName),
					checkCephRGWRoleMaxSessionDuration(t, accountID, roleName, 7200),
				),
			},
			// Re-applying the same config must be a no-op (guards against JSON drift).
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_role" "test" {
					  name                        = %q
					%s
					  assume_role_policy_document = %s
					  max_session_duration        = 7200
					}
				`, roleName, testAccRGWRoleAccountAttribute(accountID), assumePolicy),
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
					%s
					  assume_role_policy_document = %s
					  max_session_duration        = 7200
					}
				`, roleName, testAccRGWRoleAccountAttribute(accountID), otherPolicy),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rgw_role.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWRoleExists(t, accountID, roleName),
				),
			},
			// Recreate the role after it is deleted outside Terraform.
			{
				PreConfig: func() {
					if err := cephTestClusterCLI.RGWRoleDelete(t.Context(), accountID, roleName); err != nil {
						t.Fatalf("Failed to delete RGW role out of band: %v", err)
					}
					t.Logf("Deleted RGW role %s out of band", roleName)
				},
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_role" "test" {
					  name                        = %q
					%s
					  assume_role_policy_document = %s
					  max_session_duration        = 7200
					}
				`, roleName, testAccRGWRoleAccountAttribute(accountID), otherPolicy),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rgw_role.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWRoleExists(t, accountID, roleName),
					resource.TestCheckResourceAttr("ceph_rgw_role.test", "account_id", accountID),
					checkCephRGWRoleMaxSessionDuration(t, accountID, roleName, 7200),
				),
			},
		},
	})

	// Terraform has destroyed the role, while the scoped account still exists.
	// Verify the live API classifies the role itself as missing.
	endpoint, err := url.Parse(testDashboardURL)
	if err != nil {
		t.Fatalf("Failed to parse dashboard URL: %v", err)
	}

	client := &restapi.Client{}
	if err := client.Configure(t.Context(), []*url.URL{endpoint}, "admin", "password", "", "", "", time.Hour); err != nil {
		t.Fatalf("Failed to configure dashboard client: %v", err)
	}
	if err := client.RGWDeleteRole(t.Context(), accountID, roleName); !errors.Is(err, restapi.ErrNotFound) {
		t.Fatalf("Expected deleting an already missing RGW role to return ErrNotFound, got: %v", err)
	}
}

func TestAccCephRGWRoleResource_customPath(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	accountID := testAccRGWRoleAccount(t)
	roleName := acctest.RandomWithPrefix("test-role-path")
	assumePolicy := fmt.Sprintf(testAccRGWRoleAssumePolicy, accountID, "someuser")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWRoleDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_role" "test" {
					  name                        = %q
					%s
					  path                        = "/application/"
					  assume_role_policy_document = %s
					}
				`, roleName, testAccRGWRoleAccountAttribute(accountID), assumePolicy),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_role.test",
						tfjsonpath.New("path"),
						knownvalue.StringExact("/application/"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWRoleExists(t, accountID, roleName),
					resource.TestCheckResourceAttr("ceph_rgw_role.test", "path", "/application/"),
				),
			},
		},
	})
}

func TestAccCephRGWRoleResource_nonHourMaxSessionDuration(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	accountID := testAccRGWRoleAccount(t)
	roleName := acctest.RandomWithPrefix("test-role")
	principal := acctest.RandomWithPrefix("test-principal")
	assumePolicy := fmt.Sprintf(testAccRGWRoleAssumePolicy, accountID, principal)

	config := testAccProviderConfigBlock + fmt.Sprintf(`
		resource "ceph_rgw_role" "test" {
		  name                        = %q
		%s
		  assume_role_policy_document = %s
		  max_session_duration        = 3603
		}
	`, roleName, testAccRGWRoleAccountAttribute(accountID), assumePolicy)

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
	assumePolicy := fmt.Sprintf(testAccRGWRoleAssumePolicy, "", "someuser")

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

func TestAccCephRGWRoleResource_incompatibleScope(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	accountAttribute := ""
	expectError := regexp.MustCompile(`(?i)requires an account ID`)
	if !testAccRGWRoleUsesAccountScope(t) {
		accountAttribute = testAccRGWRoleAccountAttribute("RGW12345678901234567")
		expectError = regexp.MustCompile(`(?i)does not support account-scoped`)
	}

	roleName := acctest.RandomWithPrefix("test-role-scope")
	assumePolicy := fmt.Sprintf(testAccRGWRoleAssumePolicy, "", "someuser")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_role" "test" {
					  name                        = %q
					%s
					  assume_role_policy_document = %s
					}
				`, roleName, accountAttribute, assumePolicy),
				ExpectError: expectError,
			},
		},
	})
}

func TestAccCephRGWRoleResourceImport(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	accountID := testAccRGWRoleAccount(t)
	roleName := acctest.RandomWithPrefix("test-role-import")
	assumePolicy := fmt.Sprintf(testAccRGWRoleAssumePolicy, accountID, "someuser")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWRoleDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_role" "test" {
					  name                        = %q
					%s
					  assume_role_policy_document = %s
					}
				`, roleName, testAccRGWRoleAccountAttribute(accountID), assumePolicy),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_role" "test" {
					  name                        = %q
					%s
					  assume_role_policy_document = %s
					}
				`, roleName, testAccRGWRoleAccountAttribute(accountID), assumePolicy),
				ResourceName:                         "ceph_rgw_role.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateId:                        testAccRGWRoleImportID(accountID, roleName),
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

			exists, err := cephTestClusterCLI.RGWRoleExists(ctx, rs.Primary.Attributes["account_id"], rs.Primary.Attributes["name"])
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

func checkCephRGWRoleExists(t *testing.T, accountID, name string) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.RGWRoleExists(t.Context(), accountID, name)
		if err != nil {
			return fmt.Errorf("RGW role %s existence check failed: %w", name, err)
		}
		if !exists {
			return fmt.Errorf("RGW role %s does not exist", name)
		}
		return nil
	}
}

func checkCephRGWRoleMaxSessionDuration(t *testing.T, accountID, name string, expected int64) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		role, err := cephTestClusterCLI.RGWRoleGet(t.Context(), accountID, name)
		if err != nil {
			return fmt.Errorf("radosgw-admin failed to get role %s: %w", name, err)
		}
		if role.MaxSessionDuration != expected {
			return fmt.Errorf("expected role %s max_session_duration=%d, got %d", name, expected, role.MaxSessionDuration)
		}
		return nil
	}
}

func createTestRGWRoleDirectly(t *testing.T, accountID, name, path, assumePolicyDoc string) {
	t.Helper()

	if err := cephTestClusterCLI.RGWRoleCreate(t.Context(), accountID, name, path, assumePolicyDoc); err != nil {
		t.Fatalf("Failed to pre-create test RGW role: %v", err)
	}

	t.Logf("Pre-created test RGW role: %s", name)

	testCleanup(t, func(ctx context.Context) {
		if err := cephTestClusterCLI.RGWRoleDelete(ctx, accountID, name); err != nil && !errors.Is(err, cephcli.ErrRGWRoleNotFound) {
			t.Fatalf("Failed to cleanup RGW role %s: %v", name, err)
		}
	})
}
