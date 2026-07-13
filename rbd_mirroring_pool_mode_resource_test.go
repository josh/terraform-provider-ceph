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

func testAccRBDMirroringPoolModeConfig(poolName, mode string) string {
	return testAccProviderConfigBlock + testAccRBDPoolConfig(poolName) + fmt.Sprintf(`
		resource "ceph_rbd_mirroring_pool_mode" "test" {
		  pool_name = ceph_pool.test.name
		  mode      = %q
		}
	`, mode)
}

func TestAccCephRBDMirroringPoolModeResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccRBDMirroringPoolModeConfig(poolName, "pool"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rbd_mirroring_pool_mode.test",
						tfjsonpath.New("pool_name"),
						knownvalue.StringExact(poolName),
					),
					statecheck.ExpectKnownValue(
						"ceph_rbd_mirroring_pool_mode.test",
						tfjsonpath.New("mode"),
						knownvalue.StringExact("pool"),
					),
				},
				Check: checkRBDMirrorPoolMode(t, poolName, "pool"),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccRBDMirroringPoolModeConfig(poolName, "image"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rbd_mirroring_pool_mode.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: checkRBDMirrorPoolMode(t, poolName, "image"),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccRBDMirroringPoolModeConfig(poolName, "image"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccProviderConfigBlock + testAccRBDPoolConfig(poolName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rbd_mirroring_pool_mode.test", plancheck.ResourceActionDestroy),
					},
				},
				Check: checkRBDMirrorPoolMode(t, poolName, "disabled"),
			},
		},
	})
}

func TestAccCephRBDMirroringPoolModeResource_OutOfBandDisable(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccRBDMirroringPoolModeConfig(poolName, "pool"),
				Check:           checkRBDMirrorPoolMode(t, poolName, "pool"),
			},
			{
				PreConfig: func() {
					if err := cephTestClusterCLI.RBDMirrorPoolDisable(t.Context(), poolName); err != nil {
						t.Fatalf("Failed to disable rbd mirroring out of band: %v", err)
					}
				},
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccRBDMirroringPoolModeConfig(poolName, "pool"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rbd_mirroring_pool_mode.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: checkRBDMirrorPoolMode(t, poolName, "pool"),
			},
		},
	})
}

func TestAccCephRBDMirroringPoolModeResource_Import(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
			testAccRBDMirroringCLIFixture(t, poolName)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rbd_mirroring_pool_mode" "test" {
					  pool_name = %q
					  mode      = "image"
					}
				`, poolName),
				ResourceName:  "ceph_rbd_mirroring_pool_mode.test",
				ImportState:   true,
				ImportStateId: poolName,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["pool_name"] != poolName {
						return fmt.Errorf("expected pool_name %q, got %q", poolName, attrs["pool_name"])
					}
					if attrs["mode"] != "image" {
						return fmt.Errorf("expected mode image, got %q", attrs["mode"])
					}
					return nil
				},
			},
		},
	})
}

func testAccRBDMirroringCLIFixture(t *testing.T, poolName string) {
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
	if err := cephTestClusterCLI.RBDMirrorPoolEnable(ctx, poolName, "image"); err != nil {
		t.Fatalf("Failed to enable rbd mirroring: %v", err)
	}
}

func checkRBDMirrorPoolMode(t *testing.T, poolName, want string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		mode, err := cephTestClusterCLI.RBDMirrorPoolMode(t.Context(), poolName)
		if err != nil {
			return fmt.Errorf("failed to get rbd mirror pool mode: %w", err)
		}
		if mode != want {
			return fmt.Errorf("rbd mirror pool mode on %q: expected %q, got %q", poolName, want, mode)
		}
		return nil
	}
}
