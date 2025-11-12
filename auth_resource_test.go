package main

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func testAccProviderConfig() config.Variables {
	return config.Variables{
		"endpoint": config.StringVariable(testDashboardURL),
		"username": config.StringVariable("admin"),
		"password": config.StringVariable("password"),
	}
}

const testAccProviderConfigBlock = `
variable "endpoint" {
  type = string
}

variable "username" {
  type = string
}

variable "password" {
  type = string
}

provider "ceph" {
  endpoint = var.endpoint
  username = var.username
  password = var.password
}
`

func TestAccCephAuthResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testEntity := acctest.RandomWithPrefix("client.test-auth")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephAuthDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_auth" "foo" {
					  entity = %q
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=foo"
					  }
					}
				`, testEntity),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_auth.foo",
						tfjsonpath.New("entity"),
						knownvalue.StringExact(testEntity),
					),
					statecheck.ExpectKnownValue(
						"ceph_auth.foo",
						tfjsonpath.New("caps"),
						knownvalue.ObjectExact(map[string]knownvalue.Check{
							"mon": knownvalue.StringExact("allow r"),
							"osd": knownvalue.StringExact("allow rw pool=foo"),
						}),
					),
					statecheck.ExpectKnownValue(
						"ceph_auth.foo",
						tfjsonpath.New("key"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_auth.foo",
						tfjsonpath.New("keyring"),
						knownvalue.NotNull(),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephAuthExists(t, testEntity),
					checkCephAuthHasCaps(t, testEntity, map[string]string{
						"mon": "allow r",
						"osd": "allow rw pool=foo",
					}),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_auth" "foo" {
					  entity = %q
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=foo"
					  }
					}
				`, testEntity),
				ResourceName:                         "ceph_auth.foo",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "entity",
				ImportStateId:                        testEntity,
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_auth" "foo" {
					  entity = %q
					  caps = {
					    mon = "allow rw"
					    mgr = "allow r"
					    osd = "allow rw pool=bar"
					    mds = "allow rw"
					  }
					}
				`, testEntity),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_auth.foo",
						tfjsonpath.New("entity"),
						knownvalue.StringExact(testEntity),
					),
					statecheck.ExpectKnownValue(
						"ceph_auth.foo",
						tfjsonpath.New("caps"),
						knownvalue.ObjectExact(map[string]knownvalue.Check{
							"mon": knownvalue.StringExact("allow rw"),
							"mgr": knownvalue.StringExact("allow r"),
							"mds": knownvalue.StringExact("allow rw"),
							"osd": knownvalue.StringExact("allow rw pool=bar"),
						}),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephAuthExists(t, testEntity),
					checkCephAuthHasCaps(t, testEntity, map[string]string{
						"mon": "allow rw",
						"mgr": "allow r",
						"osd": "allow rw pool=bar",
						"mds": "allow rw",
					}),
				),
			},
		},
	})
}

func TestAccCephAuthResource_invalidCapType(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testEntity := acctest.RandomWithPrefix("client.test-invalid")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_auth" "invalid" {
					  entity = %q
					  caps = {
					    foo = "allow r"
					  }
					}
				`, testEntity),
				ExpectError: regexp.MustCompile(`(?i)caps attribute contains unsupported capability type`),
			},
		},
	})
}

func TestAccCephAuthResource_invalidCapTypeOnUpdate(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testEntity := acctest.RandomWithPrefix("client.test-update")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephAuthDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_auth" "test_update" {
					  entity = %q
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=test"
					  }
					}
				`, testEntity),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_auth" "test_update" {
					  entity = %q
					  caps = {
					    invalid = "allow r"
					  }
					}
				`, testEntity),
				ExpectError: regexp.MustCompile(`(?i)caps attribute contains unsupported capability type`),
			},
		},
	})
}

func TestAccCephAuthResourceImport(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testEntity := acctest.RandomWithPrefix("client.test-import")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephAuthDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_auth" "bar" {
					  entity = %q
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=bar"
					  }
					}
				`, testEntity),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_auth" "bar" {
					  entity = %q
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=bar"
					  }
					}
				`, testEntity),
				ResourceName:                         "ceph_auth.bar",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "entity",
				ImportStateId:                        testEntity,
			},
		},
	})
}

func TestAccCephAuthResourceImport_nonExistent(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testEntity := acctest.RandomWithPrefix("client.test-nonexist")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_auth" "nonexistent" {
					  entity = %q
					  caps = {
					    mon = "allow r"
					  }
					}
				`, testEntity),
				ResourceName:  "ceph_auth.nonexistent",
				ImportState:   true,
				ImportStateId: testEntity,
				ExpectError:   regexp.MustCompile(`(?i)unable to export user from ceph api`),
			},
		},
	})
}

func testAccCheckCephAuthDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "ceph_auth" {
			continue
		}

		entity := rs.Primary.Attributes["entity"]

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := cephTestClusterCLI.AuthGet(ctx, entity)
		if err == nil {
			return fmt.Errorf("ceph_auth resource %s still exists", entity)
		}
	}
	return nil
}

func checkCephAuthExists(t *testing.T, entity string) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		authInfo, err := cephTestClusterCLI.AuthGet(ctx, entity)
		if err != nil {
			return fmt.Errorf("auth entity %s does not exist: %w", entity, err)
		}

		t.Logf("Verified auth entity %s exists with caps: %v", entity, authInfo.Caps)
		return nil
	}
}

func checkCephAuthHasCaps(t *testing.T, entity string, expectedCaps map[string]string) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		authInfo, err := cephTestClusterCLI.AuthGet(ctx, entity)
		if err != nil {
			return fmt.Errorf("auth entity %s does not exist: %w", entity, err)
		}

		actualCaps := authInfo.Caps
		for capType, expectedCap := range expectedCaps {
			if actualCap, ok := actualCaps[capType]; !ok {
				return fmt.Errorf("expected cap %s not found for entity %s", capType, entity)
			} else if actualCap != expectedCap {
				return fmt.Errorf("cap %s mismatch for entity %s: expected %q, got %q", capType, entity, expectedCap, actualCap)
			}
		}

		t.Logf("Verified auth entity %s has correct caps: %v", entity, actualCaps)
		return nil
	}
}

func checkCephAuthHasKey(t *testing.T, entity string, expectedKey string) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		authInfo, err := cephTestClusterCLI.AuthGet(ctx, entity)
		if err != nil {
			return fmt.Errorf("auth entity %s does not exist: %w", entity, err)
		}

		actualKey := authInfo.Key
		if actualKey != expectedKey {
			return fmt.Errorf("key mismatch for entity %s: expected %q, got %q", entity, expectedKey, actualKey)
		}

		t.Logf("Verified auth entity %s has expected key", entity)
		return nil
	}
}

func TestAccCephAuthResource_staticKey(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testEntity := acctest.RandomWithPrefix("client.test-static-key")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephAuthDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_auth" "foo" {
					  entity = %q
					  key    = "AQBvaBVesCMcKRAAoKhLdz8Qh/qPNqF9UGKYfg=="
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=test"
					  }
					}
				`, testEntity),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_auth.foo",
						tfjsonpath.New("entity"),
						knownvalue.StringExact(testEntity),
					),
					statecheck.ExpectKnownValue(
						"ceph_auth.foo",
						tfjsonpath.New("key"),
						knownvalue.StringExact("AQBvaBVesCMcKRAAoKhLdz8Qh/qPNqF9UGKYfg=="),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephAuthExists(t, testEntity),
					checkCephAuthHasKey(t, testEntity, "AQBvaBVesCMcKRAAoKhLdz8Qh/qPNqF9UGKYfg=="),
					checkCephAuthHasCaps(t, testEntity, map[string]string{
						"mon": "allow r",
						"osd": "allow rw pool=test",
					}),
				),
			},
		},
	})
}
func TestAccCephAuthResource_capsDriftDetection(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testEntity := acctest.RandomWithPrefix("client.test-caps-drift")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephAuthDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_auth" "test" {
					  entity = %q
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=original"
					  }
					}
				`, testEntity),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephAuthExists(t, testEntity),
					checkCephAuthHasCaps(t, testEntity, map[string]string{
						"mon": "allow r",
						"osd": "allow rw pool=original",
					}),
				),
			},
			{
				PreConfig: func() {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()

					err := cephTestClusterCLI.AuthSetCaps(ctx, testEntity, map[string]string{
						"mon": "allow rw",
						"osd": "allow rw pool=modified",
						"mgr": "allow r",
					})
					if err != nil {
						t.Fatalf("Failed to modify caps out of band: %v", err)
					}
					t.Logf("Modified caps for %s out of band", testEntity)
				},
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_auth" "test" {
					  entity = %q
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=original"
					  }
					}
				`, testEntity),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephAuthExists(t, testEntity),
					checkCephAuthHasCaps(t, testEntity, map[string]string{
						"mon": "allow r",
						"osd": "allow rw pool=original",
					}),
				),
			},
		},
	})
}
