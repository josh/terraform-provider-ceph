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
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephPoolResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-pool")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephPoolDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  size              = 1
					  min_size          = 1
					  pg_num            = 1
					  pg_autoscale_mode = "off"
					}
				`, poolName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(poolName),
					),
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("size"),
						knownvalue.Int64Exact(1),
					),
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("pg_num"),
						knownvalue.Int64Exact(1),
					),
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("pool_id"),
						knownvalue.NotNull(),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "name", poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "size", "1"),
					resource.TestCheckResourceAttr("ceph_pool.test", "pg_num", "1"),
					resource.TestCheckResourceAttrSet("ceph_pool.test", "pool_id"),
					resource.TestCheckResourceAttrSet("ceph_pool.test", "crush_rule"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  size              = 2
					  pg_num            = 2
					  pg_autoscale_mode = "off"
					}
				`, poolName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("size"),
						knownvalue.Int64Exact(2),
					),
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("pg_num"),
						knownvalue.Int64Exact(2),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "size", "2"),
					resource.TestCheckResourceAttr("ceph_pool.test", "pg_num", "2"),
				),
			},
			{
				ConfigVariables:                      testAccProviderConfig(),
				ResourceName:                         "ceph_pool.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        poolName,
				ImportStateVerifyIdentifierAttribute: "name",
			},
		},
	})
}

func TestAccCephPoolResourceWithCompression(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-pool-compression")
	crushRuleName := acctest.RandomWithPrefix("test-crush-rule")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephPoolDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_crush_rule" "test" {
					  name           = %q
					  pool_type      = "replicated"
					  failure_domain = "osd"
					}

					resource "ceph_pool" "test" {
					  name                  = %q
					  pool_type             = "replicated"
					  crush_rule            = ceph_crush_rule.test.name
					  size                  = 2
					  min_size              = 1
					  pg_num                = 1
					  pg_autoscale_mode     = "off"
					  compression_mode      = "aggressive"
					  compression_algorithm = "snappy"
					}
				`, crushRuleName, poolName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("compression_mode"),
						knownvalue.StringExact("aggressive"),
					),
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("compression_algorithm"),
						knownvalue.StringExact("snappy"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "compression_mode", "aggressive"),
					resource.TestCheckResourceAttr("ceph_pool.test", "compression_algorithm", "snappy"),
				),
			},
		},
	})
}

func checkCephPoolExists(t *testing.T, poolName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		pools, err := cephTestClusterCLI.PoolList(ctx)
		if err != nil {
			return fmt.Errorf("failed to list pools: %w", err)
		}

		for _, pool := range pools {
			if pool == poolName {
				return nil
			}
		}

		return fmt.Errorf("pool %q not found in Ceph", poolName)
	}
}

func testAccCheckCephPoolDestroy(s *terraform.State) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "ceph_pool" {
			continue
		}

		poolName := rs.Primary.Attributes["name"]

		pools, err := cephTestClusterCLI.PoolList(ctx)
		if err != nil {
			return fmt.Errorf("failed to list pools: %w", err)
		}

		for _, pool := range pools {
			if pool == poolName {
				return fmt.Errorf("pool %q still exists in Ceph", poolName)
			}
		}
	}

	return nil
}

func TestAccCephPoolResource_InvalidPoolType(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_pool" "test" {
					  name       = "test-invalid-type"
					  pool_type  = "invalid"
					  pg_num     = 1
					}
				`,
				ExpectError: regexp.MustCompile(`Attribute pool_type value must be one of`),
			},
		},
	})
}

func TestAccCephPoolResource_Application(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-pool-app")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephPoolDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
						resource "ceph_pool" "test" {
						  name              = %q
						  pool_type         = "replicated"
						  size              = 1
						  min_size          = 1
						  pg_num            = 1
						  pg_autoscale_mode = "off"
						  application       = "rbd"
						}
					`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "name", poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "application", "rbd"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  size              = 1
					  min_size          = 1
					  pg_num            = 1
					  pg_autoscale_mode = "off"
					  application       = "cephfs"
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "application", "cephfs"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name        = %q
					  pool_type   = "replicated"
					  size        = 1
					  min_size    = 1
					  pg_num      = 1
					  application = "custom-app"
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "application", "custom-app"),
				),
			},
		},
	})
}

func TestAccCephPoolResource_CompressionModes(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testCases := []struct {
		mode string
	}{
		{mode: "none"},
		{mode: "passive"},
		{mode: "aggressive"},
		{mode: "force"},
	}

	for _, tc := range testCases {
		t.Run(tc.mode, func(t *testing.T) {
			poolName := acctest.RandomWithPrefix(fmt.Sprintf("test-pool-compress-%s", tc.mode))
			crushRuleName := acctest.RandomWithPrefix(fmt.Sprintf("test-crush-rule-%s", tc.mode))

			resource.Test(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             testAccCheckCephPoolDestroy,
				Steps: []resource.TestStep{
					{
						ConfigVariables: testAccProviderConfig(),
						Config: testAccProviderConfigBlock + fmt.Sprintf(`
						resource "ceph_crush_rule" "test" {
						  name           = %q
						  pool_type      = "replicated"
						  failure_domain = "osd"
						}

						resource "ceph_pool" "test" {
						  name                  = %q
						  pool_type             = "replicated"
						  crush_rule            = ceph_crush_rule.test.name
						  size                  = 2
						  min_size              = 1
						  pg_num                = 1
						  pg_autoscale_mode     = "off"
						  compression_mode      = %q
						  compression_algorithm = "snappy"
						}
					`, crushRuleName, poolName, tc.mode),
						Check: resource.ComposeAggregateTestCheckFunc(
							checkCephPoolExists(t, poolName),
							resource.TestCheckResourceAttr("ceph_pool.test", "compression_mode", tc.mode),
						),
					},
				},
			})
		})
	}
}

func TestAccCephPoolResource_InvalidCompressionMode(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_pool" "test" {
					  name             = "test-invalid-compression"
					  pool_type        = "replicated"
					  pg_num           = 1
					  compression_mode = "invalid"
					}
				`,
				ExpectError: regexp.MustCompile(`Attribute compression_mode value must be one of`),
			},
		},
	})
}

func TestAccCephPoolResource_PgAutoscaleMode(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-pool-autoscale")
	crushRuleName := acctest.RandomWithPrefix("test-crush-rule-autoscale")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephPoolDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_crush_rule" "test" {
					  name           = %q
					  pool_type      = "replicated"
					  failure_domain = "osd"
					}

					resource "ceph_pool" "test" {
					  name               = %q
					  pool_type          = "replicated"
					  crush_rule         = ceph_crush_rule.test.name
					  size               = 2
					  min_size           = 1
					  pg_num             = 1
					  pg_autoscale_mode  = "off"
					}
				`, crushRuleName, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "pg_autoscale_mode", "off"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_crush_rule" "test" {
					  name           = %q
					  pool_type      = "replicated"
					  failure_domain = "osd"
					}

					resource "ceph_pool" "test" {
					  name               = %q
					  pool_type          = "replicated"
					  crush_rule         = ceph_crush_rule.test.name
					  size               = 2
					  min_size           = 1
					  pg_num             = 1
					  pg_autoscale_mode  = "warn"
					}
				`, crushRuleName, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "pg_autoscale_mode", "warn"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_crush_rule" "test" {
					  name           = %q
					  pool_type      = "replicated"
					  failure_domain = "osd"
					}

					resource "ceph_pool" "test" {
					  name               = %q
					  pool_type          = "replicated"
					  crush_rule         = ceph_crush_rule.test.name
					  size               = 2
					  min_size           = 1
					  pg_num             = 1
					  pg_autoscale_mode  = "on"
					}
				`, crushRuleName, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "pg_autoscale_mode", "on"),
				),
			},
		},
	})
}

func TestAccCephPoolResource_InvalidPgAutoscaleMode(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_pool" "test" {
					  name               = "test-invalid-autoscale"
					  pool_type          = "replicated"
					  pg_num             = 1
					  pg_autoscale_mode  = "invalid"
					}
				`,
				ExpectError: regexp.MustCompile(`Attribute pg_autoscale_mode value must be one of`),
			},
		},
	})
}

func TestAccCephPoolResource_ErasurePool(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-pool-erasure")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephPoolDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name                 = %q
					  pool_type            = "erasure"
					  erasure_code_profile = "default"
					  pg_num               = 1
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "name", poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "pool_type", "erasure"),
					resource.TestCheckResourceAttr("ceph_pool.test", "erasure_code_profile", "default"),
					resource.TestCheckResourceAttrSet("ceph_pool.test", "size"),
				),
			},
		},
	})
}
