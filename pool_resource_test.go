package main

import (
	"fmt"
	"regexp"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
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
		CheckDestroy:             testAccCheckCephPoolDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  size              = 2
					  min_size          = 1
					  pg_num            = 32
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
						knownvalue.Int64Exact(2),
					),
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("pg_num"),
						knownvalue.Int64Exact(32),
					),
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("pool_id"),
						knownvalue.NotNull(),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolSize(t, poolName, 2),
					checkCephPoolPgNum(t, poolName, 32),
					checkCephPoolPgAutoscaleMode(t, poolName, "off"),
					resource.TestCheckResourceAttr("ceph_pool.test", "name", poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "size", "2"),
					resource.TestCheckResourceAttr("ceph_pool.test", "pg_num", "32"),
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
					  size              = 3
					  pg_num            = 64
					  pg_autoscale_mode = "off"
					}
				`, poolName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_pool.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("size"),
						knownvalue.Int64Exact(3),
					),
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("pg_num"),
						knownvalue.Int64Exact(64),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolSize(t, poolName, 3),
					checkCephPoolPgNum(t, poolName, 64),
					checkCephPoolPgAutoscaleMode(t, poolName, "off"),
					resource.TestCheckResourceAttr("ceph_pool.test", "size", "3"),
					resource.TestCheckResourceAttr("ceph_pool.test", "pg_num", "64"),
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

func TestAccCephPoolResource_WithCompression(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-pool-compression")
	crushRuleName := acctest.RandomWithPrefix("test-crush-rule")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephPoolDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
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
					  pg_num                = 32
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
		exists, err := cephTestClusterCLI.PoolExists(t.Context(), poolName)
		if err != nil {
			return fmt.Errorf("failed to check pool existence: %w", err)
		}

		if !exists {
			return fmt.Errorf("pool %q not found in Ceph", poolName)
		}

		return nil
	}
}

func checkCephPoolErasureProperties(t *testing.T, poolName, expectedProfile string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		profile, err := cephTestClusterCLI.PoolGet(t.Context(), poolName, "erasure_code_profile")
		if err != nil {
			return fmt.Errorf("failed to get erasure_code_profile from Ceph CLI: %w", err)
		}

		if profile != expectedProfile {
			return fmt.Errorf("erasure_code_profile mismatch: got %q, want %q", profile, expectedProfile)
		}

		return nil
	}
}

func checkCephPoolPgAutoscaleMode(t *testing.T, poolName, expectedMode string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		mode, err := cephTestClusterCLI.PoolGet(t.Context(), poolName, "pg_autoscale_mode")
		if err != nil {
			return fmt.Errorf("failed to get pg_autoscale_mode from Ceph CLI: %w", err)
		}

		if mode != expectedMode {
			return fmt.Errorf("pg_autoscale_mode mismatch: got %q, want %q", mode, expectedMode)
		}

		return nil
	}
}

func checkCephPoolCompressionMode(t *testing.T, poolName, expectedMode string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		mode, err := cephTestClusterCLI.PoolGet(t.Context(), poolName, "compression_mode")
		if err != nil {
			return fmt.Errorf("failed to get compression_mode from Ceph CLI: %w", err)
		}

		if mode != expectedMode {
			return fmt.Errorf("compression_mode mismatch: got %q, want %q", mode, expectedMode)
		}

		return nil
	}
}

func checkCephPoolCompressionAlgorithm(t *testing.T, poolName, expectedAlgorithm string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		algorithm, err := cephTestClusterCLI.PoolGet(t.Context(), poolName, "compression_algorithm")
		if err != nil {
			return fmt.Errorf("failed to get compression_algorithm from Ceph CLI: %w", err)
		}

		if algorithm != expectedAlgorithm {
			return fmt.Errorf("compression_algorithm mismatch: got %q, want %q", algorithm, expectedAlgorithm)
		}

		return nil
	}
}

func checkCephPoolApplication(t *testing.T, poolName, expectedApplication string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		apps, err := cephTestClusterCLI.PoolApplicationGet(t.Context(), poolName)
		if err != nil {
			return fmt.Errorf("failed to get applications from Ceph CLI: %w", err)
		}

		if slices.Contains(apps, expectedApplication) {
			return nil
		}

		return fmt.Errorf("application %q not found in pool, enabled applications: %v", expectedApplication, apps)
	}
}

func checkCephPoolSize(t *testing.T, poolName string, expectedSize int64) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		sizeStr, err := cephTestClusterCLI.PoolGet(t.Context(), poolName, "size")
		if err != nil {
			return fmt.Errorf("failed to get size from Ceph CLI: %w", err)
		}

		if sizeStr != fmt.Sprintf("%d", expectedSize) {
			return fmt.Errorf("size mismatch: got %q, want %d", sizeStr, expectedSize)
		}

		return nil
	}
}

func checkCephPoolMinSize(t *testing.T, poolName string, expectedMinSize int64) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		minSizeStr, err := cephTestClusterCLI.PoolGet(t.Context(), poolName, "min_size")
		if err != nil {
			return fmt.Errorf("failed to get min_size from Ceph CLI: %w", err)
		}

		if minSizeStr != fmt.Sprintf("%d", expectedMinSize) {
			return fmt.Errorf("min_size mismatch: got %q, want %d", minSizeStr, expectedMinSize)
		}

		return nil
	}
}

func checkCephPoolPgNum(t *testing.T, poolName string, expectedPgNum int64) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		pgNumStr, err := cephTestClusterCLI.PoolGet(t.Context(), poolName, "pg_num")
		if err != nil {
			return fmt.Errorf("failed to get pg_num from Ceph CLI: %w", err)
		}

		if pgNumStr != fmt.Sprintf("%d", expectedPgNum) {
			return fmt.Errorf("pg_num mismatch: got %q, want %d", pgNumStr, expectedPgNum)
		}

		return nil
	}
}

func testAccCheckCephPoolDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_pool" {
				continue
			}

			poolName := rs.Primary.Attributes["name"]

			exists, err := cephTestClusterCLI.PoolExists(ctx, poolName)
			if err != nil {
				return fmt.Errorf("failed to check pool existence: %w", err)
			}

			if exists {
				return fmt.Errorf("pool %q still exists in Ceph", poolName)
			}
		}

		return nil
	}
}

func TestAccCephPoolResource_InvalidPoolType(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_pool" "test" {
					  name              = "test-invalid-type"
					  pool_type         = "invalid"
					  pg_num            = 32
					  pg_autoscale_mode = "off"
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
		CheckDestroy:             testAccCheckCephPoolDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
						resource "ceph_pool" "test" {
						  name              = %q
						  pool_type         = "replicated"
						  size              = 2
						  min_size          = 1
						  pg_num            = 32
						  pg_autoscale_mode = "off"
						  application_metadata = ["rbd"]
						}
					`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolApplication(t, poolName, "rbd"),
					resource.TestCheckResourceAttr("ceph_pool.test", "name", poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "application_metadata.0", "rbd"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  size              = 2
					  min_size          = 1
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					  application_metadata = ["cephfs"]
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolApplication(t, poolName, "cephfs"),
					resource.TestCheckResourceAttr("ceph_pool.test", "application_metadata.0", "cephfs"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  size              = 2
					  min_size          = 1
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					  application_metadata = ["custom-app"]
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolApplication(t, poolName, "custom-app"),
					resource.TestCheckResourceAttr("ceph_pool.test", "application_metadata.0", "custom-app"),
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
				CheckDestroy:             testAccCheckCephPoolDestroy(t),
				PreCheck: func() {
					testAccPreCheckCephHealth(t)
				},
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
						  pg_num                = 32
						  pg_autoscale_mode     = "off"
						  compression_mode      = %q
						  compression_algorithm = "snappy"
						}
					`, crushRuleName, poolName, tc.mode),
						Check: resource.ComposeAggregateTestCheckFunc(
							checkCephPoolExists(t, poolName),
							checkCephPoolCompressionMode(t, poolName, tc.mode),
							checkCephPoolCompressionAlgorithm(t, poolName, "snappy"),
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
					  name              = "test-invalid-compression"
					  pool_type         = "replicated"
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					  compression_mode  = "invalid"
					}
				`,
				ExpectError: regexp.MustCompile(`Attribute compression_mode value must be one of`),
			},
		},
	})
}

func TestAccCephPoolResource_CompressionModeNoneWithAlgorithm(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_pool" "test" {
					  name                  = "test-invalid-compression-algo"
					  pool_type             = "replicated"
					  pg_num                = 32
					  pg_autoscale_mode     = "off"
					  compression_mode      = "none"
					  compression_algorithm = "snappy"
					}
				`,
				ExpectError: regexp.MustCompile(`compression_algorithm cannot be set when compression_mode is "none"`),
			},
		},
	})
}

func TestAccCephPoolResource_CompressionModeNoneWithRequiredRatio(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_pool" "test" {
					  name                         = "test-invalid-compression-ratio"
					  pool_type                    = "replicated"
					  pg_num                       = 32
					  pg_autoscale_mode            = "off"
					  compression_mode             = "none"
					  compression_required_ratio   = 0.8
					}
				`,
				ExpectError: regexp.MustCompile(`compression_required_ratio cannot be set when compression_mode is "none"`),
			},
		},
	})
}

func TestAccCephPoolResource_CompressionModeNoneWithMinBlobSize(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_pool" "test" {
					  name                       = "test-invalid-compression-min"
					  pool_type                  = "replicated"
					  pg_num                     = 32
					  pg_autoscale_mode          = "off"
					  compression_mode           = "none"
					  compression_min_blob_size  = 1024
					}
				`,
				ExpectError: regexp.MustCompile(`compression_min_blob_size cannot be set when compression_mode is "none"`),
			},
		},
	})
}

func TestAccCephPoolResource_CompressionModeNoneWithMaxBlobSize(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_pool" "test" {
					  name                       = "test-invalid-compression-max"
					  pool_type                  = "replicated"
					  pg_num                     = 32
					  pg_autoscale_mode          = "off"
					  compression_mode           = "none"
					  compression_max_blob_size  = 2048
					}
				`,
				ExpectError: regexp.MustCompile(`compression_max_blob_size cannot be set when compression_mode is "none"`),
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
		CheckDestroy:             testAccCheckCephPoolDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
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
					  pg_num             = 32
					  pg_autoscale_mode  = "off"
					}
				`, crushRuleName, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolPgAutoscaleMode(t, poolName, "off"),
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
					  pg_num             = 32
					  pg_autoscale_mode  = "warn"
					}
				`, crushRuleName, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolPgAutoscaleMode(t, poolName, "warn"),
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
					  pg_autoscale_mode  = "on"
					}
				`, crushRuleName, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolPgAutoscaleMode(t, poolName, "on"),
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
					  pg_num             = 32
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
		CheckDestroy:             testAccCheckCephPoolDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name                 = %q
					  pool_type            = "erasure"
					  erasure_code_profile = "default"
					  pg_num               = 32
					  pg_autoscale_mode    = "off"
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolErasureProperties(t, poolName, "default"),
					resource.TestCheckResourceAttr("ceph_pool.test", "name", poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "pool_type", "erasure"),
					resource.TestCheckResourceAttr("ceph_pool.test", "erasure_code_profile", "default"),
					resource.TestCheckResourceAttrSet("ceph_pool.test", "size"),
				),
			},
		},
	})
}

func TestAccCephPoolResource_ReplicatedWithErasureProfile(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_pool" "test" {
					  name                 = "test-invalid-replicated"
					  pool_type            = "replicated"
					  erasure_code_profile = "default"
					  pg_num               = 32
					  pg_autoscale_mode    = "off"
					}
				`,
				ExpectError: regexp.MustCompile(`erasure_code_profile is only valid for erasure pools`),
			},
		},
	})
}

func TestAccCephPoolResource_ErasureWithSize(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_pool" "test" {
					  name              = "test-invalid-erasure-size"
					  pool_type         = "erasure"
					  size              = 3
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					}
				`,
				ExpectError: regexp.MustCompile(`size is only valid for replicated pools`),
			},
		},
	})
}

func TestAccCephPoolResource_ErasureWithMinSize(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_pool" "test" {
					  name              = "test-invalid-erasure-minsize"
					  pool_type         = "erasure"
					  min_size          = 2
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					}
				`,
				ExpectError: regexp.MustCompile(`min_size is only valid for replicated pools`),
			},
		},
	})
}

func TestAccCephPoolResource_ErasureWithoutProfile(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-pool-erasure-default")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephPoolDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "erasure"
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolErasureProperties(t, poolName, "default"),
					resource.TestCheckResourceAttr("ceph_pool.test", "name", poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "pool_type", "erasure"),
					resource.TestCheckResourceAttr("ceph_pool.test", "erasure_code_profile", "default"),
				),
			},
		},
	})
}

func TestAccCephPoolResource_UpdatePgAutoscaleModeInPlace(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-pool-autoscale-update")
	crushRuleName := acctest.RandomWithPrefix("test-crush-rule-autoscale-update")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephPoolDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
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
					  pg_num             = 32
					  pg_autoscale_mode  = "off"
					}
				`, crushRuleName, poolName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("pg_autoscale_mode"),
						knownvalue.StringExact("off"),
					),
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("pool_id"),
						knownvalue.NotNull(),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolPgAutoscaleMode(t, poolName, "off"),
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
					  pg_num             = 32
					  pg_autoscale_mode  = "warn"
					}
				`, crushRuleName, poolName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_pool.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("pg_autoscale_mode"),
						knownvalue.StringExact("warn"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolPgAutoscaleMode(t, poolName, "warn"),
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
					  pg_autoscale_mode  = "on"
					}
				`, crushRuleName, poolName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_pool.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("pg_autoscale_mode"),
						knownvalue.StringExact("on"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolPgAutoscaleMode(t, poolName, "on"),
					resource.TestCheckResourceAttr("ceph_pool.test", "pg_autoscale_mode", "on"),
				),
			},
		},
	})
}

func TestAccCephPoolResource_UpdateCompressionInPlace(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-pool-compression-update")
	crushRuleName := acctest.RandomWithPrefix("test-crush-rule-compression-update")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephPoolDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
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
					  pg_num                = 32
					  pg_autoscale_mode     = "off"
					  compression_mode      = "none"
					}
				`, crushRuleName, poolName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("compression_mode"),
						knownvalue.StringExact("none"),
					),
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("pool_id"),
						knownvalue.NotNull(),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolCompressionMode(t, poolName, "none"),
					resource.TestCheckResourceAttr("ceph_pool.test", "compression_mode", "none"),
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
					  name                  = %q
					  pool_type             = "replicated"
					  crush_rule            = ceph_crush_rule.test.name
					  size                  = 2
					  min_size              = 1
					  pg_num                = 32
					  pg_autoscale_mode     = "off"
					  compression_mode      = "passive"
					  compression_algorithm = "snappy"
					}
				`, crushRuleName, poolName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_pool.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("compression_mode"),
						knownvalue.StringExact("passive"),
					),
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("compression_algorithm"),
						knownvalue.StringExact("snappy"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolCompressionMode(t, poolName, "passive"),
					checkCephPoolCompressionAlgorithm(t, poolName, "snappy"),
					resource.TestCheckResourceAttr("ceph_pool.test", "compression_mode", "passive"),
					resource.TestCheckResourceAttr("ceph_pool.test", "compression_algorithm", "snappy"),
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
					  name                  = %q
					  pool_type             = "replicated"
					  crush_rule            = ceph_crush_rule.test.name
					  size                  = 2
					  min_size              = 1
					  pg_num                = 32
					  pg_autoscale_mode     = "off"
					  compression_mode      = "aggressive"
					  compression_algorithm = "zstd"
					}
				`, crushRuleName, poolName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_pool.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("compression_mode"),
						knownvalue.StringExact("aggressive"),
					),
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("compression_algorithm"),
						knownvalue.StringExact("zstd"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolCompressionMode(t, poolName, "aggressive"),
					checkCephPoolCompressionAlgorithm(t, poolName, "zstd"),
					resource.TestCheckResourceAttr("ceph_pool.test", "compression_mode", "aggressive"),
					resource.TestCheckResourceAttr("ceph_pool.test", "compression_algorithm", "zstd"),
				),
			},
		},
	})
}

func TestAccCephPoolResource_MinSizeUpdate(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-pool-minsize")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephPoolDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  size              = 2
					  min_size          = 1
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					}
				`, poolName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("min_size"),
						knownvalue.Int64Exact(1),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolMinSize(t, poolName, 1),
					resource.TestCheckResourceAttr("ceph_pool.test", "min_size", "1"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  size              = 2
					  min_size          = 2
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					}
				`, poolName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("min_size"),
						knownvalue.Int64Exact(2),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolMinSize(t, poolName, 2),
					resource.TestCheckResourceAttr("ceph_pool.test", "min_size", "2"),
				),
			},
		},
	})
}

func TestAccCephPoolResource_NameUpdateInPlace(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-pool-name-update")
	poolNameUpdated := acctest.RandomWithPrefix("test-pool-name-updated")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephPoolDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  size              = 2
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					}
				`, poolName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(poolName),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "name", poolName),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  size              = 2
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					}
				`, poolNameUpdated),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_pool.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(poolNameUpdated),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolNameUpdated),
					resource.TestCheckResourceAttr("ceph_pool.test", "name", poolNameUpdated),
				),
			},
		},
	})
}

func TestAccCephPoolResource_ApplicationUpdateInPlace(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-pool-app-update")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephPoolDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  size              = 2
					  application_metadata = ["rbd"]
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "application_metadata.0", "rbd"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  size              = 2
					  application_metadata = ["rgw"]
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					}
				`, poolName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_pool.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "application_metadata.0", "rgw"),
				),
			},
		},
	})
}

func TestAccCephPoolResource_QuotaMaxObjects(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-pool-quota-objects")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephPoolDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  size              = 2
					  quota_max_objects = 1000
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					}
				`, poolName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("quota_max_objects"),
						knownvalue.Int64Exact(1000),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "quota_max_objects", "1000"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  size              = 2
					  quota_max_objects = 2000
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					}
				`, poolName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_pool.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("quota_max_objects"),
						knownvalue.Int64Exact(2000),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "quota_max_objects", "2000"),
				),
			},
		},
	})
}

func TestAccCephPoolResource_QuotaMaxBytes(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-pool-quota-bytes")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephPoolDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name            = %q
					  pool_type       = "replicated"
					  size            = 2
					  quota_max_bytes = 1073741824
					  pg_num          = 32
					  pg_autoscale_mode = "off"
					}
				`, poolName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("quota_max_bytes"),
						knownvalue.Int64Exact(1073741824),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "quota_max_bytes", "1073741824"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name            = %q
					  pool_type       = "replicated"
					  size            = 2
					  quota_max_bytes = 2147483648
					  pg_num          = 32
					  pg_autoscale_mode = "off"
					}
				`, poolName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_pool.test", plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("quota_max_bytes"),
						knownvalue.Int64Exact(2147483648),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "quota_max_bytes", "2147483648"),
				),
			},
		},
	})
}

func TestAccCephPoolResource_SizeRequiresReplace(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-pool-size-replace")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephPoolDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  size              = 2
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					}
				`, poolName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("size"),
						knownvalue.Int64Exact(2),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolSize(t, poolName, 2),
					resource.TestCheckResourceAttr("ceph_pool.test", "size", "2"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  size              = 3
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					}
				`, poolName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_pool.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("size"),
						knownvalue.Int64Exact(3),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					checkCephPoolSize(t, poolName, 3),
					resource.TestCheckResourceAttr("ceph_pool.test", "size", "3"),
				),
			},
		},
	})
}

func TestAccCephPoolResource_CrushRuleRequiresReplace(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandomWithPrefix("test-pool-crush-replace")
	crushRuleName1 := acctest.RandomWithPrefix("test-crush-rule-1")
	crushRuleName2 := acctest.RandomWithPrefix("test-crush-rule-2")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephPoolDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_crush_rule" "test1" {
					  name           = %q
					  pool_type      = "replicated"
					  failure_domain = "osd"
					}

					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  crush_rule        = ceph_crush_rule.test1.name
					  size              = 2
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					}
				`, crushRuleName1, poolName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("crush_rule"),
						knownvalue.StringExact(crushRuleName1),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "crush_rule", crushRuleName1),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_crush_rule" "test1" {
					  name           = %q
					  pool_type      = "replicated"
					  failure_domain = "osd"
					}

					resource "ceph_crush_rule" "test2" {
					  name           = %q
					  pool_type      = "replicated"
					  failure_domain = "osd"
					}

					resource "ceph_pool" "test" {
					  name              = %q
					  pool_type         = "replicated"
					  crush_rule        = ceph_crush_rule.test2.name
					  size              = 2
					  pg_num            = 32
					  pg_autoscale_mode = "off"
					}
				`, crushRuleName1, crushRuleName2, poolName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_pool.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_pool.test",
						tfjsonpath.New("crush_rule"),
						knownvalue.StringExact(crushRuleName2),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephPoolExists(t, poolName),
					resource.TestCheckResourceAttr("ceph_pool.test", "crush_rule", crushRuleName2),
				),
			},
		},
	})
}
