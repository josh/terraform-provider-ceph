package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephRGWS3KeyResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-s3-key-res")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWS3KeyDestroy(t),
		PreCheck: func() {
			createTestRGWUserWithoutKeys(t, testUID, "Test S3 Key Resource User")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "test" {
					  user_id = %q
					}
				`, testUID),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_s3_key.test",
						tfjsonpath.New("user_id"),
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
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.test", "user_id", testUID),
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
		CheckDestroy:             testAccCheckCephRGWS3KeyDestroy(t),
		PreCheck: func() {
			createTestRGWUserWithoutKeys(t, testUID, "Test S3 Key Custom User")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "custom" {
					  user_id    = %q
					  access_key = %q
					  secret_key = %q
					}
				`, testUID, customAccessKey, customSecretKey),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.custom", "user_id", testUID),
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
		CheckDestroy:             testAccCheckCephRGWS3KeyDestroy(t),
		PreCheck: func() {
			createTestRGWUserWithoutKeys(t, testUID, "Test S3 Key Multi User")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "key1" {
					  user_id = %q
					}

					resource "ceph_rgw_s3_key" "key2" {
					  user_id = %q
					}
				`, testUID, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key1", "user_id", testUID),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.key1", "access_key"),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key2", "user_id", testUID),
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
		CheckDestroy:             testAccCheckCephRGWS3KeyDestroy(t),
		PreCheck: func() {
			createTestRGWUserWithSubuserWithoutKeys(t, testUID, "Test S3 Key Subuser User", testSubuser)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "subuser" {
					  user_id = %q
					}
				`, testSubuserID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.subuser", "user_id", testSubuserID),
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
		CheckDestroy:             testAccCheckCephRGWS3KeyDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					resource "ceph_rgw_s3_key" "nonexistent" {
					  user_id = "nonexistent-user-12345"
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)unable to create rgw s3 key`),
			},
		},
	})
}

func createTestRGWUserWithSubuserWithoutKeys(t *testing.T, uid, displayName, subuser string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	err := cephTestClusterCLI.RgwUserCreate(ctx, uid, displayName, nil)
	if err != nil {
		t.Fatalf("Failed to create test RGW user: %v", err)
	}

	err = cephTestClusterCLI.RgwSubuserCreate(ctx, uid, uid+":"+subuser, &RgwSubuserCreateOptions{
		Access: "full",
	})
	if err != nil {
		t.Fatalf("Failed to create subuser: %v", err)
	}

	t.Logf("Created test RGW user: %s with subuser: %s", uid, subuser)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		if err := cephTestClusterCLI.RgwUserRemove(cleanupCtx, uid, true); err != nil {
			t.Logf("Note: cleanup of RGW user %s reported an error (may already be deleted): %v", uid, err)
		}
	})
}

func TestAccCephRGWS3KeyResource_rotationWorkflow(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-key-rotation")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWS3KeyDestroy(t),
		PreCheck: func() {
			createTestRGWUserWithoutKeys(t, testUID, "Key Rotation Test User")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "key1" {
					  user_id = %q
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key1", "user_id", testUID),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.key1", "access_key"),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.key1", "secret_key"),
					checkCephRGWUserKeyCount(t, testUID, 1),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "key1" {
					  user_id = %q
					}

					resource "ceph_rgw_s3_key" "key2" {
					  user_id = %q
					}
				`, testUID, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key1", "user_id", testUID),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.key1", "access_key"),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key2", "user_id", testUID),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.key2", "access_key"),
					checkCephRGWUserKeyCount(t, testUID, 2),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "key2" {
					  user_id = %q
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key2", "user_id", testUID),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.key2", "access_key"),
					checkCephRGWUserKeyCount(t, testUID, 1),
				),
			},
		},
	})
}

func TestAccCephRGWS3KeyResource_customKeyValidation(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-custom-key-val")
	customAccessKey1 := acctest.RandString(20)
	customSecretKey1 := acctest.RandString(40)
	customAccessKey2 := acctest.RandString(20)
	customSecretKey2 := acctest.RandString(40)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWS3KeyDestroy(t),
		PreCheck: func() {
			createTestRGWUserWithoutKeys(t, testUID, "Custom Key Validation User")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "test" {
					  user_id    = %q
					  access_key = %q
					  secret_key = %q
					}
				`, testUID, customAccessKey1, customSecretKey1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.test", "user_id", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.test", "access_key", customAccessKey1),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.test", "secret_key", customSecretKey1),
					checkCephRGWUserKeyCount(t, testUID, 1),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "test" {
					  user_id    = %q
					  access_key = %q
					  secret_key = %q
					}
				`, testUID, customAccessKey2, customSecretKey2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.test", "user_id", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.test", "access_key", customAccessKey2),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.test", "secret_key", customSecretKey2),
					checkCephRGWUserKeyCount(t, testUID, 1),
				),
			},
		},
	})
}

func TestAccCephRGWS3KeyResource_importWithMultipleKeys(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-import-multi")
	accessKey1 := acctest.RandString(20)
	secretKey1 := acctest.RandString(40)
	accessKey2 := acctest.RandString(20)
	secretKey2 := acctest.RandString(40)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWS3KeyDestroy(t),
		PreCheck: func() {
			createTestRGWUserWithoutKeys(t, testUID, "Import Multiple Keys Test User")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "test" {
					  user_id    = %q
					  access_key = %q
					  secret_key = %q
					}
				`, testUID, accessKey1, secretKey1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.test", "user_id", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.test", "access_key", accessKey1),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.test", "secret_key", secretKey1),
					checkCephRGWUserKeyCount(t, testUID, 1),
				),
			},
			{
				PreConfig: func() {
					createRGWS3Key(t, testUID, accessKey2, secretKey2)
				},
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "test" {
					  user_id    = %q
					  access_key = %q
					  secret_key = %q
					}
				`, testUID, accessKey1, secretKey1),
				ResourceName:                         "ceph_rgw_s3_key.test",
				ImportState:                          true,
				ImportStateId:                        fmt.Sprintf("%s:%s", testUID, accessKey1),
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "access_key",
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserKeyCount(t, testUID, 2),
				),
			},
		},
	})
}

func TestAccCephRGWS3KeyResource_deletionAndRecreation(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-del-recreate")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWS3KeyDestroy(t),
		PreCheck: func() {
			createTestRGWUserWithoutKeys(t, testUID, "Deletion Recreate Test User")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "test" {
					  user_id = %q
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.test", "user_id", testUID),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.test", "access_key"),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.test", "secret_key"),
					checkCephRGWUserKeyCount(t, testUID, 1),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccProviderConfigBlock,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWUserKeyCount(t, testUID, 0),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "test" {
					  user_id = %q
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.test", "user_id", testUID),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.test", "access_key"),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.test", "secret_key"),
					checkCephRGWUserKeyCount(t, testUID, 1),
				),
			},
		},
	})
}
func TestAccCephRGWS3KeyResource_importMultipleKeysManagement(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-import-mgmt")
	accessKey1 := acctest.RandString(20)
	secretKey1 := acctest.RandString(40)
	accessKey2 := acctest.RandString(20)
	secretKey2 := acctest.RandString(40)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWS3KeyDestroy(t),
		PreCheck: func() {
			createTestRGWUserWithoutKeys(t, testUID, "Import Multiple Keys Management User")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "key1" {
					  user_id    = %q
					  access_key = %q
					  secret_key = %q
					}

					resource "ceph_rgw_s3_key" "key2" {
					  user_id    = %q
					  access_key = %q
					  secret_key = %q
					}
				`, testUID, accessKey1, secretKey1, testUID, accessKey2, secretKey2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key1", "user_id", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key1", "access_key", accessKey1),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key2", "user_id", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key2", "access_key", accessKey2),
					checkCephRGWUserKeyCount(t, testUID, 2),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "key1" {
					  user_id    = %q
					  access_key = %q
					  secret_key = %q
					}
				`, testUID, accessKey1, secretKey1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key1", "user_id", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key1", "access_key", accessKey1),
					checkCephRGWUserKeyCount(t, testUID, 1),
				),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_s3_key" "key1" {
					  user_id    = %q
					  access_key = %q
					  secret_key = %q
					}

					resource "ceph_rgw_s3_key" "key3" {
					  user_id = %q
					}
				`, testUID, accessKey1, secretKey1, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key1", "user_id", testUID),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key1", "access_key", accessKey1),
					resource.TestCheckResourceAttr("ceph_rgw_s3_key.key3", "user_id", testUID),
					resource.TestCheckResourceAttrSet("ceph_rgw_s3_key.key3", "access_key"),
					checkCephRGWUserKeyCount(t, testUID, 2),
				),
			},
		},
	})
}

func createRGWS3Key(t *testing.T, userID, accessKey, secretKey string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	err := cephTestClusterCLI.RgwKeyCreate(ctx, userID, &RgwKeyCreateOptions{
		AccessKey: accessKey,
		SecretKey: secretKey,
	})
	if err != nil {
		t.Fatalf("Failed to create RGW S3 key %s for user %s: %v", accessKey, userID, err)
	}

	t.Logf("Created RGW S3 key %s for user %s", accessKey, userID)
}

func testAccCheckCephRGWS3KeyDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_rgw_s3_key" {
				continue
			}

			userID := rs.Primary.Attributes["user_id"]
			accessKey := rs.Primary.Attributes["access_key"]

			parts := strings.SplitN(userID, ":", 2)
			parentUID := parts[0]

			userInfo, err := cephTestClusterCLI.RgwUserInfo(ctx, parentUID)
			if err != nil {
				continue
			}

			for _, key := range userInfo.Keys {
				if key.User == userID && key.AccessKey == accessKey {
					return fmt.Errorf("ceph_rgw_s3_key %s for user %s still exists", accessKey, userID)
				}
			}
		}
		return nil
	}
}
