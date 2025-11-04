package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

type RadosgwUserInfo struct {
	DisplayName string         `json:"display_name"`
	Keys        []RadosgwS3Key `json:"keys"`
}

type RadosgwS3Key struct {
	AccessKey string `json:"access_key"`
}

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
					  user_id      = %q
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
						tfjsonpath.New("user_id"),
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
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "user_id", testUID),
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
					  user_id      = %q
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
					  user_id      = %q
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
						tfjsonpath.New("user_id"),
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
					resource.TestCheckResourceAttr("ceph_rgw_user.full", "user_id", testUID),
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
					  user_id      = %q
					  display_name = "Suspended User"
					  email        = "suspended@example.com"
					  max_buckets  = 100
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.suspended", "user_id", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.suspended", "display_name", "Suspended User"),
					resource.TestCheckResourceAttr("ceph_rgw_user.suspended", "suspended", "false"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "suspended" {
					  user_id      = %q
					  display_name = "Suspended User"
					  email        = "suspended@example.com"
					  max_buckets  = 100
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.suspended", "user_id", testUID),
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
					  user_id      = %q
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
					  user_id      = %q
					  display_name = "Import Test User"
					  max_buckets  = 50
					}
				`, testUID),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "import" {
					  user_id      = %q
					  display_name = "Import Test User"
					  max_buckets  = 50
					}
				`, testUID),
				ResourceName:                         "ceph_rgw_user.import",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "user_id",
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
					  user_id      = %q
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
					  user_id      = %q
					  display_name = "Minimal User"
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.minimal", "user_id", testUID),
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

		userID := rs.Primary.Attributes["user_id"]

		cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "user", "info", "--uid="+userID)
		output, err := cmd.CombinedOutput()
		if err == nil {
			return fmt.Errorf("ceph_rgw_user resource %s still exists (output: %s)", userID, string(output))
		}
	}
	return nil
}

func checkCephRGWUserExists(t *testing.T, userID string) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "user", "info", "--uid="+userID)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("RGW user %s does not exist: %w", userID, err)
		}

		var user RadosgwUserInfo
		if err := json.Unmarshal(output, &user); err != nil {
			return fmt.Errorf("failed to parse radosgw-admin output: %w", err)
		}

		t.Logf("Verified RGW user %s exists with display_name: %s", userID, user.DisplayName)
		return nil
	}
}

func TestAccCephRGWUserResource_noKeys(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-user-no-keys")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWUserDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Test User No Keys"
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					checkCephRGWUserKeyCount(t, testUID, 0),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "user_id", testUID),
				),
			},
		},
	})
}

func TestAccCephRGWUserResource_managedS3Keys(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-user-managed-key")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWUserDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Test User with Managed S3 Key"
					}

					resource "ceph_rgw_s3_key" "test" {
					  user_id = ceph_rgw_user.test.user_id
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					checkCephRGWUserKeyCount(t, testUID, 1),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "user_id", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.test", "user_id", testUID),
				),
			},
		},
	})
}

func checkCephRGWUserKeyCount(t *testing.T, userID string, expectedCount int) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "user", "info", "--uid="+userID)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("radosgw-admin failed to get user info: %v\nOutput: %s", err, string(output))
		}

		var userInfo RadosgwUserInfo
		if err := json.Unmarshal(output, &userInfo); err != nil {
			return fmt.Errorf("failed to parse radosgw-admin output: %v\nOutput: %s", err, string(output))
		}

		actualCount := len(userInfo.Keys)
		if actualCount != expectedCount {
			return fmt.Errorf("Expected user %s to have %d keys, but found %d keys", userID, expectedCount, actualCount)
		}

		t.Logf("Verified RGW user %s has %d keys as expected", userID, actualCount)
		return nil
	}
}

func TestAccCephRGWUserResource_alreadyExists(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-user-exists")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUserDirectly(t, testUID, "Pre-existing User")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "existing" {
					  user_id      = %q
					  display_name = "Attempt to Create Existing User"
					}
				`, testUID),
				ExpectError: regexp.MustCompile(`(?i)(unable to create rgw user|ceph api returned status)`),
			},
		},
	})
}

func createTestRGWUserDirectly(t *testing.T, uid, displayName string) {
	t.Helper()

	cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "user", "create",
		"--uid="+uid,
		"--display-name="+displayName,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to pre-create test RGW user: %v\nOutput: %s", err, string(output))
	}

	t.Logf("Pre-created test RGW user: %s", uid)

	t.Cleanup(func() {
		cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "user", "rm", "--uid="+uid, "--purge-data")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Logf("Warning: Failed to cleanup test RGW user %s: %v\nOutput: %s", uid, err, string(output))
		} else {
			t.Logf("Cleaned up test RGW user: %s", uid)
		}
	})
}

func TestAccCephRGWUserResource_suspendUnsuspendCycle(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-suspend-cycle")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWUserDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Suspend Cycle Test"
					  email        = "suspend@example.com"
					  suspended    = false
					}
				`, testUID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("user_id"),
						knownvalue.StringExact(testUID),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("suspended"),
						knownvalue.Bool(false),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "user_id", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "suspended", "false"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Suspend Cycle Test"
					  email        = "suspend@example.com"
					  suspended    = true
					}
				`, testUID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("suspended"),
						knownvalue.Bool(true),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "suspended", "true"),
					checkCephRGWUserSuspended(t, testUID, true),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Suspend Cycle Test"
					  email        = "suspend@example.com"
					  suspended    = false
					}
				`, testUID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("suspended"),
						knownvalue.Bool(false),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "suspended", "false"),
					checkCephRGWUserSuspended(t, testUID, false),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Suspend Cycle Test"
					  email        = "suspend@example.com"
					  suspended    = true
					}
				`, testUID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("suspended"),
						knownvalue.Bool(true),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "suspended", "true"),
					checkCephRGWUserSuspended(t, testUID, true),
				),
			},
		},
	})
}

func checkCephRGWUserSuspended(t *testing.T, userID string, expectedSuspended bool) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "user", "info", "--uid="+userID)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("radosgw-admin failed to get user info: %w", err)
		}

		var userInfo struct {
			Suspended int `json:"suspended"`
		}
		if err := json.Unmarshal(output, &userInfo); err != nil {
			return fmt.Errorf("failed to parse radosgw-admin output: %w", err)
		}

		actualSuspended := userInfo.Suspended != 0
		if actualSuspended != expectedSuspended {
			return fmt.Errorf("expected user %s suspended=%v, but got suspended=%v", userID, expectedSuspended, actualSuspended)
		}

		t.Logf("Verified RGW user %s has suspended=%v as expected", userID, expectedSuspended)
		return nil
	}
}

func TestAccCephRGWUserResource_emailUpdate(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-email-update")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWUserDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Email Update Test"
					}
				`, testUID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("user_id"),
						knownvalue.StringExact(testUID),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "user_id", testUID),
					resource.TestCheckNoResourceAttr("ceph_rgw_user.test", "email"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Email Update Test"
					  email        = "newemail@example.com"
					}
				`, testUID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("email"),
						knownvalue.StringExact("newemail@example.com"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "email", "newemail@example.com"),
					checkCephRGWUserEmail(t, testUID, "newemail@example.com"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Email Update Test"
					  email        = "updated@example.com"
					}
				`, testUID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("email"),
						knownvalue.StringExact("updated@example.com"),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "email", "updated@example.com"),
					checkCephRGWUserEmail(t, testUID, "updated@example.com"),
				),
			},
		},
	})
}

func checkCephRGWUserEmail(t *testing.T, userID string, expectedEmail string) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "user", "info", "--uid="+userID)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("radosgw-admin failed to get user info: %w", err)
		}

		var userInfo struct {
			Email string `json:"email"`
		}
		if err := json.Unmarshal(output, &userInfo); err != nil {
			return fmt.Errorf("failed to parse radosgw-admin output: %w", err)
		}

		if userInfo.Email != expectedEmail {
			return fmt.Errorf("expected user %s email=%q, but got email=%q", userID, expectedEmail, userInfo.Email)
		}

		t.Logf("Verified RGW user %s has email=%q as expected", userID, expectedEmail)
		return nil
	}
}

func TestAccCephRGWUserResource_maxBucketsValidation(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-max-buckets")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWUserDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Max Buckets Test"
					  max_buckets  = 0
					}
				`, testUID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("max_buckets"),
						knownvalue.Int64Exact(0),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "max_buckets", "0"),
					checkCephRGWUserMaxBuckets(t, testUID, 0),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Max Buckets Test"
					  max_buckets  = 100
					}
				`, testUID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("max_buckets"),
						knownvalue.Int64Exact(100),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "max_buckets", "100"),
					checkCephRGWUserMaxBuckets(t, testUID, 100),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Max Buckets Test"
					  max_buckets  = 100000
					}
				`, testUID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_user.test",
						tfjsonpath.New("max_buckets"),
						knownvalue.Int64Exact(100000),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "max_buckets", "100000"),
					checkCephRGWUserMaxBuckets(t, testUID, 100000),
				),
			},
		},
	})
}

func checkCephRGWUserMaxBuckets(t *testing.T, userID string, expectedMaxBuckets int) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "user", "info", "--uid="+userID)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("radosgw-admin failed to get user info: %w", err)
		}

		var userInfo struct {
			MaxBuckets int `json:"max_buckets"`
		}
		if err := json.Unmarshal(output, &userInfo); err != nil {
			return fmt.Errorf("failed to parse radosgw-admin output: %w", err)
		}

		if userInfo.MaxBuckets != expectedMaxBuckets {
			return fmt.Errorf("expected user %s max_buckets=%d, but got max_buckets=%d", userID, expectedMaxBuckets, userInfo.MaxBuckets)
		}

		t.Logf("Verified RGW user %s has max_buckets=%d as expected", userID, expectedMaxBuckets)
		return nil
	}
}

func TestAccCephRGWUserResource_suspendOutOfBand(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-suspend-oob")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWUserDestroy,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Out of Band Test"
					  suspended    = false
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "suspended", "false"),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				PreConfig: func() {
					suspendUserViaCLI(t, testUID)
				},
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Out of Band Test"
					  suspended    = false
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "suspended", "false"),
					checkCephRGWUserSuspended(t, testUID, false),
				),
			},
		},
	})
}

func suspendUserViaCLI(t *testing.T, userID string) {
	t.Helper()
	cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "user", "suspend", "--uid="+userID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to suspend user via CLI: %v\nOutput: %s", err, string(output))
	}
	t.Logf("Suspended user %s via CLI", userID)
}

func TestAccCephRGWUserResource_driftDetection(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-drift")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Original Display Name"
					  max_buckets  = 100
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "display_name", "Original Display Name"),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "max_buckets", "100"),
				),
			},
			{
				PreConfig: func() {
					cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "user", "modify",
						"--uid="+testUID,
						"--display-name=Modified Display Name",
						"--max-buckets=200")
					output, err := cmd.CombinedOutput()
					if err != nil {
						t.Fatalf("Failed to modify user out of band: %v\nOutput: %s", err, string(output))
					}
					t.Logf("Modified user %s out of band", testUID)
				},
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Original Display Name"
					  max_buckets  = 100
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserExists(t, testUID),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "display_name", "Original Display Name"),
					resource.TestCheckResourceAttr("ceph_rgw_user.test", "max_buckets", "100"),
				),
			},
		},
	})
}
