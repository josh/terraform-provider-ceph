package main

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"testing"
	"time"

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
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "ceph", "--conf", testConfPath, "config", "set", "global", configName, fmt.Sprintf("%d", testValue))
			if err := cmd.Run(); err != nil {
				t.Fatalf("Failed to set test config: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cleanupCancel()

				cleanupCmd := exec.CommandContext(cleanupCtx, "ceph", "--conf", testConfPath, "config", "rm", "global", configName)
				_ = cleanupCmd.Run()
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
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			cmd1 := exec.CommandContext(ctx, "ceph", "--conf", testConfPath, "config", "set", "global", configName, fmt.Sprintf("%d", testValue1))
			if err := cmd1.Run(); err != nil {
				t.Fatalf("Failed to set test config for global: %v", err)
			}

			cmd2 := exec.CommandContext(ctx, "ceph", "--conf", testConfPath, "config", "set", "mon", configName, fmt.Sprintf("%d", testValue2))
			if err := cmd2.Run(); err != nil {
				t.Fatalf("Failed to set test config for mon: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cleanupCancel()

				cleanupCmd1 := exec.CommandContext(cleanupCtx, "ceph", "--conf", testConfPath, "config", "rm", "global", configName)
				_ = cleanupCmd1.Run()

				cleanupCmd2 := exec.CommandContext(cleanupCtx, "ceph", "--conf", testConfPath, "config", "rm", "mon", configName)
				_ = cleanupCmd2.Run()
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
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, "ceph", "--conf", testConfPath, "config", "set", "osd/class:ssd", configName, fmt.Sprintf("%d", testValue))
			if err := cmd.Run(); err != nil {
				t.Fatalf("Failed to set masked config via CLI: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cleanupCancel()

				cleanupCmd := exec.CommandContext(cleanupCtx, "ceph", "--conf", testConfPath, "config", "rm", "osd/class:ssd", configName)
				_ = cleanupCmd.Run()
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
