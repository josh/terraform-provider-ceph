package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephMgrModuleConfigDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_mgr_module_config" "dashboard" {
					  module_name = "dashboard"
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_mgr_module_config.dashboard",
						tfjsonpath.New("id"),
						knownvalue.StringExact("dashboard"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_mgr_module_config.dashboard",
						tfjsonpath.New("module_name"),
						knownvalue.StringExact("dashboard"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_mgr_module_config.dashboard",
						tfjsonpath.New("configs").AtMapKey("ssl"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_mgr_module_config.dashboard",
						tfjsonpath.New("configs").AtMapKey("server_port"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_mgr_module_config.dashboard",
						tfjsonpath.New("configs").AtMapKey("server_addr"),
						knownvalue.NotNull(),
					),
				},
			},
		},
	})
}

func TestAccCephMgrModuleConfigDataSource_largeIntegerValues(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
					defer cancel()

					err := setCephMgrModuleConfigValue(ctx, "dashboard", "jwt_token_ttl", "31556952")
					if err != nil {
						t.Fatalf("Failed to set config value out of band: %v", err)
					}

					t.Cleanup(func() {
						cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer cleanupCancel()

						if err := removeCephMgrModuleConfigValue(cleanupCtx, "dashboard", "jwt_token_ttl"); err != nil {
							t.Errorf("Failed to cleanup mgr/dashboard/jwt_token_ttl: %v", err)
						}
					})
				},
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_mgr_module_config" "test" {
						module_name = "dashboard"
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_mgr_module_config.test",
						tfjsonpath.New("configs").AtMapKey("jwt_token_ttl"),
						knownvalue.StringExact("31556952"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_mgr_module_config.test", "configs.jwt_token_ttl", "31556952"),
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Resources["data.ceph_mgr_module_config.test"]
						if !ok {
							return fmt.Errorf("data source not found")
						}

						ttl := rs.Primary.Attributes["configs.jwt_token_ttl"]
						if ttl != "31556952" {
							return fmt.Errorf("expected jwt_token_ttl='31556952', got '%s'", ttl)
						}
						return nil
					},
					func(s *terraform.State) error {
						ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
						defer cancel()
						return assertCephMgrModuleConfigValue(ctx, "dashboard", "jwt_token_ttl", "31556952")
					},
				),
			},
		},
	})
}

func getCephMgrModuleConfigValue(ctx context.Context, module, option string) (string, error) {
	configKey := fmt.Sprintf("mgr/%s/%s", module, option)
	return cephTestClusterCLI.ConfigGet(ctx, "mgr", configKey)
}

func setCephMgrModuleConfigValue(ctx context.Context, module, option, value string) error {
	configKey := fmt.Sprintf("mgr/%s/%s", module, option)
	return cephTestClusterCLI.ConfigSet(ctx, "mgr", configKey, value)
}

func removeCephMgrModuleConfigValue(ctx context.Context, module, option string) error {
	configKey := fmt.Sprintf("mgr/%s/%s", module, option)
	return cephTestClusterCLI.ConfigRemove(ctx, "mgr", configKey)
}

func assertCephMgrModuleConfigValue(ctx context.Context, module, option, expected string) error {
	actual, err := getCephMgrModuleConfigValue(ctx, module, option)
	if err != nil {
		return err
	}

	if actual != expected {
		return fmt.Errorf("config value mismatch for mgr/%s/%s: expected %q, got %q", module, option, expected, actual)
	}

	return nil
}
