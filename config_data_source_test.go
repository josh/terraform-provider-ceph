package main

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephConfigDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_config" "all" {}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs").AtSliceIndex(0).AtMapKey("name"),
						knownvalue.StringExact("fsid"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs").AtSliceIndex(0).AtMapKey("section"),
						knownvalue.StringExact("mgr"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs").AtSliceIndex(0).AtMapKey("value"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs").AtSliceIndex(0).AtMapKey("level"),
						knownvalue.StringExact("basic"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs").AtSliceIndex(0).AtMapKey("can_update_at_runtime"),
						knownvalue.Bool(false),
					),
				},
			},
		},
	})
}

func TestAccCephConfigDataSource_multiLevel(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	globalValue := acctest.RandIntRange(100, 999)
	osdValue := acctest.RandIntRange(1000, 9999)
	osd1Value := acctest.RandIntRange(10000, 99999)
	configName := "osd_recovery_sleep"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			cmd1 := exec.CommandContext(ctx, "ceph", "--conf", testConfPath, "config", "set", "global", configName, fmt.Sprintf("%d", globalValue))
			if err := cmd1.Run(); err != nil {
				t.Fatalf("Failed to set global config: %v", err)
			}

			cmd2 := exec.CommandContext(ctx, "ceph", "--conf", testConfPath, "config", "set", "osd", configName, fmt.Sprintf("%d", osdValue))
			if err := cmd2.Run(); err != nil {
				t.Fatalf("Failed to set osd config: %v", err)
			}

			cmd3 := exec.CommandContext(ctx, "ceph", "--conf", testConfPath, "config", "set", "osd.0", configName, fmt.Sprintf("%d", osd1Value))
			if err := cmd3.Run(); err != nil {
				t.Fatalf("Failed to set osd.0 config: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cleanupCancel()

				_ = exec.CommandContext(cleanupCtx, "ceph", "--conf", testConfPath, "config", "rm", "global", configName).Run()
				_ = exec.CommandContext(cleanupCtx, "ceph", "--conf", testConfPath, "config", "rm", "osd", configName).Run()
				_ = exec.CommandContext(cleanupCtx, "ceph", "--conf", testConfPath, "config", "rm", "osd.0", configName).Run()
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_config" "all" {}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_config.all", "configs.#", "4"),
				),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs").AtSliceIndex(0).AtMapKey("section"),
						knownvalue.StringExact("mgr"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs").AtSliceIndex(0).AtMapKey("name"),
						knownvalue.StringExact("fsid"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs").AtSliceIndex(1).AtMapKey("section"),
						knownvalue.StringExact("global"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs").AtSliceIndex(1).AtMapKey("name"),
						knownvalue.StringExact(configName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs").AtSliceIndex(1).AtMapKey("value"),
						knownvalue.StringExact(fmt.Sprintf("%d.000000", globalValue)),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs").AtSliceIndex(2).AtMapKey("section"),
						knownvalue.StringExact("osd"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs").AtSliceIndex(2).AtMapKey("name"),
						knownvalue.StringExact(configName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs").AtSliceIndex(2).AtMapKey("value"),
						knownvalue.StringExact(fmt.Sprintf("%d.000000", osdValue)),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs").AtSliceIndex(3).AtMapKey("section"),
						knownvalue.StringExact("osd.0"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs").AtSliceIndex(3).AtMapKey("name"),
						knownvalue.StringExact(configName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config.all",
						tfjsonpath.New("configs").AtSliceIndex(3).AtMapKey("value"),
						knownvalue.StringExact(fmt.Sprintf("%d.000000", osd1Value)),
					),
				},
			},
		},
	})
}
