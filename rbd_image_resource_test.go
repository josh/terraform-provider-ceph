package main

import (
	"context"
	"fmt"
	"regexp"
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

func testAccRBDPoolConfig(poolName string) string {
	return fmt.Sprintf(`
		resource "ceph_pool" "test" {
		  name                 = %q
		  pool_type            = "replicated"
		  size                 = 2
		  min_size             = 1
		  pg_num               = 8
		  pg_autoscale_mode    = "off"
		  application_metadata = ["rbd"]

		  timeouts = {
		    create = "1m"
		    update = "5m"
		    delete = "1m"
		  }
		}
	`, poolName)
}

func TestAccCephRBDImageResource_invalidObjectSize(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")
	imageName := acctest.RandomWithPrefix("test-image")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRBDImageDestroy(t, poolName),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + testAccRBDPoolConfig(poolName) + fmt.Sprintf(`
					resource "ceph_rbd_image" "test" {
					  pool_name   = ceph_pool.test.name
					  name        = %q
					  size        = 8388608
					  object_size = 5000000
					}
				`, imageName),
				ExpectError: regexp.MustCompile(`(?i)power of two`),
			},
		},
	})
}

func TestAccCephRBDImageResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")
	imageName := acctest.RandomWithPrefix("test-image")

	imageConfig := func(size int64) string {
		return testAccProviderConfigBlock + testAccRBDPoolConfig(poolName) + fmt.Sprintf(`
			resource "ceph_rbd_image" "test" {
			  pool_name = ceph_pool.test.name
			  name      = %q
			  size      = %d

			  timeouts = {
			    create = "5m"
			    update = "5m"
			    delete = "5m"
			  }
			}
		`, imageName, size)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRBDImageDestroy(t, poolName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          imageConfig(8388608),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rbd_image.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(imageName),
					),
					statecheck.ExpectKnownValue(
						"ceph_rbd_image.test",
						tfjsonpath.New("pool_name"),
						knownvalue.StringExact(poolName),
					),
					statecheck.ExpectKnownValue(
						"ceph_rbd_image.test",
						tfjsonpath.New("size"),
						knownvalue.Int64Exact(8388608),
					),
					statecheck.ExpectKnownValue(
						"ceph_rbd_image.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_rbd_image.test",
						tfjsonpath.New("block_name_prefix"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_rbd_image.test",
						tfjsonpath.New("object_size"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_rbd_image.test",
						tfjsonpath.New("features_name"),
						knownvalue.NotNull(),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkRBDImageExists(t, poolName, imageName),
					checkRBDImageSize(t, poolName, imageName, 8388608),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          imageConfig(16777216),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rbd_image.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rbd_image.test",
						tfjsonpath.New("size"),
						knownvalue.Int64Exact(16777216),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkRBDImageExists(t, poolName, imageName),
					checkRBDImageSize(t, poolName, imageName, 16777216),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          imageConfig(16777216),
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
						plancheck.ExpectResourceAction("ceph_rbd_image.test", plancheck.ResourceActionDestroy),
					},
				},
				Check: checkRBDImageAbsent(t, poolName, imageName),
			},
		},
	})
}

func TestAccCephRBDImageResource_Features(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")
	imageName := acctest.RandomWithPrefix("test-image")

	imageConfig := func(features string) string {
		return testAccProviderConfigBlock + testAccRBDPoolConfig(poolName) + fmt.Sprintf(`
			resource "ceph_rbd_image" "test" {
			  pool_name = ceph_pool.test.name
			  name      = %q
			  size      = 8388608
			  features  = %s

			  timeouts = {
			    create = "5m"
			    update = "5m"
			    delete = "5m"
			  }
			}
		`, imageName, features)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRBDImageDestroy(t, poolName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          imageConfig(`["layering", "exclusive-lock"]`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rbd_image.test",
						tfjsonpath.New("features_name"),
						knownvalue.SetExact([]knownvalue.Check{
							knownvalue.StringExact("exclusive-lock"),
							knownvalue.StringExact("layering"),
						}),
					),
				},
				Check: checkRBDImageExists(t, poolName, imageName),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          imageConfig(`["layering", "exclusive-lock", "object-map", "fast-diff"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rbd_image.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rbd_image.test",
						tfjsonpath.New("features_name"),
						knownvalue.SetExact([]knownvalue.Check{
							knownvalue.StringExact("exclusive-lock"),
							knownvalue.StringExact("fast-diff"),
							knownvalue.StringExact("layering"),
							knownvalue.StringExact("object-map"),
						}),
					),
				},
				Check: checkRBDImageExists(t, poolName, imageName),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          imageConfig(`["layering", "exclusive-lock", "object-map", "fast-diff", "deep-flatten"]`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rbd_image.test", plancheck.ResourceActionReplace),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rbd_image.test",
						tfjsonpath.New("features_name"),
						knownvalue.SetExact([]knownvalue.Check{
							knownvalue.StringExact("deep-flatten"),
							knownvalue.StringExact("exclusive-lock"),
							knownvalue.StringExact("fast-diff"),
							knownvalue.StringExact("layering"),
							knownvalue.StringExact("object-map"),
						}),
					),
				},
				Check: checkRBDImageExists(t, poolName, imageName),
			},
		},
	})
}

func TestAccCephRBDImageResource_EmptyFeatures(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")
	imageName := acctest.RandomWithPrefix("test-image")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRBDImageDestroy(t, poolName),
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + testAccRBDPoolConfig(poolName) + fmt.Sprintf(`
					resource "ceph_rbd_image" "test" {
					  pool_name = ceph_pool.test.name
					  name      = %q
					  size      = 8388608
					  features  = []

					  timeouts = {
					    create = "5m"
					    update = "5m"
					    delete = "5m"
					  }
					}
				`, imageName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rbd_image.test",
						tfjsonpath.New("features_name"),
						knownvalue.SetExact([]knownvalue.Check{}),
					),
				},
				Check: checkRBDImageExists(t, poolName, imageName),
			},
		},
	})
}

func TestAccCephRBDImageResource_Import(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-rbd-pool")
	imageName := acctest.RandomWithPrefix("test-image")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckWaitForTasks(t)
			testAccPreCheckWaitForPGsActiveClean(t)
			testAccRBDImageCLIFixture(t, poolName, imageName, false)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rbd_image" "test" {
					  pool_name = %q
					  name      = %q
					  size      = 8388608
					}
				`, poolName, imageName),
				ResourceName:  "ceph_rbd_image.test",
				ImportState:   true,
				ImportStateId: poolName + "/" + imageName,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["pool_name"] != poolName {
						return fmt.Errorf("expected pool_name %q, got %q", poolName, attrs["pool_name"])
					}
					if attrs["name"] != imageName {
						return fmt.Errorf("expected name %q, got %q", imageName, attrs["name"])
					}
					if attrs["size"] != "8388608" {
						return fmt.Errorf("expected size 8388608, got %q", attrs["size"])
					}
					if attrs["object_size"] != "4194304" {
						return fmt.Errorf("expected object_size 4194304, got %q", attrs["object_size"])
					}
					if attrs["id"] == "" {
						return fmt.Errorf("expected id to be set")
					}
					if attrs["block_name_prefix"] == "" {
						return fmt.Errorf("expected block_name_prefix to be set")
					}
					return nil
				},
			},
		},
	})
}

func testAccRBDImageCLIFixture(t *testing.T, poolName, imageName string, removeImage bool) {
	ctx := t.Context()
	if err := cephTestClusterCLI.PoolCreate(ctx, poolName, 8, "replicated"); err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if removeImage {
			if err := cephTestClusterCLI.RBDRemove(cleanupCtx, poolName, imageName); err != nil {
				t.Errorf("Failed to remove rbd image: %v", err)
			}
		}
		if err := cephTestClusterCLI.PoolDelete(cleanupCtx, poolName); err != nil {
			t.Errorf("Failed to delete pool: %v", err)
		}
	})
	if err := cephTestClusterCLI.PoolApplicationEnable(ctx, poolName, "rbd"); err != nil {
		t.Fatalf("Failed to enable rbd application: %v", err)
	}
	if err := cephTestClusterCLI.RBDCreate(ctx, poolName, imageName, 8); err != nil {
		t.Fatalf("Failed to create rbd image: %v", err)
	}
}

func checkRBDImageExists(t *testing.T, poolName, imageName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.RBDExists(t.Context(), poolName, imageName)
		if err != nil {
			return fmt.Errorf("failed to check rbd image existence: %w", err)
		}

		if !exists {
			return fmt.Errorf("rbd image %q not found in pool %q", imageName, poolName)
		}

		return nil
	}
}

func checkRBDImageAbsent(t *testing.T, poolName, imageName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		exists, err := cephTestClusterCLI.RBDExists(t.Context(), poolName, imageName)
		if err != nil {
			return fmt.Errorf("failed to check rbd image existence: %w", err)
		}

		if exists {
			return fmt.Errorf("rbd image %q still exists in pool %q", imageName, poolName)
		}

		return nil
	}
}

func checkRBDImageSize(t *testing.T, poolName, imageName string, size int64) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		info, err := cephTestClusterCLI.RBDInfo(t.Context(), poolName, imageName)
		if err != nil {
			return fmt.Errorf("failed to get rbd image info: %w", err)
		}

		if info.Size != size {
			return fmt.Errorf("rbd image %q size: expected %d, got %d", imageName, size, info.Size)
		}

		return nil
	}
}

func testAccCheckRBDImageDestroy(t *testing.T, poolName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_rbd_image" {
				continue
			}

			imageName := rs.Primary.Attributes["name"]

			exists, err := cephTestClusterCLI.RBDExists(ctx, poolName, imageName)
			if err != nil {
				return fmt.Errorf("failed to check rbd image existence: %w", err)
			}

			if exists {
				return fmt.Errorf("rbd image %q still exists in pool %q", imageName, poolName)
			}
		}

		return nil
	}
}
