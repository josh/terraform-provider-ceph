package main

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCephPoolDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckCephHealth(t)

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.PoolCreate(ctx, poolName, 8, ""); err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}

			if err := cephTestClusterCLI.PoolSet(ctx, poolName, "pg_autoscale_mode", "off"); err != nil {
				t.Fatalf("Failed to disable autoscaler: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()

				if err := cephTestClusterCLI.PoolDelete(cleanupCtx, poolName); err != nil {
					t.Errorf("Failed to cleanup pool %s: %v", poolName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_pool" "test" {
						name = "%s"
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"name",
						poolName,
					),
					resource.TestCheckResourceAttrSet(
						"data.ceph_pool.test",
						"pool_id",
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"size",
						"1",
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"min_size",
						"1",
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"pg_num",
						"8",
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"crush_rule",
						"replicated_rule",
					),
				),
			},
		},
	})
}

func TestAccCephPoolDataSource_erasureCoded(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckCephHealth(t)

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.PoolCreate(ctx, poolName, 8, "erasure"); err != nil {
				t.Fatalf("Failed to create erasure coded pool: %v", err)
			}

			if err := cephTestClusterCLI.PoolSet(ctx, poolName, "pg_autoscale_mode", "off"); err != nil {
				t.Fatalf("Failed to disable autoscaler: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()

				if err := cephTestClusterCLI.PoolDelete(cleanupCtx, poolName); err != nil {
					t.Errorf("Failed to cleanup pool %s: %v", poolName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_pool" "test" {
						name = "%s"
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"name",
						poolName,
					),
					resource.TestCheckResourceAttrSet(
						"data.ceph_pool.test",
						"pool_id",
					),
					resource.TestCheckResourceAttrSet(
						"data.ceph_pool.test",
						"erasure_code_profile",
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"pg_num",
						"8",
					),
				),
			},
		},
	})
}

func TestAccCephPoolDataSource_withApplication(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckCephHealth(t)

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.PoolCreate(ctx, poolName, 8, ""); err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}

			if err := cephTestClusterCLI.PoolSet(ctx, poolName, "pg_autoscale_mode", "off"); err != nil {
				t.Fatalf("Failed to disable autoscaler: %v", err)
			}

			if err := cephTestClusterCLI.PoolApplicationEnable(ctx, poolName, "rbd"); err != nil {
				t.Fatalf("Failed to enable application: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()

				if err := cephTestClusterCLI.PoolDelete(cleanupCtx, poolName); err != nil {
					t.Errorf("Failed to cleanup pool %s: %v", poolName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_pool" "test" {
						name = "%s"
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"name",
						poolName,
					),
					resource.TestCheckResourceAttrSet(
						"data.ceph_pool.test",
						"pool_id",
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"application_metadata.#",
						"1",
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"application_metadata.0",
						"rbd",
					),
				),
			},
		},
	})
}

func TestAccCephPoolDataSource_compression(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckCephHealth(t)

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.PoolCreate(ctx, poolName, 8, ""); err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}

			if err := cephTestClusterCLI.PoolSet(ctx, poolName, "pg_autoscale_mode", "off"); err != nil {
				t.Fatalf("Failed to disable autoscaler: %v", err)
			}

			if err := cephTestClusterCLI.PoolSet(ctx, poolName, "compression_mode", "aggressive"); err != nil {
				t.Fatalf("Failed to set compression mode: %v", err)
			}

			if err := cephTestClusterCLI.PoolSet(ctx, poolName, "compression_algorithm", "snappy"); err != nil {
				t.Fatalf("Failed to set compression algorithm: %v", err)
			}

			if err := cephTestClusterCLI.PoolSet(ctx, poolName, "compression_required_ratio", "0.875"); err != nil {
				t.Fatalf("Failed to set compression required ratio: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()

				if err := cephTestClusterCLI.PoolDelete(cleanupCtx, poolName); err != nil {
					t.Errorf("Failed to cleanup pool %s: %v", poolName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_pool" "test" {
						name = "%s"
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"name",
						poolName,
					),
					resource.TestCheckResourceAttrSet(
						"data.ceph_pool.test",
						"pool_id",
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"compression_mode",
						"aggressive",
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"compression_algorithm",
						"snappy",
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"compression_required_ratio",
						"0.875",
					),
				),
			},
		},
	})
}

func TestAccCephPoolDataSource_configurationChanges(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckCephHealth(t)

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.PoolCreate(ctx, poolName, 8, ""); err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}

			if err := cephTestClusterCLI.PoolSet(ctx, poolName, "pg_autoscale_mode", "off"); err != nil {
				t.Fatalf("Failed to disable autoscaler: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()

				if err := cephTestClusterCLI.PoolDelete(cleanupCtx, poolName); err != nil {
					t.Errorf("Failed to cleanup pool %s: %v", poolName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_pool" "test" {
						name = "%s"
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"name",
						poolName,
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"pg_num",
						"8",
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"size",
						"1",
					),
				),
			},
			{
				PreConfig: func() {
					ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
					defer cancel()

					if err := cephTestClusterCLI.PoolSet(ctx, poolName, "pg_num", "16"); err != nil {
						t.Fatalf("Failed to set pg_num: %v", err)
					}

					if err := cephTestClusterCLI.PoolSet(ctx, poolName, "size", "2"); err != nil {
						t.Fatalf("Failed to set size: %v", err)
					}
				},
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_pool" "test" {
						name = "%s"
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"name",
						poolName,
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"pg_num",
						"16",
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"size",
						"2",
					),
				),
			},
		},
	})
}

func TestAccCephPoolDataSource_customPGCount(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckCephHealth(t)

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.PoolCreate(ctx, poolName, 32, ""); err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}

			if err := cephTestClusterCLI.PoolSet(ctx, poolName, "pg_autoscale_mode", "off"); err != nil {
				t.Fatalf("Failed to disable autoscaler: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()

				if err := cephTestClusterCLI.PoolDelete(cleanupCtx, poolName); err != nil {
					t.Errorf("Failed to cleanup pool %s: %v", poolName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_pool" "test" {
						name = "%s"
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"name",
						poolName,
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"pg_num",
						"32",
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"pg_placement_num",
						"32",
					),
				),
			},
		},
	})
}

func TestAccCephPoolDataSource_targetSize(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckCephHealth(t)

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.PoolCreate(ctx, poolName, 8, ""); err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}

			if err := cephTestClusterCLI.PoolSet(ctx, poolName, "pg_autoscale_mode", "off"); err != nil {
				t.Fatalf("Failed to disable autoscaler: %v", err)
			}

			if err := cephTestClusterCLI.PoolSet(ctx, poolName, "target_size_ratio", "0.1"); err != nil {
				t.Fatalf("Failed to set target size ratio: %v", err)
			}

			if err := cephTestClusterCLI.PoolSet(ctx, poolName, "target_size_bytes", "1073741824"); err != nil {
				t.Fatalf("Failed to set target size bytes: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()

				if err := cephTestClusterCLI.PoolDelete(cleanupCtx, poolName); err != nil {
					t.Errorf("Failed to cleanup pool %s: %v", poolName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_pool" "test" {
						name = "%s"
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"name",
						poolName,
					),
					resource.TestCheckResourceAttrSet(
						"data.ceph_pool.test",
						"pool_id",
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"target_size_ratio",
						"0.1",
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"target_size_bytes",
						"1073741824",
					),
				),
			},
		},
	})
}

func TestAccCephPoolDataSource_autoscaler(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckCephHealth(t)

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.PoolCreate(ctx, poolName, 8, ""); err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}

			if err := cephTestClusterCLI.PoolSet(ctx, poolName, "pg_autoscale_mode", "on"); err != nil {
				t.Fatalf("Failed to set autoscale mode: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()

				if err := cephTestClusterCLI.PoolDelete(cleanupCtx, poolName); err != nil {
					t.Errorf("Failed to cleanup pool %s: %v", poolName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_pool" "test" {
						name = "%s"
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"name",
						poolName,
					),
					resource.TestCheckResourceAttrSet(
						"data.ceph_pool.test",
						"pool_id",
					),
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"autoscale_mode",
						"on",
					),
				),
			},
		},
	})
}

func TestAccCephPoolDataSource_configuration(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckCephHealth(t)

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.PoolCreate(ctx, poolName, 8, ""); err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}

			if err := cephTestClusterCLI.PoolSet(ctx, poolName, "pg_autoscale_mode", "off"); err != nil {
				t.Fatalf("Failed to disable autoscaler: %v", err)
			}

			if err := cephTestClusterCLI.PoolSet(ctx, poolName, "noscrub", "true"); err != nil {
				t.Fatalf("Failed to set noscrub flag: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()

				if err := cephTestClusterCLI.PoolDelete(cleanupCtx, poolName); err != nil {
					t.Errorf("Failed to cleanup pool %s: %v", poolName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_pool" "test" {
						name = "%s"
					}
				`, poolName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.ceph_pool.test",
						"name",
						poolName,
					),
					resource.TestCheckResourceAttrSet(
						"data.ceph_pool.test",
						"pool_id",
					),
					resource.TestMatchResourceAttr(
						"data.ceph_pool.test",
						"configuration.#",
						regexp.MustCompile("^[1-9][0-9]*$"),
					),
				),
			},
		},
	})
}

func TestAccCephPoolDataSource_notFound(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	poolName := "nonexistent_" + acctest.RandString(8)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_pool" "test" {
						name = "%s"
					}
				`, poolName),
				ExpectError: regexp.MustCompile(`(?s).*[Uu]nable to get pool.*from [Cc]eph API.*`),
			},
		},
	})
}
