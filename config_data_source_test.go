package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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
			testAccPreCheckCephHealth(t)
			testAccPreCheckCleanConfigState(t)

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.ConfigSet(ctx, "global", configName, fmt.Sprintf("%d", globalValue)); err != nil {
				t.Fatalf("Failed to set global config: %v", err)
			}

			if err := cephTestClusterCLI.ConfigSet(ctx, "osd", configName, fmt.Sprintf("%d", osdValue)); err != nil {
				t.Fatalf("Failed to set osd config: %v", err)
			}

			if err := cephTestClusterCLI.ConfigSet(ctx, "osd.0", configName, fmt.Sprintf("%d", osd1Value)); err != nil {
				t.Fatalf("Failed to set osd.0 config: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()

				if err := cephTestClusterCLI.ConfigRemove(cleanupCtx, "global", configName); err != nil {
					t.Errorf("Failed to cleanup config global/%s: %v", configName, err)
				}
				if err := cephTestClusterCLI.ConfigRemove(cleanupCtx, "osd", configName); err != nil {
					t.Errorf("Failed to cleanup config osd/%s: %v", configName, err)
				}
				if err := cephTestClusterCLI.ConfigRemove(cleanupCtx, "osd.0", configName); err != nil {
					t.Errorf("Failed to cleanup config osd.0/%s: %v", configName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_config" "all" {}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.ceph_config.all", "configs.#"),
					checkConfigEntryExists(
						"data.ceph_config.all",
						"global",
						configName,
						fmt.Sprintf("%d.000000", globalValue),
					),
					checkConfigEntryExists(
						"data.ceph_config.all",
						"osd",
						configName,
						fmt.Sprintf("%d.000000", osdValue),
					),
					checkConfigEntryExists(
						"data.ceph_config.all",
						"osd.0",
						configName,
						fmt.Sprintf("%d.000000", osd1Value),
					),
				),
			},
		},
	})
}

func checkConfigEntryExists(resourceName, section, name, value string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s not found in state", resourceName)
		}

		configsCount := 0
		for key := range rs.Primary.Attributes {
			if key == "configs.#" {
				if _, err := fmt.Sscanf(rs.Primary.Attributes[key], "%d", &configsCount); err != nil {
					return fmt.Errorf("failed to parse configs count: %w", err)
				}
				break
			}
		}

		for i := 0; i < configsCount; i++ {
			sectionKey := fmt.Sprintf("configs.%d.section", i)
			nameKey := fmt.Sprintf("configs.%d.name", i)
			valueKey := fmt.Sprintf("configs.%d.value", i)

			if rs.Primary.Attributes[sectionKey] == section &&
				rs.Primary.Attributes[nameKey] == name &&
				rs.Primary.Attributes[valueKey] == value {
				return nil
			}
		}

		return fmt.Errorf("config entry not found: section=%s, name=%s, value=%s", section, name, value)
	}
}
