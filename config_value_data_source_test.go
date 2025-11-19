package main

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephConfigValueDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testValue := acctest.RandIntRange(100, 999)
	configName := "mon_max_pg_per_osd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckCephHealth(t)

			if err := cephTestClusterCLI.ConfigSet(t.Context(), "global", configName, fmt.Sprintf("%d", testValue)); err != nil {
				t.Fatalf("Failed to set test config: %v", err)
			}

			testCleanup(t, func(ctx context.Context) {
				if err := cephTestClusterCLI.ConfigRemove(ctx, "global", configName); err != nil {
					t.Errorf("Failed to cleanup config global/%s: %v", configName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_config_value" "test" {
					  name    = "%s"
					  section = "global"
					}
				`, configName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.test",
						tfjsonpath.New("value"),
						knownvalue.StringExact(fmt.Sprintf("%d", testValue)),
					),
				},
			},
		},
	})
}

func TestAccCephConfigValueDataSource_nonExistent(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_config_value" "nonexistent" {
					  name    = "does_not_exist_config_option"
					  section = "global"
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)unable to get cluster configuration`),
			},
		},
	})
}

func TestAccCephConfigValueDataSource_multipleSections(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testValue1 := acctest.RandIntRange(100, 999)
	testValue2 := acctest.RandIntRange(1000, 9999)
	configName := "mon_max_pg_per_osd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckCephHealth(t)

			if err := cephTestClusterCLI.ConfigSet(t.Context(), "global", configName, fmt.Sprintf("%d", testValue1)); err != nil {
				t.Fatalf("Failed to set test config for global: %v", err)
			}

			if err := cephTestClusterCLI.ConfigSet(t.Context(), "mon", configName, fmt.Sprintf("%d", testValue2)); err != nil {
				t.Fatalf("Failed to set test config for mon: %v", err)
			}

			testCleanup(t, func(ctx context.Context) {
				if err := cephTestClusterCLI.ConfigRemove(ctx, "global", configName); err != nil {
					t.Errorf("Failed to cleanup config global/%s: %v", configName, err)
				}
				if err := cephTestClusterCLI.ConfigRemove(ctx, "mon", configName); err != nil {
					t.Errorf("Failed to cleanup config mon/%s: %v", configName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_config_value" "global_value" {
					  name    = "%s"
					  section = "global"
					}

					data "ceph_config_value" "mon_value" {
					  name    = "%s"
					  section = "mon"
					}
				`, configName, configName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.global_value",
						tfjsonpath.New("value"),
						knownvalue.StringExact(fmt.Sprintf("%d", testValue1)),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.mon_value",
						tfjsonpath.New("value"),
						knownvalue.StringExact(fmt.Sprintf("%d", testValue2)),
					),
				},
			},
		},
	})
}

func TestAccCephConfigValueDataSource_MgrConfigRejection(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_config_value" "test" {
					  name    = "mgr/dashboard/ssl"
					  section = "mgr"
					}
				`,
				ExpectError: regexp.MustCompile("is not available via ceph_config_value"),
			},
		},
	})
}

func TestAccCephConfigValueDataSource_sectionNotFound(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_config_value" "test" {
					  name    = "some_option"
					  section = "invalid_section"
					}
				`,
				ExpectError: regexp.MustCompile("(?i)(not found|invalid|unknown|unrecognized)"),
			},
		},
	})
}

func TestAccCephConfigValueDataSource_readMaskedConfig(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testValue := acctest.RandIntRange(100, 999)
	configName := "osd_max_backfills"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckCephHealth(t)

			if err := cephTestClusterCLI.ConfigSet(t.Context(), "osd/class:ssd", configName, fmt.Sprintf("%d", testValue)); err != nil {
				t.Fatalf("Failed to set masked config via CLI: %v", err)
			}

			testCleanup(t, func(ctx context.Context) {
				if err := cephTestClusterCLI.ConfigRemove(ctx, "osd/class:ssd", configName); err != nil {
					t.Errorf("Failed to cleanup config osd/class:ssd/%s: %v", configName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_config_value" "masked" {
					  name    = "%s"
					  section = "osd"
					}
				`, configName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.masked",
						tfjsonpath.New("section"),
						knownvalue.StringExact("osd"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.masked",
						tfjsonpath.New("value"),
						knownvalue.StringExact(fmt.Sprintf("%d", testValue)),
					),
				},
			},
		},
	})
}
