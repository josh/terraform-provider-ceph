package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephMgrModuleConfigResource(t *testing.T) {
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
