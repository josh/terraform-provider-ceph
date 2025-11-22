package main

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccCephRGWBucketResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-bucket-owner")
	testBucket := acctest.RandomWithPrefix("test-bucket")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWBucketDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Bucket Test User"
					}

					resource "ceph_rgw_s3_key" "test" {
					  user_id = ceph_rgw_user.test.user_id
					}

					resource "ceph_rgw_bucket" "test" {
					  bucket = %q
					  owner  = ceph_rgw_user.test.user_id
					  depends_on = [ceph_rgw_s3_key.test]
					}
				`, testUID, testBucket),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephRGWBucketExists(t, testBucket),
					resource.TestCheckResourceAttr("ceph_rgw_bucket.test", "bucket", testBucket),
					resource.TestCheckResourceAttr("ceph_rgw_bucket.test", "owner", testUID),
					resource.TestCheckResourceAttrSet("ceph_rgw_bucket.test", "id"),
					resource.TestCheckResourceAttrSet("ceph_rgw_bucket.test", "creation_time"),
				),
			},
		},
	})
}

func TestAccCephRGWBucketResourceImport(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-bucket-import-owner")
	testBucket := acctest.RandomWithPrefix("test-bucket-import")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCephRGWBucketDestroy(t),
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Bucket Import Test User"
					}

					resource "ceph_rgw_s3_key" "test" {
					  user_id = ceph_rgw_user.test.user_id
					}

					resource "ceph_rgw_bucket" "test" {
					  bucket = %q
					  owner  = ceph_rgw_user.test.user_id
					  depends_on = [ceph_rgw_s3_key.test]
					}
				`, testUID, testBucket),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Bucket Import Test User"
					}

					resource "ceph_rgw_s3_key" "test" {
					  user_id = ceph_rgw_user.test.user_id
					}

					resource "ceph_rgw_bucket" "test" {
					  bucket = %q
					  owner  = ceph_rgw_user.test.user_id
					  depends_on = [ceph_rgw_s3_key.test]
					}
				`, testUID, testBucket),
				ResourceName:                         "ceph_rgw_bucket.test",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "bucket",
				ImportStateId:                        testBucket,
			},
		},
	})
}

func TestAccCephRGWBucketResourceImport_nonExistent(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testBucket := acctest.RandomWithPrefix("test-bucket-nonexist")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_bucket" "nonexistent" {
					  bucket = %q
					  owner  = "nonexistent-user"
					}
				`, testBucket),
				ResourceName:  "ceph_rgw_bucket.nonexistent",
				ImportState:   true,
				ImportStateId: testBucket,
				ExpectError:   regexp.MustCompile(`(?i)unable to read rgw bucket`),
			},
		},
	})
}

func testAccCheckCephRGWBucketDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx := t.Context()

		for _, rs := range s.RootModule().Resources {
			if rs.Type != "ceph_rgw_bucket" {
				continue
			}

			bucketName := rs.Primary.Attributes["bucket"]

			_, err := cephTestClusterCLI.RgwBucketInfo(ctx, bucketName)
			if err == nil {
				return fmt.Errorf("ceph_rgw_bucket resource %s still exists", bucketName)
			}
		}
		return nil
	}
}

func checkCephRGWBucketExists(t *testing.T, bucketName string) resource.TestCheckFunc {
	t.Helper()
	return func(s *terraform.State) error {
		bucket, err := cephTestClusterCLI.RgwBucketInfo(t.Context(), bucketName)
		if err != nil {
			return fmt.Errorf("RGW bucket %s does not exist: %w", bucketName, err)
		}

		t.Logf("Verified RGW bucket %s exists with owner: %s", bucketName, bucket.Owner)
		return nil
	}
}
