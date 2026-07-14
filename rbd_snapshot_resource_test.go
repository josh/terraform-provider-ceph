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

func testAccRBDSnapshotImageConfig(poolName, imageName string) string {
	return testAccProviderConfigBlock + testAccRBDPoolConfig(poolName) + fmt.Sprintf(`
		resource "ceph_rbd_image" "test" {
		  pool_name = ceph_pool.test.name
		  name      = %q
		  size      = 8388608

		  timeouts = {
		    create = "5m"
		    update = "5m"
		    delete = "5m"
		  }
		}
	`, imageName)
}

func TestAccCephRBDSnapshotResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")
	imageName := acctest.RandomWithPrefix("test-image")
	snapName := acctest.RandomWithPrefix("test-snap")
	renamedSnapName := acctest.RandomWithPrefix("test-snap-renamed")

	snapshotConfig := func(name string, isProtected bool) string {
		return testAccRBDSnapshotImageConfig(poolName, imageName) + fmt.Sprintf(`
			resource "ceph_rbd_snapshot" "test" {
			  pool_name    = ceph_pool.test.name
			  image_name   = ceph_rbd_image.test.name
			  name         = %q
			  is_protected = %t
			}
		`, name, isProtected)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRBDSnapshotDestroy(t, poolName, imageName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          snapshotConfig(snapName, false),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rbd_snapshot.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(snapName),
					),
					statecheck.ExpectKnownValue(
						"ceph_rbd_snapshot.test",
						tfjsonpath.New("is_protected"),
						knownvalue.Bool(false),
					),
					statecheck.ExpectKnownValue(
						"ceph_rbd_snapshot.test",
						tfjsonpath.New("size"),
						knownvalue.Int64Exact(8388608),
					),
					statecheck.ExpectKnownValue(
						"ceph_rbd_snapshot.test",
						tfjsonpath.New("timestamp"),
						knownvalue.NotNull(),
					),
				},
				Check: checkRBDSnapshotExists(t, poolName, imageName, snapName),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          snapshotConfig(snapName, true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rbd_snapshot.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rbd_snapshot.test",
						tfjsonpath.New("is_protected"),
						knownvalue.Bool(true),
					),
				},
				Check: checkRBDSnapshotProtected(t, poolName, imageName, snapName, true),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          snapshotConfig(renamedSnapName, true),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rbd_snapshot.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkRBDSnapshotExists(t, poolName, imageName, renamedSnapName),
					checkRBDSnapshotAbsent(t, poolName, imageName, snapName),
					checkRBDSnapshotProtected(t, poolName, imageName, renamedSnapName, true),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          snapshotConfig(renamedSnapName, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rbd_snapshot.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: checkRBDSnapshotProtected(t, poolName, imageName, renamedSnapName, false),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          snapshotConfig(renamedSnapName, false),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccRBDSnapshotImageConfig(poolName, imageName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rbd_snapshot.test", plancheck.ResourceActionDestroy),
					},
				},
				Check: checkRBDSnapshotAbsent(t, poolName, imageName, renamedSnapName),
			},
		},
	})
}

func TestAccCephRBDSnapshotResource_ProtectedDestroy(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")
	imageName := acctest.RandomWithPrefix("test-image")
	snapName := acctest.RandomWithPrefix("test-snap")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRBDSnapshotDestroy(t, poolName, imageName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccRBDSnapshotImageConfig(poolName, imageName) + fmt.Sprintf(`
					resource "ceph_rbd_snapshot" "test" {
					  pool_name    = ceph_pool.test.name
					  image_name   = ceph_rbd_image.test.name
					  name         = %q
					  is_protected = true
					}
				`, snapName),
				Check: checkRBDSnapshotProtected(t, poolName, imageName, snapName, true),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccRBDSnapshotImageConfig(poolName, imageName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rbd_snapshot.test", plancheck.ResourceActionDestroy),
					},
				},
				Check: checkRBDSnapshotAbsent(t, poolName, imageName, snapName),
			},
		},
	})
}

func TestAccCephRBDSnapshotResource_Import(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")
	imageName := acctest.RandomWithPrefix("test-image")
	snapName := acctest.RandomWithPrefix("test-snap")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
			testAccRBDSnapshotCLIFixture(t, poolName, imageName, snapName)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rbd_snapshot" "test" {
					  pool_name  = %q
					  image_name = %q
					  name       = %q
					}
				`, poolName, imageName, snapName),
				ResourceName:  "ceph_rbd_snapshot.test",
				ImportState:   true,
				ImportStateId: poolName + "/" + imageName + "/" + snapName,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["pool_name"] != poolName {
						return fmt.Errorf("expected pool_name %q, got %q", poolName, attrs["pool_name"])
					}
					if attrs["image_name"] != imageName {
						return fmt.Errorf("expected image_name %q, got %q", imageName, attrs["image_name"])
					}
					if attrs["name"] != snapName {
						return fmt.Errorf("expected name %q, got %q", snapName, attrs["name"])
					}
					if attrs["is_protected"] != "false" {
						return fmt.Errorf("expected is_protected false, got %q", attrs["is_protected"])
					}
					if attrs["size"] != "8388608" {
						return fmt.Errorf("expected size 8388608, got %q", attrs["size"])
					}
					if attrs["timestamp"] == "" {
						return fmt.Errorf("expected timestamp to be set")
					}
					return nil
				},
			},
		},
	})
}

func testAccRBDSnapshotCLIFixture(t *testing.T, poolName, imageName, snapName string) {
	testAccRBDImageCLIFixture(t, poolName, imageName, true)
	if err := cephTestClusterCLI.RBDSnapCreate(t.Context(), poolName, imageName, snapName); err != nil {
		t.Fatalf("Failed to create rbd snapshot: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := cephTestClusterCLI.RBDSnapRemove(cleanupCtx, poolName, imageName, snapName); err != nil {
			t.Errorf("Failed to remove rbd snapshot: %v", err)
		}
	})
}

func TestAccCephRBDSnapshotResource_namespace(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")
	namespace := acctest.RandomWithPrefix("test-ns")
	imageName := acctest.RandomWithPrefix("test-image")
	snapName := acctest.RandomWithPrefix("test-snap")

	// The rbd CLI accepts pool/namespace/image specs, so the namespaced image
	// can be addressed through the shared helpers via a combined image spec.
	imageSpec := namespace + "/" + imageName

	config := testAccProviderConfigBlock + testAccRBDPoolConfig(poolName) + fmt.Sprintf(`
		resource "ceph_rbd_namespace" "test" {
		  pool_name = ceph_pool.test.name
		  name      = %q
		}

		resource "ceph_rbd_image" "test" {
		  pool_name = ceph_pool.test.name
		  namespace = ceph_rbd_namespace.test.name
		  name      = %q
		  size      = 8388608

		  timeouts = {
		    create = "5m"
		    update = "5m"
		    delete = "5m"
		  }
		}

		resource "ceph_rbd_snapshot" "test" {
		  pool_name  = ceph_pool.test.name
		  namespace  = ceph_rbd_image.test.namespace
		  image_name = ceph_rbd_image.test.name
		  name       = %q
		}
	`, namespace, imageName, snapName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRBDSnapshotDestroy(t, poolName, imageSpec),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rbd_snapshot.test",
						tfjsonpath.New("namespace"),
						knownvalue.StringExact(namespace),
					),
					statecheck.ExpectKnownValue(
						"ceph_rbd_snapshot.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(snapName),
					),
					statecheck.ExpectKnownValue(
						"ceph_rbd_snapshot.test",
						tfjsonpath.New("size"),
						knownvalue.Int64Exact(8388608),
					),
				},
				Check: checkRBDSnapshotExists(t, poolName, imageSpec, snapName),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ResourceName:    "ceph_rbd_snapshot.test",
				ImportState:     true,
				ImportStateId:   poolName + "/" + namespace + "/" + imageName + "/" + snapName,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["pool_name"] != poolName {
						return fmt.Errorf("expected pool_name %q, got %q", poolName, attrs["pool_name"])
					}
					if attrs["namespace"] != namespace {
						return fmt.Errorf("expected namespace %q, got %q", namespace, attrs["namespace"])
					}
					if attrs["image_name"] != imageName {
						return fmt.Errorf("expected image_name %q, got %q", imageName, attrs["image_name"])
					}
					if attrs["name"] != snapName {
						return fmt.Errorf("expected name %q, got %q", snapName, attrs["name"])
					}
					return nil
				},
			},
		},
	})
}

func checkRBDSnapshotExists(t *testing.T, poolName, imageName, snapName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.RBDSnapExists(t.Context(), poolName, imageName, snapName)
		if err != nil {
			return fmt.Errorf("failed to check rbd snapshot existence: %w", err)
		}

		if !exists {
			return fmt.Errorf("rbd snapshot %q not found on image %s/%s", snapName, poolName, imageName)
		}

		return nil
	}
}

func checkRBDSnapshotAbsent(t *testing.T, poolName, imageName, snapName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.RBDSnapExists(t.Context(), poolName, imageName, snapName)
		if err != nil {
			return fmt.Errorf("failed to check rbd snapshot existence: %w", err)
		}

		if exists {
			return fmt.Errorf("rbd snapshot %q still exists on image %s/%s", snapName, poolName, imageName)
		}

		return nil
	}
}

func checkRBDSnapshotProtected(t *testing.T, poolName, imageName, snapName string, want bool) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		snaps, err := cephTestClusterCLI.RBDSnapList(t.Context(), poolName, imageName)
		if err != nil {
			return fmt.Errorf("failed to list rbd snapshots: %w", err)
		}

		for _, snap := range snaps {
			if snap.Name != snapName {
				continue
			}
			got := snap.Protected == "true"
			if got != want {
				return fmt.Errorf("rbd snapshot %q protected: expected %t, got %t", snapName, want, got)
			}
			return nil
		}

		return fmt.Errorf("rbd snapshot %q not found on image %s/%s", snapName, poolName, imageName)
	}
}

func testAccCheckRBDSnapshotDestroy(t *testing.T, poolName, imageName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_rbd_snapshot" {
				continue
			}

			snapName := rs.Primary.Attributes["name"]

			exists, err := cephTestClusterCLI.RBDSnapExists(ctx, poolName, imageName, snapName)
			if err != nil {
				return fmt.Errorf("failed to check rbd snapshot existence: %w", err)
			}

			if exists {
				return fmt.Errorf("rbd snapshot %q still exists on image %s/%s", snapName, poolName, imageName)
			}
		}

		return nil
	}
}
