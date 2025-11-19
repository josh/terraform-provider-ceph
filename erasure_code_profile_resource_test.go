package main

import (
	"context"
	"fmt"
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

func TestAccCephErasureCodeProfileResource_k2m1(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	profileName := fmt.Sprintf("test-profile-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephErasureCodeProfileDestroy(t),
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
				`, profileName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(profileName),
					),
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("k"),
						knownvalue.Int64Exact(2),
					),
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("m"),
						knownvalue.Int64Exact(1),
					),
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("crush_failure_domain"),
						knownvalue.StringExact("osd"),
					),
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("plugin"),
						knownvalue.NotNull(),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephErasureCodeProfileExists(t, profileName),
					resource.TestCheckResourceAttr("ceph_erasure_code_profile.test", "name", profileName),
					resource.TestCheckResourceAttr("ceph_erasure_code_profile.test", "k", "2"),
					resource.TestCheckResourceAttr("ceph_erasure_code_profile.test", "m", "1"),
					resource.TestCheckResourceAttr("ceph_erasure_code_profile.test", "crush_failure_domain", "osd"),
					resource.TestCheckResourceAttrSet("ceph_erasure_code_profile.test", "plugin"),
				),
			},
			{
				ConfigVariables:                      testAccProviderConfig(),
				ResourceName:                         "ceph_erasure_code_profile.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateId:                        profileName,
				ImportStateVerifyIdentifierAttribute: "name",
			},
		},
	})
}

func TestAccCephErasureCodeProfileResource_k3m2(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	profileName := fmt.Sprintf("test-profile-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephErasureCodeProfileDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_erasure_code_profile" "test" {
					  name                 = %q
					  k                    = 3
					  m                    = 2
					  crush_failure_domain = "host"
					}
				`, profileName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(profileName),
					),
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("k"),
						knownvalue.Int64Exact(3),
					),
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("m"),
						knownvalue.Int64Exact(2),
					),
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("crush_failure_domain"),
						knownvalue.StringExact("host"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephErasureCodeProfileExists(t, profileName),
					resource.TestCheckResourceAttr("ceph_erasure_code_profile.test", "k", "3"),
					resource.TestCheckResourceAttr("ceph_erasure_code_profile.test", "m", "2"),
					resource.TestCheckResourceAttr("ceph_erasure_code_profile.test", "crush_failure_domain", "host"),
				),
			},
		},
	})
}

func TestAccCephErasureCodeProfileResource_withOptionalParams(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	profileName := fmt.Sprintf("test-profile-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephErasureCodeProfileDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_erasure_code_profile" "test" {
					  name                 = %q
					  k                    = 2
					  m                    = 1
					  plugin               = "jerasure"
					  crush_failure_domain = "osd"
					  technique            = "reed_sol_van"
					  crush_device_class   = "hdd"
					}
				`, profileName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(profileName),
					),
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("plugin"),
						knownvalue.StringExact("jerasure"),
					),
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("technique"),
						knownvalue.StringExact("reed_sol_van"),
					),
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("crush_device_class"),
						knownvalue.StringExact("hdd"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephErasureCodeProfileExists(t, profileName),
					resource.TestCheckResourceAttr("ceph_erasure_code_profile.test", "plugin", "jerasure"),
					resource.TestCheckResourceAttr("ceph_erasure_code_profile.test", "technique", "reed_sol_van"),
					resource.TestCheckResourceAttr("ceph_erasure_code_profile.test", "crush_device_class", "hdd"),
				),
			},
		},
	})
}

func TestAccCephErasureCodeProfileResource_defaults(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	profileName := fmt.Sprintf("test-profile-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephErasureCodeProfileDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_erasure_code_profile" "test" {
					  name = %q
					}
				`, profileName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(profileName),
					),
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("k"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("m"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_erasure_code_profile.test",
						tfjsonpath.New("plugin"),
						knownvalue.NotNull(),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephErasureCodeProfileExists(t, profileName),
					resource.TestCheckResourceAttrSet("ceph_erasure_code_profile.test", "k"),
					resource.TestCheckResourceAttrSet("ceph_erasure_code_profile.test", "m"),
					resource.TestCheckResourceAttrSet("ceph_erasure_code_profile.test", "plugin"),
					resource.TestCheckResourceAttrSet("ceph_erasure_code_profile.test", "crush_failure_domain"),
				),
			},
		},
	})
}

func checkCephErasureCodeProfileExists(t *testing.T, profileName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()

		profile, err := cephTestClusterCLI.ErasureCodeProfileGet(ctx, profileName)
		if err != nil {
			return fmt.Errorf("failed to get erasure code profile '%s': %w", profileName, err)
		}

		if len(profile) == 0 {
			return fmt.Errorf("erasure code profile '%s' not found", profileName)
		}

		return nil
	}
}

func testAccCheckCephErasureCodeProfileDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_erasure_code_profile" {
				continue
			}

			profileName := rs.Primary.Attributes["name"]

			profiles, err := cephTestClusterCLI.ErasureCodeProfileList(ctx)
			if err != nil {
				return fmt.Errorf("failed to list erasure code profiles: %w", err)
			}

			if slices.Contains(profiles, profileName) {
				return fmt.Errorf("erasure code profile %q still exists in Ceph", profileName)
			}
		}

		return nil
	}
}
