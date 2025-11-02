package main

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
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
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						configs = {
							"global" = {
								%q = "%d"
							}
							"osd" = {
								%q = "%d.000000"
							}
						}
					}

					data "ceph_config_value" "mon_value" {
						name    = %q
						section = "global"
						depends_on = [ceph_config.test]
					}

					data "ceph_config_value" "osd_value" {
						name    = %q
						section = "osd"
						depends_on = [ceph_config.test]
					}
				`, configName1, testValue1, configName2, testValue2, configName1, configName2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("configs"),
						knownvalue.NotNull(),
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
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						configs = {
							"global" = {
								%q = "%d"
							}
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
						tfjsonpath.New("configs"),
						knownvalue.NotNull(),
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
						configs = {
							"global" = {
								%q = "%d"
							}
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
						tfjsonpath.New("configs"),
						knownvalue.NotNull(),
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
						configs = {
							"osd" = {
								%q = "%d"
							}
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
						tfjsonpath.New("configs"),
						knownvalue.NotNull(),
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
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						configs = {
							"global" = {
								"mon_max_pg_per_osd" = "%d"
							}
							"osd" = {
								"osd_recovery_sleep" = "%d.000000"
							}
						}
					}

					data "ceph_config_value" "mon_value" {
						name    = "mon_max_pg_per_osd"
						section = "global"
						depends_on = [ceph_config.test]
					}

					data "ceph_config_value" "osd_value" {
						name    = "osd_recovery_sleep"
						section = "osd"
						depends_on = [ceph_config.test]
					}
				`, value1, value2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("configs"),
						knownvalue.NotNull(),
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
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						configs = {
							"global" = {
								"mon_max_pg_per_osd" = "%d"
							}
							"osd" = {
								"osd_recovery_sleep" = "%d.000000"
							}
						}
					}

					data "ceph_config_value" "mon_value" {
						name    = "mon_max_pg_per_osd"
						section = "global"
						depends_on = [ceph_config.test]
					}

					data "ceph_config_value" "osd_value" {
						name    = "osd_recovery_sleep"
						section = "osd"
						depends_on = [ceph_config.test]
					}
				`, customValue1, customValue2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("configs"),
						knownvalue.NotNull(),
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
					resource "ceph_config" "test" {
						configs = {
							"osd" = {
								"osd_recovery_sleep" = "%d.000000"
							}
						}
					}

					data "ceph_config_value" "osd_value" {
						name    = "osd_recovery_sleep"
						section = "osd"
						depends_on = [ceph_config.test]
					}

					data "ceph_config" "all" {
						depends_on = [ceph_config.test]
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
					resource.TestCheckResourceAttr("ceph_config.test", "configs.osd.osd_recovery_sleep", fmt.Sprintf("%d.000000", customValue2)),
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
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						configs = {
							"global" = {
								%q = "%d"
							}
						}
					}
				`, configName, testValue),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("configs"),
						knownvalue.NotNull(),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						configs = {
							"global" = {
								%q = "%d"
							}
						}
					}
				`, configName, testValue),
				ResourceName:  "ceph_config.test",
				ImportState:   true,
				ImportStateId: fmt.Sprintf("global.%s", configName),
			},
		},
	})
}

func TestAccCephConfigResource_importMultiple(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	value1 := acctest.RandIntRange(100, 999)
	value2 := acctest.RandIntRange(1000, 9999)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						configs = {
							"global" = {
								"mon_max_pg_per_osd" = "%d"
							}
							"osd" = {
								"osd_recovery_sleep" = "%d.000000"
							}
						}
					}
				`, value1, value2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_config.test",
						tfjsonpath.New("configs"),
						knownvalue.NotNull(),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_config" "test" {
						configs = {
							"global" = {
								"mon_max_pg_per_osd" = "%d"
							}
							"osd" = {
								"osd_recovery_sleep" = "%d.000000"
							}
						}
					}
				`, value1, value2),
				ResourceName:  "ceph_config.test",
				ImportState:   true,
				ImportStateId: "global.mon_max_pg_per_osd,osd.osd_recovery_sleep",
			},
		},
	})
}

func TestAccCephConfigResource_MgrConfigRejection(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_config" "test" {
						configs = {
							"mgr" = {
								"mgr/dashboard/ssl" = "false"
							}
						}
					}
				`,
				ExpectError: regexp.MustCompile("cannot be managed via ceph_config"),
			},
		},
	})
}
