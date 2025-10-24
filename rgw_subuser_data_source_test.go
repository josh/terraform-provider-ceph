package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCephRGWSubuserDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-subuser-ds-user")
	testSubuser := "testsub"
	testSubuserID := fmt.Sprintf("%s:%s", testUID, testSubuser)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUserWithSubuser(t, testUID, "Test Subuser DS User", testSubuser, "full")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_subuser" "test" {
					  id = %q
					}
				`, testSubuserID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_subuser.test", "id", testSubuserID),
					resource.TestCheckResourceAttr("data.ceph_rgw_subuser.test", "permissions", "full-control"),
				),
			},
		},
	})
}

func TestAccCephRGWSubuserDataSource_nonExistent(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-subuser-ds-nonexist")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUser(t, testUID, "Test Subuser DS User NonExist")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_subuser" "nonexistent" {
					  id = %q
					}
				`, testUID+":nonexistent"),
				ExpectError: regexp.MustCompile(`(?i)subuser not found`),
			},
		},
	})
}

func TestAccCephRGWSubuserDataSource_invalidFormat(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_rgw_subuser" "invalid" {
					  id = "invalid_format_without_colon"
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)must be in the format`),
			},
		},
	})
}

func createTestRGWUserWithSubuser(t *testing.T, uid, displayName, subuser, permissions string) {
	t.Helper()

	cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "user", "rm", "--uid="+uid, "--purge-data")
	_ = cmd.Run()

	cmd = exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "user", "create",
		"--uid="+uid,
		"--display-name="+displayName,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create test RGW user: %v\nOutput: %s", err, string(output))
	}

	cmd = exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "subuser", "create",
		"--uid="+uid,
		"--subuser="+uid+":"+subuser,
		"--access="+permissions,
	)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create subuser: %v\nOutput: %s", err, string(output))
	}

	t.Logf("Created test RGW user: %s with subuser: %s", uid, subuser)

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
