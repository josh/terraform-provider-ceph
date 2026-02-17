package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func testAccCheckCephMgrModuleConfigDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		configDump, err := cephTestClusterCLI.ConfigDump(ctx)
		if err != nil {
			return fmt.Errorf("failed to get config dump: %w", err)
		}

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_mgr_module_config" {
				continue
			}

			moduleName := rs.Primary.Attributes["module_name"]

			for key := range rs.Primary.Attributes {
				if !strings.HasPrefix(key, "configs.") || key == "configs.%" {
					continue
				}

				configKey := strings.TrimPrefix(key, "configs.")
				fullKey := fmt.Sprintf("mgr/%s/%s", moduleName, configKey)

				for _, entry := range configDump {
					if entry.Section == "mgr" && entry.Name == fullKey {
						return fmt.Errorf("config %s still exists in config dump after destroy", fullKey)
					}
				}
			}
		}

		return nil
	}
}

func TestAccCephMgrModuleConfigResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephMgrModuleConfigDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							ssl         = "false"
							server_port = "8080"
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact("dashboard"),
					),
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("module_name"),
						knownvalue.StringExact("dashboard"),
					),
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("ssl"),
						knownvalue.StringExact("false"),
					),
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("server_port"),
						knownvalue.StringExact("8080"),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							ssl         = "true"
							server_port = "8443"
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("ssl"),
						knownvalue.StringExact("true"),
					),
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("server_port"),
						knownvalue.StringExact("8443"),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				ResourceName:    "ceph_mgr_module_config.test",
				ImportState:     true,
				ImportStateId:   "dashboard",
			},
		},
	})
}

func TestAccCephMgrModuleConfigResource_nonStringLiterals(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephMgrModuleConfigDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							ssl         = false
							server_port = 8080
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("ssl"),
						knownvalue.StringExact("false"),
					),
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("server_port"),
						knownvalue.StringExact("8080"),
					),
				},
			},
		},
	})
}

func TestAccCephMgrModuleConfigResource_delete(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephMgrModuleConfigDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							standby_behaviour = "redirect"
							standby_error_status_code = "503"
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("standby_behaviour"),
						knownvalue.StringExact("redirect"),
					),
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("standby_error_status_code"),
						knownvalue.StringExact("503"),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccProviderConfigBlock,
			},
		},
	})
}

func TestAccCephMgrModuleConfigResource_importOnlyExplicitlySet(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							standby_error_status_code = "503"
							url_prefix = "/test"
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("standby_error_status_code"),
						knownvalue.StringExact("503"),
					),
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("url_prefix"),
						knownvalue.StringExact("/test"),
					),
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				ResourceName:    "ceph_mgr_module_config.test",
				ImportState:     true,
				ImportStateId:   "dashboard",
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}

					state := states[0]
					configs := make(map[string]string)
					for k, v := range state.Attributes {
						if strings.HasPrefix(k, "configs.") && k != "configs.%" {
							key := strings.TrimPrefix(k, "configs.")
							configs[key] = v
						}
					}

					if _, ok := configs["standby_error_status_code"]; !ok {
						return fmt.Errorf("expected 'standby_error_status_code' config in imported state")
					}
					if _, ok := configs["url_prefix"]; !ok {
						return fmt.Errorf("expected 'url_prefix' config in imported state")
					}

					return nil
				},
			},
		},
	})
}

func TestAccCephMgrModuleConfigResource_largeIntegerValues(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							jwt_token_ttl = "31556952"
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("jwt_token_ttl"),
						knownvalue.StringExact("31556952"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						return assertCephMgrModuleConfigValue(t.Context(), "dashboard", "jwt_token_ttl", "31556952")
					},
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				ResourceName:    "ceph_mgr_module_config.test",
				ImportState:     true,
				ImportStateId:   "dashboard",
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					state := states[0]
					ttl := state.Attributes["configs.jwt_token_ttl"]
					if ttl != "31556952" {
						return fmt.Errorf("expected jwt_token_ttl='31556952', got '%s'", ttl)
					}
					return nil
				},
			},
		},
	})
}

func TestAccCephMgrModuleConfigResource_mixedNumericTypes(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephMgrModuleConfigDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							server_port                = 8080
							standby_error_status_code  = "503"
							jwt_token_ttl              = 31556952
							ssl                        = false
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("server_port"),
						knownvalue.StringExact("8080"),
					),
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("standby_error_status_code"),
						knownvalue.StringExact("503"),
					),
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("jwt_token_ttl"),
						knownvalue.StringExact("31556952"),
					),
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("ssl"),
						knownvalue.StringExact("false"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						if err := assertCephMgrModuleConfigValue(t.Context(), "dashboard", "server_port", "8080"); err != nil {
							return err
						}
						if err := assertCephMgrModuleConfigValue(t.Context(), "dashboard", "standby_error_status_code", "503"); err != nil {
							return err
						}
						if err := assertCephMgrModuleConfigValue(t.Context(), "dashboard", "jwt_token_ttl", "31556952"); err != nil {
							return err
						}
						return assertCephMgrModuleConfigValue(t.Context(), "dashboard", "ssl", "false")
					},
				),
			},
		},
	})
}

func TestAccCephMgrModuleConfigResource_booleanValues(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephMgrModuleConfigDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							ssl = false
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("ssl"),
						knownvalue.StringExact("false"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						return assertCephMgrModuleConfigValue(t.Context(), "dashboard", "ssl", "false")
					},
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							ssl = "false"
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("ssl"),
						knownvalue.StringExact("false"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						return assertCephMgrModuleConfigValue(t.Context(), "dashboard", "ssl", "false")
					},
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							ssl = true
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("ssl"),
						knownvalue.StringExact("true"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						return assertCephMgrModuleConfigValue(t.Context(), "dashboard", "ssl", "true")
					},
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							ssl = "true"
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("ssl"),
						knownvalue.StringExact("true"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						return assertCephMgrModuleConfigValue(t.Context(), "dashboard", "ssl", "true")
					},
				),
			},
		},
	})
}

func TestAccCephMgrModuleConfigResource_stringValues(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							url_prefix        = "/ceph-dashboard"
							standby_behaviour = "redirect"
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("url_prefix"),
						knownvalue.StringExact("/ceph-dashboard"),
					),
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("standby_behaviour"),
						knownvalue.StringExact("redirect"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						if err := assertCephMgrModuleConfigValue(t.Context(), "dashboard", "url_prefix", "/ceph-dashboard"); err != nil {
							return err
						}
						return assertCephMgrModuleConfigValue(t.Context(), "dashboard", "standby_behaviour", "redirect")
					},
				),
			},
		},
	})
}

func TestAccCephMgrModuleConfigResource_importLargeInteger(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							jwt_token_ttl = "31556952"
						}
					}
				`,
			},
			{
				ConfigVariables: testAccProviderConfig(),
				ResourceName:    "ceph_mgr_module_config.test",
				ImportState:     true,
				ImportStateId:   "dashboard",
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					state := states[0]
					ttl := state.Attributes["configs.jwt_token_ttl"]

					if ttl == "" {
						return fmt.Errorf("jwt_token_ttl not found in imported state")
					}

					if ttl != "31556952" {
						return fmt.Errorf("expected jwt_token_ttl='31556952', got '%s'", ttl)
					}

					return nil
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							jwt_token_ttl = "31556952"
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("jwt_token_ttl"),
						knownvalue.StringExact("31556952"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						return assertCephMgrModuleConfigValue(t.Context(), "dashboard", "jwt_token_ttl", "31556952")
					},
				),
			},
		},
	})
}

func checkCephMgrModuleConfigExists(t *testing.T, module, option string) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		value, err := getCephMgrModuleConfigValue(t.Context(), module, option)
		if err != nil {
			return fmt.Errorf("config mgr/%s/%s does not exist: %w", module, option, err)
		}
		if value == "" {
			return fmt.Errorf("config mgr/%s/%s has empty value", module, option)
		}
		t.Logf("Verified config mgr/%s/%s exists with value: %s", module, option, value)
		return nil
	}
}

func TestAccCephMgrModuleConfigResource_removeProperty(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	var defaultStandbyErrorStatusCode string
	var defaultServerPort string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephMgrModuleConfigDestroy(t),
		PreCheck: func() {
			ctx := t.Context()

			var err error
			defaultStandbyErrorStatusCode, err = getCephMgrModuleConfigValue(ctx, "dashboard", "standby_error_status_code")
			if err != nil {
				t.Fatalf("Failed to get default standby_error_status_code: %v", err)
			}
			t.Logf("Default standby_error_status_code: %s", defaultStandbyErrorStatusCode)

			defaultServerPort, err = getCephMgrModuleConfigValue(ctx, "dashboard", "server_port")
			if err != nil {
				t.Fatalf("Failed to get default server_port: %v", err)
			}
			t.Logf("Default server_port: %s", defaultServerPort)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							standby_error_status_code = "503"
							server_port               = "8080"
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("standby_error_status_code"),
						knownvalue.StringExact("503"),
					),
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("server_port"),
						knownvalue.StringExact("8080"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						if err := assertCephMgrModuleConfigValue(t.Context(), "dashboard", "standby_error_status_code", "503"); err != nil {
							return err
						}
						return assertCephMgrModuleConfigValue(t.Context(), "dashboard", "server_port", "8080")
					},
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							standby_error_status_code = "503"
						}
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_mgr_module_config.test",
						tfjsonpath.New("configs"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"standby_error_status_code": knownvalue.StringExact("503"),
						}),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						ctx := t.Context()

						if err := assertCephMgrModuleConfigValue(ctx, "dashboard", "standby_error_status_code", "503"); err != nil {
							return err
						}

						if err := assertCephMgrModuleConfigValue(ctx, "dashboard", "server_port", defaultServerPort); err != nil {
							return fmt.Errorf("server_port should have reverted to default %q: %w", defaultServerPort, err)
						}

						_, err := cephTestClusterCLI.ConfigGetFromDump(ctx, "mgr", "mgr/dashboard/server_port")
						if err == nil {
							return fmt.Errorf("server_port should not be explicitly set in config dump after removal")
						}

						return nil
					},
				),
			},
		},
	})
}

func TestAccCephMgrModuleConfigResource_OutOfBandDeletion(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephMgrModuleConfigDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							standby_behaviour = "error"
						}
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephMgrModuleConfigExists(t, "dashboard", "standby_behaviour"),
					resource.TestCheckResourceAttr("ceph_mgr_module_config.test", "module_name", "dashboard"),
					resource.TestCheckResourceAttr("ceph_mgr_module_config.test", "configs.standby_behaviour", "error"),
				),
			},
			{
				PreConfig: func() {
					err := removeCephMgrModuleConfigValue(t.Context(), "dashboard", "standby_behaviour")
					if err != nil {
						t.Fatalf("Failed to delete config out of band: %v", err)
					}
					t.Logf("Deleted config mgr/dashboard/standby_behaviour out of band")
				},
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
						configs = {
							standby_behaviour = "error"
						}
					}
				`,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_mgr_module_config.test", plancheck.ResourceActionNoop),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephMgrModuleConfigExists(t, "dashboard", "standby_behaviour"),
					resource.TestCheckResourceAttr("ceph_mgr_module_config.test", "module_name", "dashboard"),
					resource.TestCheckResourceAttr("ceph_mgr_module_config.test", "configs.standby_behaviour", "error"),
				),
			},
		},
	})
}
