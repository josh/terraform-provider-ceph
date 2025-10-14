package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/config"
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
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"ceph": providerserver.NewProtocol6WithError(providerFunc()),
		},
		CheckDestroy: testAccCheckCephAuthDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_auth" "foo" {
					  entity = "client.foo"
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=foo"
					  }
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_auth.foo",
						tfjsonpath.New("entity"),
						knownvalue.StringExact("client.foo"),
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
					checkCephAuthExists(t, "client.foo"),
					checkCephAuthHasCaps(t, "client.foo", map[string]string{
						"mon": "allow r",
						"osd": "allow rw pool=foo",
					}),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_auth" "foo" {
					  entity = "client.foo"
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=foo"
					  }
					}
				`,
				ResourceName:                         "ceph_auth.foo",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "entity",
				ImportStateId:                        "client.foo",
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_auth" "foo" {
					  entity = "client.foo"
					  caps = {
					    mon = "allow rw"
					    mgr = "allow r"
					    osd = "allow rw pool=bar"
					    mds = "allow rw"
					  }
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_auth.foo",
						tfjsonpath.New("entity"),
						knownvalue.StringExact("client.foo"),
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
					checkCephAuthExists(t, "client.foo"),
					checkCephAuthHasCaps(t, "client.foo", map[string]string{
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
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"ceph": providerserver.NewProtocol6WithError(providerFunc()),
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_auth" "invalid" {
					  entity = "client.invalid"
					  caps = {
					    foo = "allow r"
					  }
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)caps attribute contains unsupported capability type`),
			},
		},
	})
}

func TestAccCephAuthResource_invalidCapTypeOnUpdate(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"ceph": providerserver.NewProtocol6WithError(providerFunc()),
		},
		CheckDestroy: testAccCheckCephAuthDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_auth" "test_update" {
					  entity = "client.test_update"
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=test"
					  }
					}
				`,
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_auth" "test_update" {
					  entity = "client.test_update"
					  caps = {
					    invalid = "allow r"
					  }
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)caps attribute contains unsupported capability type`),
			},
		},
	})
}

func TestAccCephAuthResourceImport(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"ceph": providerserver.NewProtocol6WithError(providerFunc()),
		},
		CheckDestroy: testAccCheckCephAuthDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_auth" "bar" {
					  entity = "client.bar"
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=bar"
					  }
					}
				`,
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_auth" "bar" {
					  entity = "client.bar"
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=bar"
					  }
					}
				`,
				ResourceName:                         "ceph_auth.bar",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "entity",
				ImportStateId:                        "client.bar",
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

		cmd := exec.CommandContext(ctx, "ceph", "--conf", testConfPath, "auth", "get", entity, "--format", "json")
		output, err := cmd.Output()
		if err == nil {
			return fmt.Errorf("ceph_auth resource %s still exists (output: %s)", entity, string(output))
		}
	}
	return nil
}

func checkCephAuthExists(t *testing.T, entity string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "ceph", "--conf", testConfPath, "auth", "get", entity, "--format", "json")
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("auth entity %s does not exist: %w", entity, err)
		}

		var authData []struct {
			Entity string            `json:"entity"`
			Key    string            `json:"key"`
			Caps   map[string]string `json:"caps"`
		}
		if err := json.Unmarshal(output, &authData); err != nil {
			return fmt.Errorf("failed to parse auth output: %w", err)
		}

		if len(authData) == 0 {
			return fmt.Errorf("auth entity %s not found in output", entity)
		}

		t.Logf("Verified auth entity %s exists with caps: %v", entity, authData[0].Caps)
		return nil
	}
}

func checkCephAuthHasCaps(t *testing.T, entity string, expectedCaps map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "ceph", "--conf", testConfPath, "auth", "get", entity, "--format", "json")
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("auth entity %s does not exist: %w", entity, err)
		}

		var authData []struct {
			Entity string            `json:"entity"`
			Key    string            `json:"key"`
			Caps   map[string]string `json:"caps"`
		}
		if err := json.Unmarshal(output, &authData); err != nil {
			return fmt.Errorf("failed to parse auth output: %w", err)
		}

		if len(authData) == 0 {
			return fmt.Errorf("auth entity %s not found in output", entity)
		}

		actualCaps := authData[0].Caps
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
	return func(s *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "ceph", "--conf", testConfPath, "auth", "get", entity, "--format", "json")
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("auth entity %s does not exist: %w", entity, err)
		}

		var authData []struct {
			Entity string            `json:"entity"`
			Key    string            `json:"key"`
			Caps   map[string]string `json:"caps"`
		}
		if err := json.Unmarshal(output, &authData); err != nil {
			return fmt.Errorf("failed to parse auth output: %w", err)
		}

		if len(authData) == 0 {
			return fmt.Errorf("auth entity %s not found in output", entity)
		}

		actualKey := authData[0].Key
		if actualKey != expectedKey {
			return fmt.Errorf("key mismatch for entity %s: expected %q, got %q", entity, expectedKey, actualKey)
		}

		t.Logf("Verified auth entity %s has expected key", entity)
		return nil
	}
}

func TestAccCephAuthResource_staticKey(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"ceph": providerserver.NewProtocol6WithError(providerFunc()),
		},
		CheckDestroy: testAccCheckCephAuthDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_auth" "foo" {
					  entity = "client.foo"
					  key    = "AQBvaBVesCMcKRAAoKhLdz8Qh/qPNqF9UGKYfg=="
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=test"
					  }
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_auth.foo",
						tfjsonpath.New("entity"),
						knownvalue.StringExact("client.foo"),
					),
					statecheck.ExpectKnownValue(
						"ceph_auth.foo",
						tfjsonpath.New("key"),
						knownvalue.StringExact("AQBvaBVesCMcKRAAoKhLdz8Qh/qPNqF9UGKYfg=="),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephAuthExists(t, "client.foo"),
					checkCephAuthHasKey(t, "client.foo", "AQBvaBVesCMcKRAAoKhLdz8Qh/qPNqF9UGKYfg=="),
					checkCephAuthHasCaps(t, "client.foo", map[string]string{
						"mon": "allow r",
						"osd": "allow rw pool=test",
					}),
				),
			},
		},
	})
}
