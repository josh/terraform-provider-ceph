package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCephRGWS3KeyDataSource(t *testing.T) {
	testUID := "test-s3-key-ds-user"
	testAccessKey := "TESTDSACCESSKEY12345"
	testSecretKey := "TestDSSecretKey1234567890123456789012"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUserWithCustomS3Key(t, testUID, "Test S3 Key DS User", testAccessKey, testSecretKey)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_s3_key" "test" {
					  uid        = %q
					  access_key = %q
					}
				`, testUID, testAccessKey),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "uid", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "access_key", testAccessKey),
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "secret_key", testSecretKey),
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "user", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "active", "true"),
				),
			},
		},
	})
}

func TestAccCephRGWS3KeyDataSource_nonExistent(t *testing.T) {
	testUID := "test-s3-key-ds-nonexist"
	testAccessKey := "TESTNONEXISTKEY12345"
	testSecretKey := "TestNonExistSecretKey123456789012345"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUserWithCustomS3Key(t, testUID, "Test S3 Key DS User NonExist", testAccessKey, testSecretKey)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_s3_key" "nonexistent" {
					  uid        = %q
					  access_key = "NONEXISTENTKEY123456789"
					}
				`, testUID),
				ExpectError: regexp.MustCompile(`(?i)key not found`),
			},
		},
	})
}

func createTestRGWUserWithCustomS3Key(t *testing.T, uid, displayName, accessKey, secretKey string) {
	t.Helper()

	cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "user", "rm", "--uid="+uid, "--purge-data")
	_ = cmd.Run()

	cmd = exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "user", "create",
		"--uid="+uid,
		"--display-name="+displayName,
		"--gen-access-key=false",
		"--gen-secret=false",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create test RGW user: %v\nOutput: %s", err, string(output))
	}

	cmd = exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "key", "create",
		"--uid="+uid,
		"--key-type=s3",
		"--access-key="+accessKey,
		"--secret-key="+secretKey,
		"--gen-access-key=false",
		"--gen-secret=false",
	)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create S3 key: %v\nOutput: %s", err, string(output))
	}

	t.Logf("Created test RGW user: %s with custom S3 key: %s", uid, accessKey)

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
