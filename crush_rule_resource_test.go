package main

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephCrushRuleResource_replicated(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	ruleName := fmt.Sprintf("test-replicated-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephCrushRuleDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_crush_rule" "test" {
					  name           = %q
					  pool_type      = "replicated"
					  failure_domain = "host"
					}
				`, ruleName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(ruleName),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("pool_type"),
						knownvalue.StringExact("replicated"),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("failure_domain"),
						knownvalue.StringExact("host"),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("rule_id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("type"),
						knownvalue.Int64Exact(1),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(0).AtMapKey("op"),
						knownvalue.StringExact("take"),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(0).AtMapKey("item"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(1).AtMapKey("op"),
						knownvalue.StringExact("chooseleaf_firstn"),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(1).AtMapKey("type"),
						knownvalue.StringExact("host"),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(2).AtMapKey("op"),
						knownvalue.StringExact("emit"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephCrushRuleExists(t, ruleName),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "name", ruleName),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "pool_type", "replicated"),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "failure_domain", "host"),
					resource.TestCheckResourceAttrSet("ceph_crush_rule.test", "rule_id"),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "type", "1"),
				),
			},
			{
				ConfigVariables:                      testAccProviderConfig(),
				ResourceName:                         "ceph_crush_rule.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        ruleName,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateVerifyIgnore:              []string{"pool_type", "failure_domain", "device_class", "profile", "root"},
			},
		},
	})
}

func TestAccCephCrushRuleResource_erasure(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	ruleName := fmt.Sprintf("test-erasure-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	profileName := fmt.Sprintf("test-profile-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephCrushRuleDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_erasure_code_profile" "test" {
					  name                 = %q
					  k                    = 2
					  m                    = 1
					  crush_failure_domain = "osd"
					}

					resource "ceph_crush_rule" "test" {
					  name           = %q
					  pool_type      = "erasure"
					  failure_domain = "osd"
					  profile        = ceph_erasure_code_profile.test.name
					}
				`, profileName, ruleName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(ruleName),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("pool_type"),
						knownvalue.StringExact("erasure"),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("failure_domain"),
						knownvalue.StringExact("osd"),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("profile"),
						knownvalue.StringExact(profileName),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("rule_id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("type"),
						knownvalue.Int64Exact(3),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(0).AtMapKey("op"),
						knownvalue.StringExact("set_chooseleaf_tries"),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(0).AtMapKey("num"),
						knownvalue.Int64Exact(5),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(1).AtMapKey("op"),
						knownvalue.StringExact("set_choose_tries"),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(1).AtMapKey("num"),
						knownvalue.Int64Exact(100),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(2).AtMapKey("op"),
						knownvalue.StringExact("take"),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(2).AtMapKey("item"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(3).AtMapKey("op"),
						knownvalue.StringExact("choose_indep"),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(3).AtMapKey("type"),
						knownvalue.StringExact("osd"),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(4).AtMapKey("op"),
						knownvalue.StringExact("emit"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephCrushRuleExists(t, ruleName),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "name", ruleName),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "pool_type", "erasure"),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "type", "3"),
				),
			},
			{
				ConfigVariables:                      testAccProviderConfig(),
				ResourceName:                         "ceph_crush_rule.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        ruleName,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateVerifyIgnore:              []string{"pool_type", "failure_domain", "device_class", "profile", "root"},
			},
		},
	})
}

func TestAccCephCrushRuleResource_withDeviceClass(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	ruleName := fmt.Sprintf("test-device-class-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephCrushRuleDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_crush_rule" "test" {
					  name           = %q
					  pool_type      = "replicated"
					  failure_domain = "osd"
					  device_class   = "hdd"
					}
				`, ruleName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(ruleName),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("device_class"),
						knownvalue.StringExact("hdd"),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(0).AtMapKey("op"),
						knownvalue.StringExact("take"),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(0).AtMapKey("item"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(1).AtMapKey("op"),
						knownvalue.StringExact("choose_firstn"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephCrushRuleExists(t, ruleName),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "device_class", "hdd"),
				),
			},
		},
	})
}

func TestAccCephCrushRuleResource_InvalidPoolType(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_crush_rule" "test" {
					  name           = "test-invalid-type"
					  pool_type      = "invalid"
					  failure_domain = "host"
					}
				`,
				ExpectError: regexp.MustCompile(`Attribute pool_type value must be one of`),
			},
		},
	})
}

func checkCephCrushRuleExists(t *testing.T, ruleName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()

		rule, err := cephTestClusterCLI.CrushRuleDump(ctx, ruleName)
		if err != nil {
			return fmt.Errorf("failed to get CRUSH rule '%s': %w", ruleName, err)
		}

		if rule.RuleName != ruleName {
			return fmt.Errorf("CRUSH rule name mismatch: expected %q, got %q", ruleName, rule.RuleName)
		}

		return nil
	}
}

func testAccCheckCephCrushRuleDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_crush_rule" {
				continue
			}

			ruleName := rs.Primary.Attributes["name"]

			rules, err := cephTestClusterCLI.CrushRuleList(ctx)
			if err != nil {
				return fmt.Errorf("failed to list CRUSH rules: %w", err)
			}

			if slices.Contains(rules, ruleName) {
				return fmt.Errorf("CRUSH rule %q still exists in Ceph", ruleName)
			}
		}

		return nil
	}
}
