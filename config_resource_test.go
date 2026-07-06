package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephConfigResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testValue1 := acctest.RandIntRange(100, 999)
	testValue2 := acctest.RandIntRange(1000, 9999)
	configName1 := "mon_max_pg_per_osd"
	configName2 := "osd_recovery_sleep"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "global" {
						section = "global"
						config = {
							%q = "%d"
						}
					}

					resource "ceph_config" "osd" {
						section = "osd"
						config = {
							%q = "%d.000000"
						}
					}

					data "ceph_config_value" "mon_value" {
						name    = %q
						section = "global"
						depends_on = [ceph_config.global]
					}

					data "ceph_config_value" "osd_value" {
						name    = %q
						section = "osd"
						depends_on = [ceph_config.osd]
					}
				`, configName1, testValue1, configName2, testValue2, configName1, configName2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.global",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							configName1: knownvalue.StringExact(fmt.Sprintf("%d", testValue1)),
						}),
					),
					statecheck.ExpectKnownValue(
						"ceph_config.osd",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							configName2: knownvalue.StringExact(fmt.Sprintf("%d.000000", testValue2)),
						}),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.mon_value",
						tfjsonpath.New("value"),
						knownvalue.StringExact(fmt.Sprintf("%d", testValue1)),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.osd_value",
						tfjsonpath.New("value"),
						knownvalue.StringExact(fmt.Sprintf("%d.000000", testValue2)),
					),
				},
			},
		},
	})
}

func TestAccCephConfigResource_update(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testValue1 := acctest.RandIntRange(100, 999)
	testValue2 := acctest.RandIntRange(1000, 9999)
	configName := "mon_max_pg_per_osd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						section = "global"
						config = {
							%q = "%d"
						}
					}

					data "ceph_config_value" "test_value" {
						name    = %q
						section = "global"
						depends_on = [ceph_config.test]
					}
				`, configName, testValue1, configName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							configName: knownvalue.StringExact(fmt.Sprintf("%d", testValue1)),
						}),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.test_value",
						tfjsonpath.New("value"),
						knownvalue.StringExact(fmt.Sprintf("%d", testValue1)),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						section = "global"
						config = {
							%q = "%d"
						}
					}

					data "ceph_config_value" "test_value" {
						name    = %q
						section = "global"
						depends_on = [ceph_config.test]
					}
				`, configName, testValue2, configName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							configName: knownvalue.StringExact(fmt.Sprintf("%d", testValue2)),
						}),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.test_value",
						tfjsonpath.New("value"),
						knownvalue.StringExact(fmt.Sprintf("%d", testValue2)),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						section = "osd"
						config = {
							%q = "%d"
						}
					}

					data "ceph_config_value" "test_value" {
						name    = %q
						section = "osd"
						depends_on = [ceph_config.test]
					}
				`, configName, testValue2, configName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							configName: knownvalue.StringExact(fmt.Sprintf("%d", testValue2)),
						}),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.test_value",
						tfjsonpath.New("value"),
						knownvalue.StringExact(fmt.Sprintf("%d", testValue2)),
					),
				},
			},
		},
	})
}

func TestAccCephConfigResource_multipleConfigs(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	value1 := acctest.RandIntRange(100, 999)
	value2 := acctest.RandIntRange(1000, 9999)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "global" {
						section = "global"
						config = {
							"mon_max_pg_per_osd" = "%d"
						}
					}

					resource "ceph_config" "osd" {
						section = "osd"
						config = {
							"osd_recovery_sleep" = "%d.000000"
						}
					}

					data "ceph_config_value" "mon_value" {
						name    = "mon_max_pg_per_osd"
						section = "global"
						depends_on = [ceph_config.global]
					}

					data "ceph_config_value" "osd_value" {
						name    = "osd_recovery_sleep"
						section = "osd"
						depends_on = [ceph_config.osd]
					}
				`, value1, value2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.global",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"mon_max_pg_per_osd": knownvalue.StringExact(fmt.Sprintf("%d", value1)),
						}),
					),
					statecheck.ExpectKnownValue(
						"ceph_config.osd",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"osd_recovery_sleep": knownvalue.StringExact(fmt.Sprintf("%d.000000", value2)),
						}),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.mon_value",
						tfjsonpath.New("value"),
						knownvalue.StringExact(fmt.Sprintf("%d", value1)),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.osd_value",
						tfjsonpath.New("value"),
						knownvalue.StringExact(fmt.Sprintf("%d.000000", value2)),
					),
				},
			},
		},
	})
}

func TestAccCephConfigResource_removeConfig(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	customValue1 := acctest.RandIntRange(100, 999)
	customValue2 := acctest.RandIntRange(1000, 9999)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "global" {
						section = "global"
						config = {
							"mon_max_pg_per_osd" = "%d"
						}
					}

					resource "ceph_config" "osd" {
						section = "osd"
						config = {
							"osd_recovery_sleep" = "%d.000000"
						}
					}

					data "ceph_config_value" "mon_value" {
						name    = "mon_max_pg_per_osd"
						section = "global"
						depends_on = [ceph_config.global]
					}

					data "ceph_config_value" "osd_value" {
						name    = "osd_recovery_sleep"
						section = "osd"
						depends_on = [ceph_config.osd]
					}
				`, customValue1, customValue2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.global",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"mon_max_pg_per_osd": knownvalue.StringExact(fmt.Sprintf("%d", customValue1)),
						}),
					),
					statecheck.ExpectKnownValue(
						"ceph_config.osd",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"osd_recovery_sleep": knownvalue.StringExact(fmt.Sprintf("%d.000000", customValue2)),
						}),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.mon_value",
						tfjsonpath.New("value"),
						knownvalue.StringExact(fmt.Sprintf("%d", customValue1)),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.osd_value",
						tfjsonpath.New("value"),
						knownvalue.StringExact(fmt.Sprintf("%d.000000", customValue2)),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "osd" {
						section = "osd"
						config = {
							"osd_recovery_sleep" = "%d.000000"
						}
					}

					data "ceph_config_value" "osd_value" {
						name    = "osd_recovery_sleep"
						section = "osd"
						depends_on = [ceph_config.osd]
					}

					data "ceph_config" "all" {
						depends_on = [ceph_config.osd]
					}
				`, customValue2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.osd_value",
						tfjsonpath.New("value"),
						knownvalue.StringExact(fmt.Sprintf("%d.000000", customValue2)),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_config.osd", "config.osd_recovery_sleep", fmt.Sprintf("%d.000000", customValue2)),
				),
			},
		},
	})
}

func TestAccCephConfigResource_import(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testValue := acctest.RandIntRange(100, 999)
	configName := "mon_max_pg_per_osd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						section = "global"
						config = {
							%q = "%d"
						}
					}
				`, configName, testValue),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							configName: knownvalue.StringExact(fmt.Sprintf("%d", testValue)),
						}),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						section = "global"
						config = {
							%q = "%d"
						}
					}
				`, configName, testValue),
				ResourceName:  "ceph_config.test",
				ImportState:   true,
				ImportStateId: "global",
			},
		},
	})
}

func TestAccCephConfigResource_importMultiple(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	value1 := acctest.RandIntRange(100, 999)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "global" {
						section = "global"
						config = {
							"mon_max_pg_per_osd" = "%d"
							"osd_recovery_sleep" = "0.100000"
						}
					}
				`, value1),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.global",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"mon_max_pg_per_osd": knownvalue.StringExact(fmt.Sprintf("%d", value1)),
							"osd_recovery_sleep": knownvalue.StringExact("0.100000"),
						}),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "global" {
						section = "global"
						config = {
							"mon_max_pg_per_osd" = "%d"
							"osd_recovery_sleep" = "0.100000"
						}
					}
				`, value1),
				ResourceName:  "ceph_config.global",
				ImportState:   true,
				ImportStateId: "global",
			},
		},
	})
}

func TestAccCephConfigResource_MgrConfigRejection(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_config" "test" {
						section = "mgr"
						config = {
							"mgr/dashboard/ssl" = "false"
						}
					}
				`,
				ExpectError: regexp.MustCompile("cannot be managed via ceph_config"),
			},
		},
	})
}

func TestAccCephConfigResource_bulkImport(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	value1 := acctest.RandIntRange(100, 999)
	value2 := acctest.RandIntRange(1000, 9999)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "global" {
						section = "global"
						config = {
							"mon_max_pg_per_osd" = "%d"
						}
					}

					resource "ceph_config" "osd" {
						section = "osd"
						config = {
							"osd_recovery_sleep" = "%d.000000"
						}
					}
				`, value1, value2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.global",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"mon_max_pg_per_osd": knownvalue.StringExact(fmt.Sprintf("%d", value1)),
						}),
					),
					statecheck.ExpectKnownValue(
						"ceph_config.osd",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"osd_recovery_sleep": knownvalue.StringExact(fmt.Sprintf("%d.000000", value2)),
						}),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "global" {
						section = "global"
						config = {
							"mon_max_pg_per_osd" = "%d"
						}
					}

					resource "ceph_config" "osd" {
						section = "osd"
						config = {
							"osd_recovery_sleep" = "%d.000000"
						}
					}
				`, value1, value2),
				ResourceName:  "ceph_config.global",
				ImportState:   true,
				ImportStateId: "global",
			},
		},
	})
}

func TestAccCephConfigResource_nativeIntValue(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testValue1 := acctest.RandIntRange(100, 999)
	testValue2 := acctest.RandIntRange(1000, 9999)
	configName := "mon_max_pg_per_osd"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						section = "global"
						config = {
							%s = %d
						}
					}

					data "ceph_config_value" "test_value" {
						name    = %q
						section = "global"
						depends_on = [ceph_config.test]
					}
				`, configName, testValue1, configName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							configName: knownvalue.StringExact(fmt.Sprintf("%d", testValue1)),
						}),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.test_value",
						tfjsonpath.New("value"),
						knownvalue.StringExact(fmt.Sprintf("%d", testValue1)),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_config.test", "config.mon_max_pg_per_osd", fmt.Sprintf("%d", testValue1)),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						section = "global"
						config = {
							%s = %d
						}
					}

					data "ceph_config_value" "test_value" {
						name    = %q
						section = "global"
						depends_on = [ceph_config.test]
					}
				`, configName, testValue2, configName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							configName: knownvalue.StringExact(fmt.Sprintf("%d", testValue2)),
						}),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.test_value",
						tfjsonpath.New("value"),
						knownvalue.StringExact(fmt.Sprintf("%d", testValue2)),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_config.test", "config.mon_max_pg_per_osd", fmt.Sprintf("%d", testValue2)),
				),
			},
		},
	})
}

func TestAccCephConfigResource_normalizedValues(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	config := testAccProviderConfigBlock + `
		resource "ceph_config" "test" {
			section = "osd"
			config = {
				osd_scrub_load_threshold = "0.5"
				osd_scrub_auto_repair    = "1"
				osd_max_object_size      = "256Mi"
			}
		}
	`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"osd_scrub_load_threshold": knownvalue.StringExact("0.5"),
							"osd_scrub_auto_repair":    knownvalue.StringExact("1"),
							"osd_max_object_size":      knownvalue.StringExact("256Mi"),
						}),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccCephConfigResource_nativeBoolValue(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	configName := "mon_allow_pool_delete"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						section = "global"
						config = {
							%s = true
						}
					}

					data "ceph_config_value" "test_value" {
						name    = %q
						section = "global"
						depends_on = [ceph_config.test]
					}
				`, configName, configName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							configName: knownvalue.StringExact("true"),
						}),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.test_value",
						tfjsonpath.New("value"),
						knownvalue.StringExact("true"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_config.test", "config.mon_allow_pool_delete", "true"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						section = "global"
						config = {
							%s = false
						}
					}

					data "ceph_config_value" "test_value" {
						name    = %q
						section = "global"
						depends_on = [ceph_config.test]
					}
				`, configName, configName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("config"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							configName: knownvalue.StringExact("false"),
						}),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_config_value.test_value",
						tfjsonpath.New("value"),
						knownvalue.StringExact("false"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_config.test", "config.mon_allow_pool_delete", "false"),
				),
			},
		},
	})
}

func TestAccCephConfigResource_invalidSectionRejection(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_config" "test" {
						section = "invalid_section"
						config = {
							"some_option" = "value"
						}
					}
				`,
				ExpectError: regexp.MustCompile("(?i)(invalid|unknown|not found|unrecognized).*section"),
			},
		},
	})
}

func TestAccCephConfigResource_numericTypeBoundaries(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_config" "test" {
						section = "global"
						config = {
							"mon_max_pg_per_osd" = "2147483647"
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("config").AtMapKey("mon_max_pg_per_osd"),
						knownvalue.StringExact("2147483647"),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_config" "test" {
						section = "global"
						config = {
							"mon_max_pg_per_osd" = "1"
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("config").AtMapKey("mon_max_pg_per_osd"),
						knownvalue.StringExact("1"),
					),
				},
			},
		},
	})
}

func TestAccCephConfigResource_partialUpdate(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	value1 := acctest.RandIntRange(100, 999)
	value2 := acctest.RandIntRange(1000, 9999)
	value3 := acctest.RandIntRange(10000, 99999)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						section = "global"
						config = {
							"mon_max_pg_per_osd" = "%d"
							"mon_osd_down_out_interval" = "%d"
						}
					}
				`, value1, value2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("config").AtMapKey("mon_max_pg_per_osd"),
						knownvalue.StringExact(fmt.Sprintf("%d", value1)),
					),
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("config").AtMapKey("mon_osd_down_out_interval"),
						knownvalue.StringExact(fmt.Sprintf("%d", value2)),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						section = "global"
						config = {
							"mon_max_pg_per_osd" = "%d"
							"mon_osd_down_out_interval" = "%d"
							"mon_max_pool_pg_num" = "%d"
						}
					}
				`, value3, value2, value1),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("config").AtMapKey("mon_max_pg_per_osd"),
						knownvalue.StringExact(fmt.Sprintf("%d", value3)),
					),
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("config").AtMapKey("mon_osd_down_out_interval"),
						knownvalue.StringExact(fmt.Sprintf("%d", value2)),
					),
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("config").AtMapKey("mon_max_pool_pg_num"),
						knownvalue.StringExact(fmt.Sprintf("%d", value1)),
					),
				},
			},
		},
	})
}

func TestAccCephConfigResource_differentSections(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	globalValue := acctest.RandIntRange(100, 999)
	monValue := acctest.RandIntRange(1000, 9999)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "global" {
						section = "global"
						config = {
							"mon_osd_down_out_interval" = "%d"
						}
					}

					resource "ceph_config" "mon" {
						section = "mon"
						config = {
							"mon_max_pg_per_osd" = "%d"
						}
					}
				`, globalValue, monValue),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_config.global", "config.mon_osd_down_out_interval", fmt.Sprintf("%d", globalValue)),
					resource.TestCheckResourceAttr("ceph_config.mon", "config.mon_max_pg_per_osd", fmt.Sprintf("%d", monValue)),
				),
			},
		},
	})
}

func checkCephConfigExists(t *testing.T, section, name string) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		value, err := cephTestClusterCLI.ConfigGetFromDump(t.Context(), section, name)
		if err != nil {
			return fmt.Errorf("failed to get config %s/%s: %w", section, name, err)
		}

		if value == "" {
			return fmt.Errorf("config %s/%s not found", section, name)
		}

		t.Logf("Verified config %s/%s exists with value: %s", section, name, value)
		return nil
	}
}

func TestAccCephConfigResource_OutOfBandDeletion(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testValue := acctest.RandIntRange(100, 999)
	configName := "mon_max_pg_per_osd"
	section := "global"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						section = %q
						config = {
							%q = "%d"
						}
					}
				`, section, configName, testValue),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephConfigExists(t, section, configName),
					resource.TestCheckResourceAttr("ceph_config.test", "section", section),
					resource.TestCheckResourceAttr("ceph_config.test", "config."+configName, fmt.Sprintf("%d", testValue)),
				),
			},
			{
				PreConfig: func() {
					err := cephTestClusterCLI.ConfigRemove(t.Context(), section, configName)
					if err != nil {
						t.Fatalf("Failed to delete config out of band: %v", err)
					}
					t.Logf("Deleted config %s/%s out of band", section, configName)
				},
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						section = %q
						config = {
							%q = "%d"
						}
					}
				`, section, configName, testValue),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_config.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephConfigExists(t, section, configName),
					resource.TestCheckResourceAttr("ceph_config.test", "section", section),
					resource.TestCheckResourceAttr("ceph_config.test", "config."+configName, fmt.Sprintf("%d", testValue)),
				),
			},
		},
	})
}

func TestAccCephConfigResource_OutOfBandDeletionDestroy(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testValue := acctest.RandIntRange(100, 999)
	configName := "mon_max_pg_per_osd"
	section := "global"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						section = %q
						config = {
							%q = "%d"
						}
					}
				`, section, configName, testValue),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephConfigExists(t, section, configName),
				),
			},
			{
				PreConfig: func() {
					err := cephTestClusterCLI.ConfigRemove(t.Context(), section, configName)
					if err != nil {
						t.Fatalf("Failed to delete config out of band: %v", err)
					}
					t.Logf("Deleted config %s/%s out of band", section, configName)
				},
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccProviderConfigBlock,
			},
		},
	})
}

func testAccCheckCephConfigDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_config" {
				continue
			}

			section := rs.Primary.Attributes["section"]

			for key := range rs.Primary.Attributes {
				if !strings.HasPrefix(key, "config.") || key == "config.%" {
					continue
				}

				configName := strings.TrimPrefix(key, "config.")

				_, err := cephTestClusterCLI.ConfigGetFromDump(ctx, section, configName)
				if err == nil {
					return fmt.Errorf("ceph_config %s/%s still exists after destroy", section, configName)
				}
			}
		}
		return nil
	}
}

func TestAccCephConfigResource_emptyValueRejected(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_config" "test" {
						section = "global"
						config = {
							"mon_max_pg_per_osd" = ""
						}
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)cannot be an empty string`),
			},
		},
	})
}

func TestAccCephConfigResource_importMgrSkipsFsid(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_config" "test" {
						section = "mgr"
						config = {
							"mgr_stats_period" = "7"
						}
					}
				`,
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_config" "test" {
						section = "mgr"
						config = {
							"mgr_stats_period" = "7"
						}
					}
				`,
				ResourceName:  "ceph_config.test",
				ImportState:   true,
				ImportStateId: "mgr",
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["config.mgr_stats_period"] != "7" {
						return fmt.Errorf("expected config.mgr_stats_period=7, got %q", attrs["config.mgr_stats_period"])
					}
					// The dashboard injects a synthetic fsid entry into the
					// mgr section; it must not be imported.
					if v, ok := attrs["config.fsid"]; ok {
						return fmt.Errorf("imported state unexpectedly contains fsid=%q", v)
					}
					return nil
				},
			},
		},
	})
}

func TestAccCephConfigResource_maskSectionRejected(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_config" "test" {
						section = "osd/class:ssd"
						config = {
							"osd_max_backfills" = "3"
						}
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)unsupported section mask`),
			},
		},
	})
}
