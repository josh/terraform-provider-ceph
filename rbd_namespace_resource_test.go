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
)

func TestAccCephRBDNamespaceResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")
	namespaceName := acctest.RandomWithPrefix("test-namespace")
	renamedNamespaceName := acctest.RandomWithPrefix("test-namespace-renamed")

	namespaceConfig := func(name string) string {
		return testAccProviderConfigBlock + testAccRBDPoolConfig(poolName) + fmt.Sprintf(`
			resource "ceph_rbd_namespace" "test" {
			  pool_name = ceph_pool.test.name
			  name      = %q
			}
		`, name)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRBDNamespaceDestroy(t, poolName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          namespaceConfig(namespaceName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rbd_namespace.test",
						tfjsonpath.New("pool_name"),
						knownvalue.StringExact(poolName),
					),
					statecheck.ExpectKnownValue(
						"ceph_rbd_namespace.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(namespaceName),
					),
				},
				Check: checkRBDNamespaceExists(t, poolName, namespaceName),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          namespaceConfig(namespaceName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          namespaceConfig(renamedNamespaceName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rbd_namespace.test", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkRBDNamespaceExists(t, poolName, renamedNamespaceName),
					checkRBDNamespaceAbsent(t, poolName, namespaceName),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccProviderConfigBlock + testAccRBDPoolConfig(poolName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rbd_namespace.test", plancheck.ResourceActionDestroy),
					},
				},
				Check: checkRBDNamespaceAbsent(t, poolName, renamedNamespaceName),
			},
		},
	})
}

func TestAccCephRBDNamespaceResource_Import(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")
	namespaceName := acctest.RandomWithPrefix("test-namespace")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
			testAccRBDNamespaceCLIFixture(t, poolName, namespaceName)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rbd_namespace" "test" {
					  pool_name = %q
					  name      = %q
					}
				`, poolName, namespaceName),
				ResourceName:  "ceph_rbd_namespace.test",
				ImportState:   true,
				ImportStateId: poolName + "/" + namespaceName,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["pool_name"] != poolName {
						return fmt.Errorf("expected pool_name %q, got %q", poolName, attrs["pool_name"])
					}
					if attrs["name"] != namespaceName {
						return fmt.Errorf("expected name %q, got %q", namespaceName, attrs["name"])
					}
					return nil
				},
			},
		},
	})
}

func testAccRBDNamespaceCLIFixture(t *testing.T, poolName, namespaceName string) {
	ctx := t.Context()
	if err := cephTestClusterCLI.PoolCreate(ctx, poolName, 8, "replicated"); err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := cephTestClusterCLI.PoolDelete(cleanupCtx, poolName); err != nil {
			t.Errorf("Failed to delete pool: %v", err)
		}
	})
	if err := cephTestClusterCLI.PoolApplicationEnable(ctx, poolName, "rbd"); err != nil {
		t.Fatalf("Failed to enable rbd application: %v", err)
	}
	if err := cephTestClusterCLI.RBDNamespaceCreate(ctx, poolName, namespaceName); err != nil {
		t.Fatalf("Failed to create rbd namespace: %v", err)
	}
}

func checkRBDNamespaceExists(t *testing.T, poolName, namespaceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.RBDNamespaceExists(t.Context(), poolName, namespaceName)
		if err != nil {
			return fmt.Errorf("failed to check rbd namespace existence: %w", err)
		}

		if !exists {
			return fmt.Errorf("rbd namespace %q not found in pool %q", namespaceName, poolName)
		}

		return nil
	}
}

func checkRBDNamespaceAbsent(t *testing.T, poolName, namespaceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.RBDNamespaceExists(t.Context(), poolName, namespaceName)
		if err != nil {
			return fmt.Errorf("failed to check rbd namespace existence: %w", err)
		}

		if exists {
			return fmt.Errorf("rbd namespace %q still exists in pool %q", namespaceName, poolName)
		}

		return nil
	}
}

func testAccCheckRBDNamespaceDestroy(t *testing.T, poolName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		poolExists, err := cephTestClusterCLI.PoolExists(ctx, poolName)
		if err != nil {
			return fmt.Errorf("failed to check pool existence: %w", err)
		}
		if !poolExists {
			return nil
		}

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_rbd_namespace" {
				continue
			}

			namespaceName := rs.Primary.Attributes["name"]

			exists, err := cephTestClusterCLI.RBDNamespaceExists(ctx, poolName, namespaceName)
			if err != nil {
				return fmt.Errorf("failed to check rbd namespace existence: %w", err)
			}

			if exists {
				return fmt.Errorf("rbd namespace %q still exists in pool %q", namespaceName, poolName)
			}
		}

		return nil
	}
}
