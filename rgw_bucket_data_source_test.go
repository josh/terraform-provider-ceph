package main

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephRGWBucketDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-bucket-owner-ds")
	testBucket := acctest.RandomWithPrefix("test-bucket-ds")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Bucket DataSource Test User"
					}

					resource "ceph_rgw_s3_key" "test" {
					  user_id = ceph_rgw_user.test.user_id
					}

					resource "ceph_rgw_bucket" "test" {
					  bucket = %q
					  owner  = ceph_rgw_user.test.user_id
					  depends_on = [ceph_rgw_s3_key.test]
					}

					data "ceph_rgw_bucket" "test" {
					  bucket = ceph_rgw_bucket.test.bucket
					}
				`, testUID, testBucket),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_bucket.test", "bucket", testBucket),
					resource.TestCheckResourceAttr("data.ceph_rgw_bucket.test", "owner", testUID),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_bucket.test", "id"),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_bucket.test", "zonegroup"),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_bucket.test", "placement_rule"),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_bucket.test", "creation_time"),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_bucket.test", "bid"),
				),
			},
		},
	})
}

func TestAccCephRGWBucketDataSource_nonExistent(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_rgw_bucket" "nonexistent" {
					  bucket = "nonexistent-bucket-12345"
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)unable to get rgw bucket from ceph api`),
			},
		},
	})
}

func TestAccCephRGWBucketDataSource_Enriched(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-bucket-owner-dse")
	testBucket := acctest.RandomWithPrefix("test-bucket-dse")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					resource "ceph_rgw_user" "test" {
					  user_id      = %q
					  display_name = "Bucket DataSource Enriched Test User"
					}

					resource "ceph_rgw_s3_key" "test" {
					  user_id = ceph_rgw_user.test.user_id
					}

					resource "ceph_rgw_bucket" "test" {
					  bucket           = %q
					  owner            = ceph_rgw_user.test.user_id
					  versioning_state = "Enabled"
					  tags             = { env = "test" }
					  depends_on       = [ceph_rgw_s3_key.test]
					}

					data "ceph_rgw_bucket" "test" {
					  bucket = ceph_rgw_bucket.test.bucket
					}
				`, testUID, testBucket),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_rgw_bucket.test",
						tfjsonpath.New("versioning_state"),
						knownvalue.StringExact("Enabled"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rgw_bucket.test",
						tfjsonpath.New("tags"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"env": knownvalue.StringExact("test"),
						}),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rgw_bucket.test",
						tfjsonpath.New("bucket_policy"),
						knownvalue.Null(),
					),
				},
			},
		},
	})
}
