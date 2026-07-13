package main

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephRGWUserQuotaDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	uid := acctest.RandomWithPrefix("test-quota-user")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccRGWUserQuotaUserConfig(uid) + `
					resource "ceph_rgw_user_quota" "test" {
					  uid         = ceph_rgw_user.test.user_id
					  quota_type  = "user"
					  enabled     = true
					  max_size_kb = 10240
					  max_objects = 100
					}

					data "ceph_rgw_user_quota" "test" {
					  uid        = ceph_rgw_user.test.user_id
					  quota_type = "user"
					  depends_on = [ceph_rgw_user_quota.test]
					}

					data "ceph_rgw_user_quota" "bucket" {
					  uid        = ceph_rgw_user.test.user_id
					  quota_type = "bucket"
					  depends_on = [ceph_rgw_user_quota.test]
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_rgw_user_quota.test",
						tfjsonpath.New("enabled"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rgw_user_quota.test",
						tfjsonpath.New("max_size"),
						knownvalue.Int64Exact(10240*1024),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rgw_user_quota.test",
						tfjsonpath.New("max_size_kb"),
						knownvalue.Int64Exact(10240),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rgw_user_quota.test",
						tfjsonpath.New("max_objects"),
						knownvalue.Int64Exact(100),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rgw_user_quota.bucket",
						tfjsonpath.New("enabled"),
						knownvalue.Bool(false),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rgw_user_quota.bucket",
						tfjsonpath.New("max_size_kb"),
						knownvalue.Int64Exact(-1),
					),
				},
			},
		},
	})
}
