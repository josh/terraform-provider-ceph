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
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.CrushRuleCreateReplicated(ctx, ruleName, "default", "host"); err != nil {
				t.Fatalf("Failed to create replicated crush rule: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()

				_ = cephTestClusterCLI.CrushRuleRemove(cleanupCtx, ruleName)
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
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.CrushRuleCreateSimple(ctx, ruleName, "default", "host"); err != nil {
				t.Fatalf("Failed to create simple crush rule: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()

				_ = cephTestClusterCLI.CrushRuleRemove(cleanupCtx, ruleName)
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
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()

				_ = cephTestClusterCLI.CrushRuleRemove(cleanupCtx, ruleName)
				_ = cephTestClusterCLI.ErasureCodeProfileRemove(cleanupCtx, profileName)
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
				},
			},
		},
	})
}
