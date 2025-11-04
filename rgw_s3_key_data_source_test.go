package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCephRGWS3KeyDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-s3-key-ds-user")
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
					  user_id    = %q
					  access_key = %q
					}
				`, testUID, testAccessKey),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "user_id", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "access_key", testAccessKey),
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "secret_key", testSecretKey),
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "active", "true"),
				),
			},
		},
	})
}

func TestAccCephRGWS3KeyDataSource_nonExistent(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-s3-key-ds-nonexist")
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
					  user_id    = %q
					  access_key = "NONEXISTENTKEY123456789"
					}
				`, testUID),
				ExpectError: regexp.MustCompile(`(?i)key not found`),
			},
		},
	})
}

func TestAccCephRGWS3KeyDataSource_singleKeyNoAccessKey(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-s3-key-ds-single")
	testAccessKey := "TESTSINGLEDSAKEY12345"
	testSecretKey := "TestSingleDSSecretKey1234567890123"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUserWithCustomS3Key(t, testUID, "Test S3 Key DS Single", testAccessKey, testSecretKey)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_s3_key" "test" {
					  user_id = %q
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "user_id", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "secret_key", testSecretKey),
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "active", "true"),
				),
			},
		},
	})
}

func TestAccCephRGWS3KeyDataSource_subuserWithParentKeys(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-s3-key-ds-subuser")
	testSubuser := "testsub"
	testSubuserID := fmt.Sprintf("%s:%s", testUID, testSubuser)
	testParentAccessKey := "TESTPARENTKEY1234567"
	testParentSecretKey := "TestParentSecretKey123456789012345"
	testSubuserAccessKey := "TESTSUBUSERKEY123456"
	testSubuserSecretKey := "TestSubuserSecretKey123456789012"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUserWithSubuserAndS3Keys(t, testUID, "Test S3 Key DS Subuser", testSubuser,
				testParentAccessKey, testParentSecretKey, testSubuserAccessKey, testSubuserSecretKey)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_s3_key" "subuser" {
					  user_id = %q
					}
				`, testSubuserID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.subuser", "user_id", testSubuserID),
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.subuser", "secret_key", testSubuserSecretKey),
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.subuser", "active", "true"),
				),
			},
		},
	})
}

func TestAccCephRGWS3KeyDataSource_multipleKeys(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-s3-key-ds-multi")
	testAccessKey1 := "TESTMULTIKEY1ACCESSK"
	testSecretKey1 := "TestMultiKey1SecretKey12345678901"
	testAccessKey2 := "TESTMULTIKEY2ACCESSK"
	testSecretKey2 := "TestMultiKey2SecretKey12345678901"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUserWithMultipleS3Keys(t, testUID, "Test S3 Key DS Multi",
				testAccessKey1, testSecretKey1, testAccessKey2, testSecretKey2)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_s3_key" "test" {
					  user_id = %q
					}
				`, testUID),
				ExpectError: regexp.MustCompile(`(?i)multiple keys found`),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_s3_key" "test" {
					  user_id    = %q
					  access_key = %q
					}
				`, testUID, testAccessKey1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "user_id", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "access_key", testAccessKey1),
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "secret_key", testSecretKey1),
				),
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
		"--access-key="+accessKey,
		"--secret-key="+secretKey,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create test RGW user: %v\nOutput: %s", err, string(output))
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

func createTestRGWUserWithSubuserAndS3Keys(t *testing.T, uid, displayName, subuser, parentAccessKey, parentSecretKey, subuserAccessKey, subuserSecretKey string) {
	t.Helper()

	cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "user", "rm", "--uid="+uid, "--purge-data")
	_ = cmd.Run()

	cmd = exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "user", "create",
		"--uid="+uid,
		"--display-name="+displayName,
		"--access-key="+parentAccessKey,
		"--secret-key="+parentSecretKey,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create test RGW user: %v\nOutput: %s", err, string(output))
	}

	cmd = exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "subuser", "create",
		"--uid="+uid,
		"--subuser="+uid+":"+subuser,
		"--access=full",
	)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create subuser: %v\nOutput: %s", err, string(output))
	}

	cmd = exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "key", "create",
		"--uid="+uid,
		"--subuser="+uid+":"+subuser,
		"--key-type=s3",
		"--access-key="+subuserAccessKey,
		"--secret-key="+subuserSecretKey,
	)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create subuser S3 key: %v\nOutput: %s", err, string(output))
	}

	t.Logf("Created test RGW user: %s with subuser: %s and S3 keys", uid, subuser)

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

func createTestRGWUserWithMultipleS3Keys(t *testing.T, uid, displayName, accessKey1, secretKey1, accessKey2, secretKey2 string) {
	t.Helper()

	cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "user", "rm", "--uid="+uid, "--purge-data")
	_ = cmd.Run()

	cmd = exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "user", "create",
		"--uid="+uid,
		"--display-name="+displayName,
		"--access-key="+accessKey1,
		"--secret-key="+secretKey1,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create test RGW user: %v\nOutput: %s", err, string(output))
	}

	cmd = exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "key", "create",
		"--uid="+uid,
		"--key-type=s3",
		"--access-key="+accessKey2,
		"--secret-key="+secretKey2,
	)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create second S3 key: %v\nOutput: %s", err, string(output))
	}

	t.Logf("Created test RGW user: %s with multiple S3 keys", uid)

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

func TestAccCephRGWS3KeyDataSource_ambiguousResults(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-ambiguous-s3key")
	accessKey1 := acctest.RandString(20)
	secretKey1 := acctest.RandString(40)
	accessKey2 := acctest.RandString(20)
	secretKey2 := acctest.RandString(40)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUserWithoutKeys(t, testUID, "Test Ambiguous S3 Key User")

			cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "key", "create",
				"--uid="+testUID,
				"--access-key="+accessKey1,
				"--secret-key="+secretKey1,
			)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Failed to create first key: %v\nOutput: %s", err, string(output))
			}

			cmd = exec.Command("radosgw-admin", "--conf", testConfPath, "key", "create",
				"--uid="+testUID,
				"--access-key="+accessKey2,
				"--secret-key="+secretKey2,
			)
			output, err = cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Failed to create second key: %v\nOutput: %s", err, string(output))
			}

			t.Logf("Created user %s with two keys", testUID)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_s3_key" "test" {
					  user_id = %q
					}
				`, testUID),
				ExpectError: regexp.MustCompile("(?i)(multiple|ambiguous)"),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_s3_key" "test" {
					  user_id    = %q
					  access_key = %q
					}
				`, testUID, accessKey1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "user_id", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_s3_key.test", "access_key", accessKey1),
				),
			},
		},
	})
}
