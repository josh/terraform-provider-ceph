package main

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// Opening the ephemeral resource on an entity that already exists must fail:
// adopting it would hand out the long-lived key and then Close would delete
// an entity the resource never created.
func TestAccCephAuthEphemeralResource_existingEntity(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testEntity := acctest.RandomWithPrefix("client.test-ephemeral-pre")
	caps := map[string]string{"mon": "allow r"}

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesWithEcho,
		PreCheck: func() {
			if err := cephTestClusterCLI.AuthGetOrCreate(t.Context(), testEntity, caps); err != nil {
				t.Fatalf("Failed to create auth entity: %v", err)
			}
			testCleanup(t, func(ctx context.Context) {
				if err := cephTestClusterCLI.AuthDel(ctx, testEntity); err != nil {
					t.Logf("Warning: failed to cleanup auth entity %s: %v", testEntity, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					ephemeral "ceph_auth_ephemeral" "test" {
					  entity = %q
					  caps = {
					    mon = "allow r"
					  }
					}

					provider "echo" {
					  data = ephemeral.ceph_auth_ephemeral.test
					}

					resource "echo" "test" {}
				`, testEntity),
				ExpectError: regexp.MustCompile(`(?i)already exists`),
			},
		},
	})

	if _, err := cephTestClusterCLI.AuthGet(t.Context(), testEntity); err != nil {
		t.Fatalf("Pre-existing entity %s was deleted by the ephemeral resource: %v", testEntity, err)
	}
}

func TestAccCephAuthEphemeralResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testEntity := acctest.RandomWithPrefix("client.test-ephemeral")

	resource.Test(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesWithEcho,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					ephemeral "ceph_auth_ephemeral" "test" {
					  entity = %q
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=ephemeral"
					  }
					}

					provider "echo" {
					  data = ephemeral.ceph_auth_ephemeral.test
					}

					resource "echo" "test" {}
				`, testEntity),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"echo.test",
						tfjsonpath.New("data").AtMapKey("entity"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"echo.test",
						tfjsonpath.New("data").AtMapKey("caps"),
						knownvalue.ObjectExact(map[string]knownvalue.Check{
							"mon": knownvalue.StringExact("allow r"),
							"osd": knownvalue.StringExact("allow rw pool=ephemeral"),
						}),
					),
					statecheck.ExpectKnownValue(
						"echo.test",
						tfjsonpath.New("data").AtMapKey("key"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"echo.test",
						tfjsonpath.New("data").AtMapKey("keyring"),
						knownvalue.NotNull(),
					),
				},
			},
		},
	})
}
