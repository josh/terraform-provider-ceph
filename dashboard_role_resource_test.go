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

func TestAccCephDashboardRoleResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	roleName := acctest.RandomWithPrefix("test-dash-role")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDashboardRoleDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_dashboard_role" "test" {
					  name        = %q
					  description = "Test role"
					  scopes_permissions = {
					    "pool"      = ["read", "create", "update", "delete"]
					    "rbd-image" = ["read"]
					  }
					}
				`, roleName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_dashboard_role.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(roleName),
					),
					statecheck.ExpectKnownValue(
						"ceph_dashboard_role.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("Test role"),
					),
					statecheck.ExpectKnownValue(
						"ceph_dashboard_role.test",
						tfjsonpath.New("scopes_permissions"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"pool": knownvalue.SetExact([]knownvalue.Check{
								knownvalue.StringExact("read"),
								knownvalue.StringExact("create"),
								knownvalue.StringExact("update"),
								knownvalue.StringExact("delete"),
							}),
							"rbd-image": knownvalue.SetExact([]knownvalue.Check{
								knownvalue.StringExact("read"),
							}),
						}),
					),
				},
				Check: checkDashboardRole(t, roleName, func(role *cephcli.DashboardRole) error {
					if role.Description == nil || *role.Description != "Test role" {
						return fmt.Errorf("expected description 'Test role', got %v", role.Description)
					}
					if len(role.ScopesPermissions) != 2 {
						return fmt.Errorf("expected 2 scopes, got %v", role.ScopesPermissions)
					}
					if len(role.ScopesPermissions["pool"]) != 4 {
						return fmt.Errorf("expected 4 pool permissions, got %v", role.ScopesPermissions["pool"])
					}
					return nil
				}),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_dashboard_role" "test" {
					  name        = %q
					  description = "Updated role"
					  scopes_permissions = {
					    "pool" = ["read", "update"]
					  }
					}
				`, roleName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_dashboard_role.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: checkDashboardRole(t, roleName, func(role *cephcli.DashboardRole) error {
					if role.Description == nil || *role.Description != "Updated role" {
						return fmt.Errorf("expected description 'Updated role', got %v", role.Description)
					}
					if _, ok := role.ScopesPermissions["rbd-image"]; ok {
						return fmt.Errorf("expected rbd-image scope to be removed, got %v", role.ScopesPermissions)
					}
					if len(role.ScopesPermissions["pool"]) != 2 {
						return fmt.Errorf("expected 2 pool permissions, got %v", role.ScopesPermissions["pool"])
					}
					return nil
				}),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_dashboard_role" "test" {
					  name        = %q
					  description = "Updated role"
					  scopes_permissions = {
					    "pool" = ["read", "update"]
					  }
					}
				`, roleName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccCephDashboardRoleResource_Import(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	roleName := acctest.RandomWithPrefix("test-dash-role")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccDashboardRoleCLIFixture(t, roleName)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_dashboard_role" "test" {
					  name        = %q
					  description = "Fixture role"
					  scopes_permissions = {
					    "pool" = ["read"]
					  }
					}
				`, roleName),
				ResourceName:  "ceph_dashboard_role.test",
				ImportState:   true,
				ImportStateId: roleName,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["name"] != roleName {
						return fmt.Errorf("expected name %q, got %q", roleName, attrs["name"])
					}
					if attrs["description"] != "Fixture role" {
						return fmt.Errorf("expected description 'Fixture role', got %q", attrs["description"])
					}
					if attrs["scopes_permissions.pool.#"] != "1" {
						return fmt.Errorf("expected 1 pool permission, got %q", attrs["scopes_permissions.pool.#"])
					}
					return nil
				},
			},
		},
	})
}

func TestAccCephDashboardRoleResource_OutOfBandDeletion(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	roleName := acctest.RandomWithPrefix("test-dash-role-oob")

	config := testAccProviderConfigBlock + fmt.Sprintf(`
		resource "ceph_dashboard_role" "test" {
		  name        = %q
		  description = "OOB role"
		  scopes_permissions = {
		    "pool" = ["read"]
		  }
		}
	`, roleName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDashboardRoleDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				Check:           checkDashboardRoleExists(t, roleName),
			},
			{
				PreConfig: func() {
					if err := cephTestClusterCLI.DashboardRoleDelete(t.Context(), roleName); err != nil {
						t.Fatalf("Failed to delete dashboard role out of band: %v", err)
					}
				},
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_dashboard_role.test", plancheck.ResourceActionCreate),
					},
				},
				Check: checkDashboardRoleExists(t, roleName),
			},
		},
	})
}

func testAccDashboardRoleCLIFixture(t *testing.T, roleName string) {
	ctx := t.Context()
	if err := cephTestClusterCLI.DashboardRoleCreate(ctx, roleName, "Fixture role"); err != nil {
		t.Fatalf("Failed to create dashboard role: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := cephTestClusterCLI.DashboardRoleDelete(cleanupCtx, roleName); err != nil {
			t.Errorf("Failed to delete dashboard role: %v", err)
		}
	})
	if err := cephTestClusterCLI.DashboardRoleAddScopePerms(ctx, roleName, "pool", []string{"read"}); err != nil {
		t.Fatalf("Failed to add scope perms to dashboard role: %v", err)
	}
}

func checkDashboardRole(t *testing.T, roleName string, check func(*cephcli.DashboardRole) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		role, err := cephTestClusterCLI.DashboardRoleShow(t.Context(), roleName)
		if err != nil {
			return fmt.Errorf("failed to show dashboard role: %w", err)
		}
		return check(role)
	}
}

func checkDashboardRoleExists(t *testing.T, roleName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.DashboardRoleExists(t.Context(), roleName)
		if err != nil {
			return fmt.Errorf("failed to check dashboard role existence: %w", err)
		}
		if !exists {
			return fmt.Errorf("dashboard role %q not found", roleName)
		}
		return nil
	}
}

func testAccCheckDashboardRoleDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_dashboard_role" {
				continue
			}

			roleName := rs.Primary.Attributes["name"]

			exists, err := cephTestClusterCLI.DashboardRoleExists(ctx, roleName)
			if err != nil {
				return fmt.Errorf("failed to check dashboard role existence: %w", err)
			}

			if exists {
				return fmt.Errorf("dashboard role %q still exists", roleName)
			}
		}

		return nil
	}
}
