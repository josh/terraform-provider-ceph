package main

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephDashboardSettingResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	settingConfig := func(value string) string {
		return testAccProviderConfigBlock + fmt.Sprintf(`
			resource "ceph_dashboard_setting" "test" {
			  name  = "GRAFANA_API_URL"
			  value = %q
			}
		`, value)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDashboardSettingDestroy(t, "GRAFANA_API_URL", ""),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          settingConfig("https://grafana.example.com:3000"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_dashboard_setting.test",
						tfjsonpath.New("value"),
						knownvalue.StringExact("https://grafana.example.com:3000"),
					),
					statecheck.ExpectKnownValue(
						"ceph_dashboard_setting.test",
						tfjsonpath.New("type"),
						knownvalue.StringExact("str"),
					),
					statecheck.ExpectKnownValue(
						"ceph_dashboard_setting.test",
						tfjsonpath.New("default"),
						knownvalue.StringExact(""),
					),
				},
				Check: checkDashboardSetting(t, "GRAFANA_API_URL", "https://grafana.example.com:3000"),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          settingConfig("https://grafana2.example.com:3000"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_dashboard_setting.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: checkDashboardSetting(t, "GRAFANA_API_URL", "https://grafana2.example.com:3000"),
			},
			{
				PreConfig: func() {
					if err := cephTestClusterCLI.DashboardSettingSet(t.Context(), "GRAFANA_API_URL", "https://drifted.example.com"); err != nil {
						t.Fatalf("Failed to set dashboard setting out of band: %v", err)
					}
				},
				ConfigVariables: testAccProviderConfig(),
				Config:          settingConfig("https://grafana2.example.com:3000"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_dashboard_setting.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: checkDashboardSetting(t, "GRAFANA_API_URL", "https://grafana2.example.com:3000"),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          settingConfig("https://grafana2.example.com:3000"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccCephDashboardSettingResource_Bool(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	boolConfig := testAccProviderConfigBlock + `
		resource "ceph_dashboard_setting" "test" {
		  name  = "ISCSI_API_SSL_VERIFICATION"
		  value = "false"
		}
	`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDashboardSettingDestroy(t, "ISCSI_API_SSL_VERIFICATION", "True"),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          boolConfig,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_dashboard_setting.test",
						tfjsonpath.New("value"),
						knownvalue.StringExact("false"),
					),
					statecheck.ExpectKnownValue(
						"ceph_dashboard_setting.test",
						tfjsonpath.New("type"),
						knownvalue.StringExact("bool"),
					),
					statecheck.ExpectKnownValue(
						"ceph_dashboard_setting.test",
						tfjsonpath.New("default"),
						knownvalue.StringExact("true"),
					),
				},
				Check: checkDashboardSetting(t, "ISCSI_API_SSL_VERIFICATION", "False"),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          boolConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccCephDashboardSettingResource_UnknownName(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_dashboard_setting" "test" {
					  name  = "NO_SUCH_SETTING"
					  value = "whatever"
					}
				`,
				ExpectError: regexp.MustCompile(`does not exist`),
			},
		},
	})
}

func TestAccCephDashboardSettingResource_Import(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			if err := cephTestClusterCLI.DashboardSettingSet(t.Context(), "GRAFANA_API_URL", "https://imported.example.com"); err != nil {
				t.Fatalf("Failed to set dashboard setting: %v", err)
			}
			t.Cleanup(func() {
				cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				if err := cephTestClusterCLI.DashboardSettingReset(cleanupCtx, "GRAFANA_API_URL"); err != nil {
					t.Errorf("Failed to reset dashboard setting: %v", err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_dashboard_setting" "test" {
					  name  = "GRAFANA_API_URL"
					  value = "https://imported.example.com"
					}
				`,
				ResourceName:  "ceph_dashboard_setting.test",
				ImportState:   true,
				ImportStateId: "GRAFANA_API_URL",
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["name"] != "GRAFANA_API_URL" {
						return fmt.Errorf("expected name GRAFANA_API_URL, got %q", attrs["name"])
					}
					if attrs["value"] != "https://imported.example.com" {
						return fmt.Errorf("expected imported value, got %q", attrs["value"])
					}
					if attrs["type"] != "str" {
						return fmt.Errorf("expected type str, got %q", attrs["type"])
					}
					return nil
				},
			},
		},
	})
}

func checkDashboardSetting(t *testing.T, name, want string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		value, err := cephTestClusterCLI.DashboardSettingGet(t.Context(), name)
		if err != nil {
			return fmt.Errorf("failed to get dashboard setting: %w", err)
		}
		if value != want {
			return fmt.Errorf("dashboard setting %q: expected %q, got %q", name, want, value)
		}
		return nil
	}
}

func testAccCheckDashboardSettingDestroy(t *testing.T, name, defaultValue string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		value, err := cephTestClusterCLI.DashboardSettingGet(t.Context(), name)
		if err != nil {
			return fmt.Errorf("failed to get dashboard setting: %w", err)
		}
		if value != defaultValue {
			return fmt.Errorf("dashboard setting %q: expected default %q after destroy, got %q", name, defaultValue, value)
		}
		return nil
	}
}
