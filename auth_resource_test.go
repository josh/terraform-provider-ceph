package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephAuthResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"ceph": providerserver.NewProtocol6WithError(providerFunc()),
		},
		CheckDestroy: testAccCheckCephAuthDestroy,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					provider "ceph" {
					  endpoint = %q
					  username = "admin"
					  password = "password"
					}

					resource "ceph_auth" "test" {
					  entity = "client.foo"
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=foo"
					  }
					}
				`, testDashboardURL),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_auth.test",
						tfjsonpath.New("entity"),
						knownvalue.StringExact("client.foo"),
					),
					statecheck.ExpectKnownValue(
						"ceph_auth.test",
						tfjsonpath.New("caps"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"mon": knownvalue.StringExact("allow r"),
							"osd": knownvalue.StringExact("allow rw pool=foo"),
						}),
					),
					statecheck.ExpectKnownValue(
						"ceph_auth.test",
						tfjsonpath.New("key"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_auth.test",
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
				Config: fmt.Sprintf(`
					provider "ceph" {
					  endpoint = %q
					  username = "admin"
					  password = "password"
					}

					resource "ceph_auth" "test" {
					  entity = "client.foo"
					  caps = {
					    mon = "allow rw"
					    osd = "allow rw pool=bar"
					    mds = "allow rw"
					  }
					}
				`, testDashboardURL),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_auth.test",
						tfjsonpath.New("entity"),
						knownvalue.StringExact("client.foo"),
					),
					statecheck.ExpectKnownValue(
						"ceph_auth.test",
						tfjsonpath.New("caps"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"mon": knownvalue.StringExact("allow rw"),
							"osd": knownvalue.StringExact("allow rw pool=bar"),
							"mds": knownvalue.StringExact("allow rw"),
						}),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephAuthExists(t, "client.foo"),
					checkCephAuthHasCaps(t, "client.foo", map[string]string{
						"mon": "allow rw",
						"osd": "allow rw pool=bar",
						"mds": "allow rw",
					}),
				),
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

