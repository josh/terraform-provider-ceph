package main

import (
	"fmt"
	"os/exec"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephRGWS3KeyResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-s3-key-res")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUserWithoutKeys(t, testUID, "Test S3 Key Resource User")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "test" {
					  uid = %q
					}
				`, testUID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_s3_key.test",
						tfjsonpath.New("uid"),
						knownvalue.StringExact(testUID),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_s3_key.test",
						tfjsonpath.New("user"),
						knownvalue.StringExact(testUID),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_s3_key.test",
						tfjsonpath.New("active"),
						knownvalue.Bool(true),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.test", "uid", testUID),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.test", "access_key"),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.test", "secret_key"),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.test", "user", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.test", "active", "true"),
				),
			},
		},
	})
}

func TestAccCephRGWS3KeyResource_customKeys(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-s3-key-custom")
	customAccessKey := acctest.RandString(20)
	customSecretKey := acctest.RandString(40)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUserWithoutKeys(t, testUID, "Test S3 Key Custom User")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "custom" {
					  uid        = %q
					  access_key = %q
					  secret_key = %q
					}
				`, testUID, customAccessKey, customSecretKey),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.custom", "uid", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.custom", "access_key", customAccessKey),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.custom", "secret_key", customSecretKey),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.custom", "active", "true"),
				),
			},
		},
	})
}

func TestAccCephRGWS3KeyResource_multipleKeys(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-s3-key-multi")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUserWithoutKeys(t, testUID, "Test S3 Key Multi User")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "key1" {
					  uid = %q
					}

					resource "ceph_rgw_s3_key" "key2" {
					  uid = %q
					}
				`, testUID, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key1", "uid", testUID),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.key1", "access_key"),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key2", "uid", testUID),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.key2", "access_key"),
				),
			},
		},
	})
}

func TestAccCephRGWS3KeyResource_subuser(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-s3-key-sub")
	testSubuser := "testsub"
	testSubuserID := fmt.Sprintf("%s:%s", testUID, testSubuser)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUserWithSubuserWithoutKeys(t, testUID, "Test S3 Key Subuser User", testSubuser)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "subuser" {
					  uid = %q
					}
				`, testSubuserID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.subuser", "uid", testSubuserID),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.subuser", "user", testSubuserID),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.subuser", "access_key"),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.subuser", "secret_key"),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.subuser", "active", "true"),
				),
			},
		},
	})
}

func TestAccCephRGWS3KeyResource_nonExistentUser(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_rgw_s3_key" "nonexistent" {
					  uid = "nonexistent-user-12345"
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)unable to create rgw s3 key`),
			},
		},
	})
}

func createTestRGWUserWithoutKeys(t *testing.T, uid, displayName string) {
	t.Helper()

	cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "user", "create",
		"--uid="+uid,
		"--display-name="+displayName,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to create test RGW user: %v\nOutput: %s", err, string(output))
	}

	t.Logf("Created test RGW user without keys: %s", uid)

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

func createTestRGWUserWithSubuserWithoutKeys(t *testing.T, uid, displayName, subuser string) {
	t.Helper()

	cmd := exec.Command("radosgw-admin", "--conf", testConfPath, "--format=json", "user", "create",
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
		"--access=full",
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
