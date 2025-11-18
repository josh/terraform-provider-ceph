package main

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

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

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	_ = cephTestClusterCLI.RgwUserRemove(ctx, uid, true)

	err := cephTestClusterCLI.RgwUserCreate(ctx, uid, displayName, &RgwUserCreateOptions{
		AccessKey: accessKey,
		SecretKey: secretKey,
	})
	if err != nil {
		t.Fatalf("Failed to create test RGW user: %v", err)
	}

	t.Logf("Created test RGW user: %s with custom S3 key: %s", uid, accessKey)

	cleanupCtxParent := t.Context()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(cleanupCtxParent, 10*time.Second)
		defer cleanupCancel()

		if err := cephTestClusterCLI.RgwUserRemove(cleanupCtx, uid, true); err != nil {
			t.Logf("Warning: Failed to cleanup test RGW user %s: %v", uid, err)
		} else {
			t.Logf("Cleaned up test RGW user: %s", uid)
		}
	})
}

func createTestRGWUserWithSubuserAndS3Keys(t *testing.T, uid, displayName, subuser, parentAccessKey, parentSecretKey, subuserAccessKey, subuserSecretKey string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	_ = cephTestClusterCLI.RgwUserRemove(ctx, uid, true)

	err := cephTestClusterCLI.RgwUserCreate(ctx, uid, displayName, &RgwUserCreateOptions{
		AccessKey: parentAccessKey,
		SecretKey: parentSecretKey,
	})
	if err != nil {
		t.Fatalf("Failed to create test RGW user: %v", err)
	}

	err = cephTestClusterCLI.RgwSubuserCreate(ctx, uid, uid+":"+subuser, &RgwSubuserCreateOptions{
		Access: "full",
	})
	if err != nil {
		t.Fatalf("Failed to create subuser: %v", err)
	}

	err = cephTestClusterCLI.RgwKeyCreate(ctx, uid, &RgwKeyCreateOptions{
		Subuser:   uid + ":" + subuser,
		KeyType:   "s3",
		AccessKey: subuserAccessKey,
		SecretKey: subuserSecretKey,
	})
	if err != nil {
		t.Fatalf("Failed to create subuser S3 key: %v", err)
	}

	t.Logf("Created test RGW user: %s with subuser: %s and S3 keys", uid, subuser)

	cleanupCtxParent := t.Context()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(cleanupCtxParent, 10*time.Second)
		defer cleanupCancel()

		if err := cephTestClusterCLI.RgwUserRemove(cleanupCtx, uid, true); err != nil {
			t.Logf("Warning: Failed to cleanup test RGW user %s: %v", uid, err)
		} else {
			t.Logf("Cleaned up test RGW user: %s", uid)
		}
	})
}

func createTestRGWUserWithMultipleS3Keys(t *testing.T, uid, displayName, accessKey1, secretKey1, accessKey2, secretKey2 string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	_ = cephTestClusterCLI.RgwUserRemove(ctx, uid, true)

	err := cephTestClusterCLI.RgwUserCreate(ctx, uid, displayName, &RgwUserCreateOptions{
		AccessKey: accessKey1,
		SecretKey: secretKey1,
	})
	if err != nil {
		t.Fatalf("Failed to create test RGW user: %v", err)
	}

	err = cephTestClusterCLI.RgwKeyCreate(ctx, uid, &RgwKeyCreateOptions{
		KeyType:   "s3",
		AccessKey: accessKey2,
		SecretKey: secretKey2,
	})
	if err != nil {
		t.Fatalf("Failed to create second S3 key: %v", err)
	}

	t.Logf("Created test RGW user: %s with multiple S3 keys", uid)

	cleanupCtxParent := t.Context()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(cleanupCtxParent, 10*time.Second)
		defer cleanupCancel()

		if err := cephTestClusterCLI.RgwUserRemove(cleanupCtx, uid, true); err != nil {
			t.Logf("Warning: Failed to cleanup test RGW user %s: %v", uid, err)
		} else {
			t.Logf("Cleaned up test RGW user: %s", uid)
		}
	})
}

func createTestRGWUserWithoutKeys(t *testing.T, uid, displayName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	err := cephTestClusterCLI.RgwUserCreate(ctx, uid, displayName, nil)
	if err != nil {
		t.Fatalf("Failed to create test RGW user: %v", err)
	}

	userInfo, err := cephTestClusterCLI.RgwUserInfo(ctx, uid)
	if err != nil {
		t.Fatalf("Failed to get user info: %v", err)
	}

	for _, key := range userInfo.Keys {
		if err := cephTestClusterCLI.RgwKeyRemove(ctx, uid, key.AccessKey); err != nil {
			t.Fatalf("Failed to remove auto-generated key: %v", err)
		}
		t.Logf("Removed auto-generated key %s from user %s", key.AccessKey, uid)
	}

	t.Logf("Created test RGW user without keys: %s", uid)

	cleanupCtxParent := t.Context()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(cleanupCtxParent, 10*time.Second)
		defer cleanupCancel()

		if err := cephTestClusterCLI.RgwUserRemove(cleanupCtx, uid, true); err != nil {
			t.Logf("Warning: Failed to cleanup test RGW user %s: %v", uid, err)
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

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			err := cephTestClusterCLI.RgwKeyCreate(ctx, testUID, &RgwKeyCreateOptions{
				AccessKey: accessKey1,
				SecretKey: secretKey1,
			})
			if err != nil {
				t.Fatalf("Failed to create first key: %v", err)
			}

			err = cephTestClusterCLI.RgwKeyCreate(ctx, testUID, &RgwKeyCreateOptions{
				AccessKey: accessKey2,
				SecretKey: secretKey2,
			})
			if err != nil {
				t.Fatalf("Failed to create second key: %v", err)
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
