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
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephCrushRuleDataSource_replicated(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	ruleName := fmt.Sprintf("test-replicated-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckCephHealth(t)

			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.CrushRuleCreateReplicated(ctx, ruleName, "default", "host"); err != nil {
				t.Fatalf("Failed to create replicated crush rule: %v", err)
			}

			testCleanup(t, func(ctx context.Context) {
				if err := cephTestClusterCLI.CrushRuleRemove(ctx, ruleName); err != nil {
					t.Errorf("Failed to cleanup crush rule %s: %v", ruleName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_crush_rule" "test" {
						name = %q
					}
				`, ruleName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(ruleName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("rule_id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("type"),
						knownvalue.Int64Exact(1),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(0).AtMapKey("op"),
						knownvalue.StringExact("take"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(0).AtMapKey("item"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(1).AtMapKey("op"),
						knownvalue.StringExact("chooseleaf_firstn"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(1).AtMapKey("type"),
						knownvalue.StringExact("host"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(2).AtMapKey("op"),
						knownvalue.StringExact("emit"),
					),
				},
			},
		},
	})
}

func TestAccCephCrushRuleDataSource_simple(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	ruleName := fmt.Sprintf("test-simple-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckCephHealth(t)

			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.CrushRuleCreateSimple(ctx, ruleName, "default", "host"); err != nil {
				t.Fatalf("Failed to create simple crush rule: %v", err)
			}

			testCleanup(t, func(ctx context.Context) {
				if err := cephTestClusterCLI.CrushRuleRemove(ctx, ruleName); err != nil {
					t.Errorf("Failed to cleanup crush rule %s: %v", ruleName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_crush_rule" "test" {
						name = %q
					}
				`, ruleName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(ruleName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("rule_id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("type"),
						knownvalue.Int64Exact(1),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(0).AtMapKey("op"),
						knownvalue.StringExact("take"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(0).AtMapKey("item"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(1).AtMapKey("op"),
						knownvalue.StringExact("chooseleaf_firstn"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(1).AtMapKey("type"),
						knownvalue.StringExact("host"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(2).AtMapKey("op"),
						knownvalue.StringExact("emit"),
					),
				},
			},
		},
	})
}

func TestAccCephCrushRuleDataSource_erasure(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	ruleName := fmt.Sprintf("test-erasure-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	profileName := fmt.Sprintf("test-profile-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			testAccPreCheckCephHealth(t)

			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.ErasureCodeProfileSet(ctx, profileName, map[string]string{
				"k":                    "2",
				"m":                    "1",
				"crush-failure-domain": "osd",
			}); err != nil {
				t.Fatalf("Failed to create erasure code profile: %v", err)
			}

			if err := cephTestClusterCLI.CrushRuleCreateErasure(ctx, ruleName, profileName); err != nil {
				t.Fatalf("Failed to create erasure crush rule: %v", err)
			}

			testCleanup(t, func(ctx context.Context) {
				if err := cephTestClusterCLI.CrushRuleRemove(ctx, ruleName); err != nil {
					t.Errorf("Failed to cleanup crush rule %s: %v", ruleName, err)
				}
				if err := cephTestClusterCLI.ErasureCodeProfileRemove(ctx, profileName); err != nil {
					t.Errorf("Failed to cleanup erasure code profile %s: %v", profileName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_crush_rule" "test" {
						name = %q
					}
				`, ruleName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(ruleName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("rule_id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("type"),
						knownvalue.Int64Exact(3),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(0).AtMapKey("op"),
						knownvalue.StringExact("set_chooseleaf_tries"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(0).AtMapKey("num"),
						knownvalue.Int64Exact(5),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(1).AtMapKey("op"),
						knownvalue.StringExact("set_choose_tries"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(1).AtMapKey("num"),
						knownvalue.Int64Exact(100),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(2).AtMapKey("op"),
						knownvalue.StringExact("take"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(2).AtMapKey("item"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(3).AtMapKey("op"),
						knownvalue.StringExact("choose_indep"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(3).AtMapKey("type"),
						knownvalue.StringExact("osd"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_crush_rule.test",
						tfjsonpath.New("steps").AtSliceIndex(4).AtMapKey("op"),
						knownvalue.StringExact("emit"),
					),
				},
			},
		},
	})
}
