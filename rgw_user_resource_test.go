package main

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephRGWUserResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-user-resource")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWUserDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  uid          = %q
					  display_name = "Test User"
					  email        = "test@example.com"
					  max_buckets  = 100
					  suspended    = false
					  system       = false
					}
				`, testUID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("uid"),
						knownvalue.StringExact(testUID),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("display_name"),
						knownvalue.StringExact("Test User"),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("user_id"),
						knownvalue.StringExact(testUID),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("max_buckets"),
						knownvalue.Int64Exact(100),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("suspended"),
						knownvalue.Bool(false),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("system"),
						knownvalue.Bool(false),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("admin"),
						knownvalue.NotNull(),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "uid", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "display_name", "Test User"),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "user_id", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "suspended", "false"),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "system", "false"),
					resource.TestCheckResourceAttrSet("ceph_rgw_user.test", "admin"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  uid          = %q
					  display_name = "Updated Test User"
					  email        = "updated@example.com"
					  max_buckets  = 200
					  suspended    = true
					  system       = true
					}
				`, testUID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("display_name"),
						knownvalue.StringExact("Updated Test User"),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("max_buckets"),
						knownvalue.Int64Exact(200),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("suspended"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("system"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("admin"),
						knownvalue.NotNull(),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "display_name", "Updated Test User"),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "max_buckets", "200"),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "suspended", "true"),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "system", "true"),
					resource.TestCheckResourceAttrSet("ceph_rgw_user.test", "admin"),
				),
			},
		},
	})
}

func TestAccCephRGWUserResource_fullConfig(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-full-config")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWUserDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "full" {
					  uid          = %q
					  display_name = "Full Config User"
					  email        = "full@example.com"
					  max_buckets  = 500
					  system       = false
					  suspended    = false
					}
				`, testUID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.full",
						tfjsonpath.New("uid"),
						knownvalue.StringExact(testUID),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.full",
						tfjsonpath.New("display_name"),
						knownvalue.StringExact("Full Config User"),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.full",
						tfjsonpath.New("email"),
						knownvalue.StringExact("full@example.com"),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.full",
						tfjsonpath.New("max_buckets"),
						knownvalue.Int64Exact(500),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.full",
						tfjsonpath.New("system"),
						knownvalue.Bool(false),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.full",
						tfjsonpath.New("suspended"),
						knownvalue.Bool(false),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.full", "uid", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.full", "display_name", "Full Config User"),
					resource.TestCheckResourceAttr("ceph_rgw_user.full", "email", "full@example.com"),
					resource.TestCheckResourceAttr("ceph_rgw_user.full", "max_buckets", "500"),
					resource.TestCheckResourceAttr("ceph_rgw_user.full", "system", "false"),
					resource.TestCheckResourceAttr("ceph_rgw_user.full", "suspended", "false"),
					resource.TestCheckResourceAttrSet("ceph_rgw_user.full", "user_id"),
					resource.TestCheckResourceAttrSet("ceph_rgw_user.full", "admin"),
					resource.TestCheckResourceAttrSet("ceph_rgw_user.full", "suspended"),
				),
			},
		},
	})
}

func TestAccCephRGWUserResource_suspendedUser(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-suspended")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWUserDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "suspended" {
					  uid          = %q
					  display_name = "Suspended User"
					  email        = "suspended@example.com"
					  max_buckets  = 100
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.suspended", "uid", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.suspended", "display_name", "Suspended User"),
					resource.TestCheckResourceAttr("ceph_rgw_user.suspended", "suspended", "false"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "suspended" {
					  uid          = %q
					  display_name = "Suspended User"
					  email        = "suspended@example.com"
					  max_buckets  = 100
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.suspended", "uid", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.suspended", "display_name", "Suspended User"),
					resource.TestCheckResourceAttrSet("ceph_rgw_user.suspended", "suspended"),
					resource.TestCheckResourceAttrSet("ceph_rgw_user.suspended", "admin"),
				),
			},
		},
	})
}

func TestAccCephRGWUserResource_systemUser(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-system")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWUserDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "system" {
					  uid          = %q
					  display_name = "System User"
					  system       = true
					}
				`, testUID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.system",
						tfjsonpath.New("system"),
						knownvalue.Bool(true),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.system", "system", "true"),
				),
			},
		},
	})
}

func TestAccCephRGWUserResourceImport(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-import")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWUserDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "import" {
					  uid          = %q
					  display_name = "Import Test User"
					  max_buckets  = 50
					}
				`, testUID),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "import" {
					  uid          = %q
					  display_name = "Import Test User"
					  max_buckets  = 50
					}
				`, testUID),
				ResourceName:                         "ceph_rgw_user.import",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "uid",
				ImportStateId:                        testUID,
			},
		},
	})
}

func TestAccCephRGWUserResourceImport_nonExistent(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-nonexist")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWUserDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "nonexistent" {
					  uid          = %q
					  display_name = "Non-existent User"
					}
				`, testUID),
				ResourceName:  "ceph_rgw_user.nonexistent",
				ImportState:   true,
				ImportStateId: testUID,
				ExpectError:   regexp.MustCompile(`(?i)unable to read rgw user during import`),
			},
		},
	})
}

func TestAccCephRGWUserResource_minimalConfig(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-minimal")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWUserDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "minimal" {
					  uid          = %q
					  display_name = "Minimal User"
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.minimal", "uid", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.minimal", "display_name", "Minimal User"),
					resource.TestCheckResourceAttrSet("ceph_rgw_user.minimal", "user_id"),
				),
			},
		},
	})
}

func testAccCheckCephRGWUserDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "ceph_rgw_user" {
			continue
		}

		uid := rs.Primary.Attributes["uid"]

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := &CephAPIClient{}
		endpoint, err := url.Parse(testDashboardURL)
		if err != nil {
			return fmt.Errorf("failed to parse test dashboard URL: %v", err)
		}

		if err := client.Configure(ctx, []*url.URL{endpoint}, "admin", "password", ""); err != nil {
			return fmt.Errorf("failed to configure client: %v", err)
		}

		_, err = client.RGWGetUser(ctx, uid)
		if err == nil {
			return fmt.Errorf("ceph_rgw_user resource %s still exists", uid)
		}
	}
	return nil
}

func checkCephRGWUserExists(t *testing.T, uid string) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client := &CephAPIClient{}
		endpoint, err := url.Parse(testDashboardURL)
		if err != nil {
			return fmt.Errorf("failed to parse test dashboard URL: %v", err)
		}

		if err := client.Configure(ctx, []*url.URL{endpoint}, "admin", "password", ""); err != nil {
			return fmt.Errorf("failed to configure client: %v", err)
		}

		user, err := client.RGWGetUser(ctx, uid)
		if err != nil {
			return fmt.Errorf("RGW user %s does not exist: %w", uid, err)
		}

		t.Logf("Verified RGW user %s exists with display_name: %s", uid, user.DisplayName)
		return nil
	}
}
