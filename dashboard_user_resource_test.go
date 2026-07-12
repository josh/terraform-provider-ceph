package main

import (
	"context"
	"fmt"
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
)

const testAccDashboardUserPassword = "Terraform-Test-Passw0rd!"

func TestAccCephDashboardUserResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	username := acctest.RandomWithPrefix("test-dash-user")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDashboardUserDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_dashboard_user" "test" {
					  username = %q
					  password = %q
					  name     = "Test User"
					  email    = "test@example.com"
					  roles    = ["read-only"]
					}
				`, username, testAccDashboardUserPassword),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_dashboard_user.test",
						tfjsonpath.New("username"),
						knownvalue.StringExact(username),
					),
					statecheck.ExpectKnownValue(
						"ceph_dashboard_user.test",
						tfjsonpath.New("enabled"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"ceph_dashboard_user.test",
						tfjsonpath.New("pwd_update_required"),
						knownvalue.Bool(false),
					),
					statecheck.ExpectKnownValue(
						"ceph_dashboard_user.test",
						tfjsonpath.New("roles"),
						knownvalue.SetExact([]knownvalue.Check{
							knownvalue.StringExact("read-only"),
						}),
					),
				},
				Check: checkDashboardUser(t, username, func(user *cephcli.DashboardUser) error {
					if !user.Enabled {
						return fmt.Errorf("expected user to be enabled")
					}
					if user.PwdUpdateRequired {
						return fmt.Errorf("expected pwdUpdateRequired to be false")
					}
					if len(user.Roles) != 1 || user.Roles[0] != "read-only" {
						return fmt.Errorf("expected roles [read-only], got %v", user.Roles)
					}
					if user.Name == nil || *user.Name != "Test User" {
						return fmt.Errorf("expected name 'Test User', got %v", user.Name)
					}
					return nil
				}),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_dashboard_user" "test" {
					  username = %q
					  password = %q
					  name     = "Renamed User"
					  email    = "renamed@example.com"
					  roles    = ["block-manager", "pool-manager"]
					}
				`, username, testAccDashboardUserPassword),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_dashboard_user.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: checkDashboardUser(t, username, func(user *cephcli.DashboardUser) error {
					if len(user.Roles) != 2 {
						return fmt.Errorf("expected 2 roles, got %v", user.Roles)
					}
					if user.Name == nil || *user.Name != "Renamed User" {
						return fmt.Errorf("expected name 'Renamed User', got %v", user.Name)
					}
					return nil
				}),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_dashboard_user" "test" {
					  username = %q
					  password = %q
					  roles    = ["block-manager", "pool-manager"]
					}
				`, username, testAccDashboardUserPassword),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_dashboard_user.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: checkDashboardUser(t, username, func(user *cephcli.DashboardUser) error {
					if user.Name != nil {
						return fmt.Errorf("expected name to be cleared, got %q", *user.Name)
					}
					if user.Email != nil {
						return fmt.Errorf("expected email to be cleared, got %q", *user.Email)
					}
					return nil
				}),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_dashboard_user" "test" {
					  username = %q
					  password = %q
					  roles    = ["block-manager", "pool-manager"]
					  enabled  = false
					}
				`, username, testAccDashboardUserPassword),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_dashboard_user.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: checkDashboardUser(t, username, func(user *cephcli.DashboardUser) error {
					if user.Enabled {
						return fmt.Errorf("expected user to be disabled")
					}
					return nil
				}),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_dashboard_user" "test" {
					  username = %q
					  password = %q
					  roles    = ["block-manager", "pool-manager"]
					  enabled  = false
					}
				`, username, testAccDashboardUserPassword),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccCephDashboardUserResource_Import(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	username := acctest.RandomWithPrefix("test-dash-user")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccDashboardUserCLIFixture(t, username)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_dashboard_user" "test" {
					  username = %q
					  password = %q
					  roles    = ["read-only"]
					}
				`, username, testAccDashboardUserPassword),
				ResourceName:  "ceph_dashboard_user.test",
				ImportState:   true,
				ImportStateId: username,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["username"] != username {
						return fmt.Errorf("expected username %q, got %q", username, attrs["username"])
					}
					if attrs["roles.#"] != "1" {
						return fmt.Errorf("expected 1 role, got %q", attrs["roles.#"])
					}
					if attrs["enabled"] != "true" {
						return fmt.Errorf("expected enabled true, got %q", attrs["enabled"])
					}
					if attrs["password"] != "" {
						return fmt.Errorf("expected password to be unset after import")
					}
					return nil
				},
			},
		},
	})
}

func TestAccCephDashboardUserResource_OutOfBandDeletion(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	username := acctest.RandomWithPrefix("test-dash-user-oob")

	config := testAccProviderConfigBlock + fmt.Sprintf(`
		resource "ceph_dashboard_user" "test" {
		  username = %q
		  password = %q
		  roles    = ["read-only"]
		}
	`, username, testAccDashboardUserPassword)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDashboardUserDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				Check:           checkDashboardUserExists(t, username),
			},
			{
				PreConfig: func() {
					if err := cephTestClusterCLI.DashboardUserDelete(t.Context(), username); err != nil {
						t.Fatalf("Failed to delete dashboard user out of band: %v", err)
					}
				},
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_dashboard_user.test", plancheck.ResourceActionCreate),
					},
				},
				Check: checkDashboardUserExists(t, username),
			},
		},
	})
}

func testAccDashboardUserCLIFixture(t *testing.T, username string) {
	if err := cephTestClusterCLI.DashboardUserCreate(t.Context(), username, testAccDashboardUserPassword, "read-only"); err != nil {
		t.Fatalf("Failed to create dashboard user: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := cephTestClusterCLI.DashboardUserDelete(cleanupCtx, username); err != nil {
			t.Errorf("Failed to delete dashboard user: %v", err)
		}
	})
}

func checkDashboardUser(t *testing.T, username string, check func(*cephcli.DashboardUser) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		user, err := cephTestClusterCLI.DashboardUserShow(t.Context(), username)
		if err != nil {
			return fmt.Errorf("failed to show dashboard user: %w", err)
		}
		return check(user)
	}
}

func checkDashboardUserExists(t *testing.T, username string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.DashboardUserExists(t.Context(), username)
		if err != nil {
			return fmt.Errorf("failed to check dashboard user existence: %w", err)
		}
		if !exists {
			return fmt.Errorf("dashboard user %q not found", username)
		}
		return nil
	}
}

func testAccCheckDashboardUserDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_dashboard_user" {
				continue
			}

			username := rs.Primary.Attributes["username"]

			exists, err := cephTestClusterCLI.DashboardUserExists(ctx, username)
			if err != nil {
				return fmt.Errorf("failed to check dashboard user existence: %w", err)
			}

			if exists {
				return fmt.Errorf("dashboard user %q still exists", username)
			}
		}

		return nil
	}
}
