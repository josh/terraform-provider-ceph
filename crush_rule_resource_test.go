package main

import (
	"fmt"
	"regexp"
	"slices"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
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
				),
			},
			{
				ConfigVariables:                      testAccProviderConfig(),
				ResourceName:                         "ceph_crush_rule.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        ruleName,
				ImportStateVerifyIdentifierAttribute: "name",
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
				),
			},
			{
				ConfigVariables:                      testAccProviderConfig(),
				ResourceName:                         "ceph_crush_rule.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        ruleName,
				ImportStateVerifyIdentifierAttribute: "name",
				// The erasure profile is not recoverable from the rule dump.
				ImportStateVerifyIgnore: []string{"profile"},
			},
		},
	})
}

func TestAccCephCrushRuleResource_erasureWithOSDsPerFailureDomain(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	ruleName := fmt.Sprintf("test-erasure-msr-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	profileName := fmt.Sprintf("test-profile-msr-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

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
					  name                          = %q
					  k                             = 4
					  m                             = 2
					  crush_failure_domain          = "osd"
					  crush_num_failure_domains     = 3
					  crush_osds_per_failure_domain = 2
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
						tfjsonpath.New("profile"),
						knownvalue.StringExact(profileName),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephCrushRuleExists(t, ruleName),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "name", ruleName),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "pool_type", "erasure"),
				),
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
		rule, err := cephTestClusterCLI.CrushRuleDump(t.Context(), ruleName)
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

func testAccCleanupCrushRule(t *testing.T, ruleName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if err := cephTestClusterCLI.CrushRuleRemove(t.Context(), ruleName); err != nil {
			return fmt.Errorf("failed to cleanup crush rule %s: %w", ruleName, err)
		}
		return nil
	}
}

func TestAccCephCrushRuleResource_OutOfBandDeletion(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	ruleName := fmt.Sprintf("test-crush-rule-oob-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

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
					}
				`, ruleName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephCrushRuleExists(t, ruleName),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "name", ruleName),
				),
			},
			{
				PreConfig: func() {
					err := cephTestClusterCLI.CrushRuleRemove(t.Context(), ruleName)
					if err != nil {
						t.Fatalf("Failed to delete CRUSH rule out of band: %v", err)
					}
					t.Logf("Deleted CRUSH rule %s out of band", ruleName)
				},
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_crush_rule" "test" {
					  name           = %q
					  pool_type      = "replicated"
					  failure_domain = "osd"
					}
				`, ruleName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_crush_rule.test", plancheck.ResourceActionCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephCrushRuleExists(t, ruleName),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "name", ruleName),
				),
			},
		},
	})
}

func TestAccCephCrushRuleResource_OutOfBandDeletionDestroy(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	ruleName := fmt.Sprintf("test-crush-rule-oob-destroy-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

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
					}
				`, ruleName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephCrushRuleExists(t, ruleName),
				),
			},
			{
				PreConfig: func() {
					err := cephTestClusterCLI.CrushRuleRemove(t.Context(), ruleName)
					if err != nil {
						t.Fatalf("Failed to delete CRUSH rule out of band: %v", err)
					}
					t.Logf("Deleted CRUSH rule %s out of band", ruleName)
				},
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccProviderConfigBlock,
			},
		},
	})
}

func TestAccCephCrushRuleResource_OutOfBandModification(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	ruleName := fmt.Sprintf("test-crush-rule-oob-mod-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	config := testAccProviderConfigBlock + fmt.Sprintf(`
		resource "ceph_crush_rule" "test" {
		  name           = %q
		  pool_type      = "replicated"
		  failure_domain = "host"
		}
	`, ruleName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephCrushRuleDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("failure_domain"),
						knownvalue.StringExact("host"),
					),
				},
			},
			// Rebuild the rule out of band with a different failure domain under
			// the same name; Read must reconcile the drift and plan a replacement.
			{
				PreConfig: func() {
					if err := cephTestClusterCLI.CrushRuleRemove(t.Context(), ruleName); err != nil {
						t.Fatalf("Failed to remove CRUSH rule out of band: %v", err)
					}
					if err := cephTestClusterCLI.CrushRuleCreateReplicated(t.Context(), ruleName, "default", "osd"); err != nil {
						t.Fatalf("Failed to recreate CRUSH rule out of band: %v", err)
					}
					t.Logf("Recreated CRUSH rule %s out of band with failure_domain=osd", ruleName)
				},
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_crush_rule.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_crush_rule.test",
						tfjsonpath.New("failure_domain"),
						knownvalue.StringExact("host"),
					),
				},
			},
		},
	})
}

func TestAccCephCrushRuleResource_ReplacementOnPoolTypeChange(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	ruleName := fmt.Sprintf("test-replacement-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
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
					resource "ceph_crush_rule" "test" {
						name           = %q
						pool_type      = "replicated"
						failure_domain = "host"
					}
				`, ruleName),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephCrushRuleExists(t, ruleName),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "name", ruleName),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "pool_type", "replicated"),
				),
			},
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
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_crush_rule.test", plancheck.ResourceActionDestroyBeforeCreate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephCrushRuleExists(t, ruleName),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "name", ruleName),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "pool_type", "erasure"),
				),
			},
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
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephCrushRuleExists(t, ruleName),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "failure_domain", "osd"),
				),
			},
		},
	})
}

func TestAccCephCrushRuleResource_FailureDomainMismatchWithProfile(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	profileName := acctest.RandomWithPrefix("test-ec-profile")
	ruleName := acctest.RandomWithPrefix("test-crush-rule")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
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
						crush_failure_domain = "host"
					}

					resource "ceph_crush_rule" "test" {
						name           = %q
						pool_type      = "erasure"
						failure_domain = "osd"
						profile        = ceph_erasure_code_profile.test.name
					}
				`, profileName, ruleName),
				ExpectError: regexp.MustCompile(`Failure Domain Mismatch`),
			},
		},
	})
}

func TestAccCephCrushRuleResource_erasureProfileDeviceClass(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	profileName := acctest.RandomWithPrefix("test-ec-profile-dc")
	ruleName := acctest.RandomWithPrefix("test-crush-rule-dc")
	mismatchRuleName := acctest.RandomWithPrefix("test-crush-rule-dc-miss")

	profileConfig := fmt.Sprintf(`
		resource "ceph_erasure_code_profile" "test" {
			name                 = %q
			k                    = 2
			m                    = 1
			crush_failure_domain = "osd"
			crush_device_class   = "hdd"
		}
	`, profileName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephCrushRuleDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)
		},
		Steps: []resource.TestStep{
			// Omitting device_class must fail with a clear mismatch error,
			// since the mon builds the erasure rule from the profile alone.
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + profileConfig + fmt.Sprintf(`
					resource "ceph_crush_rule" "test" {
						name           = %q
						pool_type      = "erasure"
						failure_domain = "osd"
						profile        = ceph_erasure_code_profile.test.name
					}
				`, mismatchRuleName),
				ExpectError: regexp.MustCompile(`Device Class Mismatch`),
			},
			// A matching device_class applies cleanly.
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + profileConfig + fmt.Sprintf(`
					resource "ceph_crush_rule" "test" {
						name           = %q
						pool_type      = "erasure"
						failure_domain = "osd"
						device_class   = "hdd"
						profile        = ceph_erasure_code_profile.test.name
					}
				`, ruleName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "device_class", "hdd"),
					resource.TestCheckResourceAttr("ceph_crush_rule.test", "root", "default"),
				),
			},
		},
	})
}

func TestAccCephCrushRuleResource_importOutOfBand(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	ruleName := fmt.Sprintf("test-import-oob-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	config := testAccProviderConfigBlock + fmt.Sprintf(`
		resource "ceph_crush_rule" "test" {
		  name           = %q
		  pool_type      = "replicated"
		  failure_domain = "host"
		}
	`, ruleName)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephCrushRuleDestroy(t),
		PreCheck: func() {
			testAccPreCheckCephHealth(t)

			if err := cephTestClusterCLI.CrushRuleCreateReplicated(t.Context(), ruleName, "default", "host"); err != nil {
				t.Fatalf("Failed to create crush rule out of band: %v", err)
			}
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables:    testAccProviderConfig(),
				Config:             config,
				ResourceName:       "ceph_crush_rule.test",
				ImportState:        true,
				ImportStateId:      ruleName,
				ImportStatePersist: true,
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["failure_domain"] != "host" {
						return fmt.Errorf("expected failure_domain host, got %q", attrs["failure_domain"])
					}
					if attrs["root"] != "default" {
						return fmt.Errorf("expected root default, got %q", attrs["root"])
					}
					return nil
				},
			},
			// The plan after import must be empty; before placement
			// attributes were recovered it forced a replacement.
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				PlanOnly:        true,
			},
		},
	})
}
