package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCephRGWUserDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-user-ds")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUser(t, testUID, "Test User DataSource")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_user" "test" {
					  user_id = %q
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "user_id", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "display_name", "Test User DataSource"),
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "email", ""),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_user.test", "max_buckets"),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_user.test", "system"),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_user.test", "suspended"),
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "tenant", ""),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_user.test", "admin"),
				),
			},
		},
	})
}

func TestAccCephRGWUserDataSource_nonExistent(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_rgw_user" "nonexistent" {
					  user_id = "nonexistent-user-12345"
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)unable to get rgw user from ceph api`),
			},
		},
	})
}

func TestAccCephRGWUserDataSource_adminFlagOutOfBand(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-admin-flag")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUser(t, testUID, "Test Admin Flag User")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_user" "test" {
					  user_id = %q
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "user_id", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "admin", "false"),
				),
			},
			{
				PreConfig: func() {
					cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "user", "modify", "--uid="+testUID, "--admin")
					output, err := cmd.CombinedOutput()
					if err != nil {
						t.Logf("radosgw-admin failed (expected in test environment): %v\nOutput: %s", err, string(output))
						return
					}
					t.Logf("Set admin flag to true for user: %s", testUID)
				},
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_user" "test" {
					  user_id = %q
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "user_id", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "admin", "true"),
				),
			},
			{
				PreConfig: func() {
					cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "user", "modify", "--uid="+testUID, "--admin=0")
					output, err := cmd.CombinedOutput()
					if err != nil {
						t.Logf("radosgw-admin failed (expected in test environment): %v\nOutput: %s", err, string(output))
						return
					}
					t.Logf("Set admin flag to false for user: %s", testUID)
				},
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_user" "test" {
					  user_id = %q
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "user_id", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "admin", "false"),
				),
			},
		},
	})
}

func createTestRGWUser(t *testing.T, uid, displayName string) {
	t.Helper()

	cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "user", "create",
		"--uid="+uid,
		"--display-name="+displayName,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create test RGW user: %v\nOutput: %s", err, string(output))
	}

	t.Logf("Created test RGW user: %s", uid)

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
