package main

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/josh/terraform-provider-ceph/internal/cephcli"
)

func testAccRGWUserQuotaUserConfig(uid string) string {
	return testAccProviderConfigBlock + fmt.Sprintf(`
		resource "ceph_rgw_user" "test" {
		  user_id      = %q
		  display_name = "Quota Test User"
		}
	`, uid)
}

func TestAccCephRGWUserQuotaResource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	uid := acctest.RandomWithPrefix("test-quota-user")

	quotaConfig := func(enabled bool, maxSizeKB, maxObjects int64) string {
		return testAccRGWUserQuotaUserConfig(uid) + fmt.Sprintf(`
			resource "ceph_rgw_user_quota" "test" {
			  uid         = ceph_rgw_user.test.user_id
			  quota_type  = "user"
			  enabled     = %t
			  max_size_kb = %d
			  max_objects = %d
			}
		`, enabled, maxSizeKB, maxObjects)
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          quotaConfig(true, 10240, 100),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_user_quota.test",
						tfjsonpath.New("enabled"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user_quota.test",
						tfjsonpath.New("max_size_kb"),
						knownvalue.Int64Exact(10240),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user_quota.test",
						tfjsonpath.New("max_objects"),
						knownvalue.Int64Exact(100),
					),
				},
				Check: checkRGWUserQuota(t, uid, "user", func(q cephcli.RgwQuotaInfo) error {
					if !q.Enabled {
						return fmt.Errorf("expected quota enabled")
					}
					if q.MaxSize != 10240*1024 {
						return fmt.Errorf("expected max_size %d, got %d", 10240*1024, q.MaxSize)
					}
					if q.MaxObjects != 100 {
						return fmt.Errorf("expected max_objects 100, got %d", q.MaxObjects)
					}
					return nil
				}),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          quotaConfig(true, 20480, 200),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rgw_user_quota.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: checkRGWUserQuota(t, uid, "user", func(q cephcli.RgwQuotaInfo) error {
					if q.MaxSize != 20480*1024 {
						return fmt.Errorf("expected max_size %d, got %d", 20480*1024, q.MaxSize)
					}
					if q.MaxObjects != 200 {
						return fmt.Errorf("expected max_objects 200, got %d", q.MaxObjects)
					}
					return nil
				}),
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          quotaConfig(true, 20480, 200),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          testAccRGWUserQuotaUserConfig(uid),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rgw_user_quota.test", plancheck.ResourceActionDestroy),
					},
				},
				Check: checkRGWUserQuota(t, uid, "user", func(q cephcli.RgwQuotaInfo) error {
					if q.Enabled {
						return fmt.Errorf("expected quota disabled after destroy")
					}
					// The admin op stores -1024 rather than -1 for
					// unlimited; any negative value means unlimited.
					if q.MaxSize >= 0 {
						return fmt.Errorf("expected unlimited max_size, got %d", q.MaxSize)
					}
					if q.MaxObjects != -1 {
						return fmt.Errorf("expected max_objects -1, got %d", q.MaxObjects)
					}
					return nil
				}),
			},
		},
	})
}

func TestAccCephRGWUserQuotaResource_BucketQuota(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	uid := acctest.RandomWithPrefix("test-quota-user")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccRGWUserQuotaUserConfig(uid) + `
					resource "ceph_rgw_user_quota" "user" {
					  uid         = ceph_rgw_user.test.user_id
					  quota_type  = "user"
					  enabled     = true
					  max_size_kb = 10240
					}

					resource "ceph_rgw_user_quota" "bucket" {
					  uid         = ceph_rgw_user.test.user_id
					  quota_type  = "bucket"
					  enabled     = true
					  max_objects = 500
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_rgw_user_quota.user",
						tfjsonpath.New("max_objects"),
						knownvalue.Int64Exact(-1),
					),
					statecheck.ExpectKnownValue(
						"ceph_rgw_user_quota.bucket",
						tfjsonpath.New("max_size_kb"),
						knownvalue.Int64Exact(-1),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkRGWUserQuota(t, uid, "user", func(q cephcli.RgwQuotaInfo) error {
						if q.MaxSize != 10240*1024 {
							return fmt.Errorf("expected user max_size %d, got %d", 10240*1024, q.MaxSize)
						}
						return nil
					}),
					checkRGWUserQuota(t, uid, "bucket", func(q cephcli.RgwQuotaInfo) error {
						if q.MaxObjects != 500 {
							return fmt.Errorf("expected bucket max_objects 500, got %d", q.MaxObjects)
						}
						return nil
					}),
				),
			},
		},
	})
}

func TestAccCephRGWUserQuotaResource_Drift(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	uid := acctest.RandomWithPrefix("test-quota-user")

	config := testAccRGWUserQuotaUserConfig(uid) + `
		resource "ceph_rgw_user_quota" "test" {
		  uid         = ceph_rgw_user.test.user_id
		  quota_type  = "user"
		  enabled     = true
		  max_size_kb = 10240
		  max_objects = 100
		}
	`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
			},
			{
				PreConfig: func() {
					if err := cephTestClusterCLI.RgwQuotaSet(t.Context(), uid, "user", 99*1024*1024, 999); err != nil {
						t.Fatalf("Failed to set quota out of band: %v", err)
					}
				},
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("ceph_rgw_user_quota.test", plancheck.ResourceActionUpdate),
					},
				},
				Check: checkRGWUserQuota(t, uid, "user", func(q cephcli.RgwQuotaInfo) error {
					if q.MaxSize != 10240*1024 {
						return fmt.Errorf("expected max_size %d after converge, got %d", 10240*1024, q.MaxSize)
					}
					if q.MaxObjects != 100 {
						return fmt.Errorf("expected max_objects 100 after converge, got %d", q.MaxObjects)
					}
					return nil
				}),
			},
		},
	})
}

func TestAccCephRGWUserQuotaResource_Import(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	uid := acctest.RandomWithPrefix("test-quota-user")

	config := testAccRGWUserQuotaUserConfig(uid) + `
		resource "ceph_rgw_user_quota" "test" {
		  uid         = ceph_rgw_user.test.user_id
		  quota_type  = "user"
		  enabled     = true
		  max_size_kb = 10240
		  max_objects = 100
		}
	`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ResourceName:    "ceph_rgw_user_quota.test",
				ImportState:     true,
				ImportStateId:   uid + ":user",
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected 1 state, got %d", len(states))
					}
					attrs := states[0].Attributes
					if attrs["uid"] != uid {
						return fmt.Errorf("expected uid %q, got %q", uid, attrs["uid"])
					}
					if attrs["quota_type"] != "user" {
						return fmt.Errorf("expected quota_type user, got %q", attrs["quota_type"])
					}
					if attrs["enabled"] != "true" {
						return fmt.Errorf("expected enabled true, got %q", attrs["enabled"])
					}
					if attrs["max_size_kb"] != "10240" {
						return fmt.Errorf("expected max_size_kb 10240, got %q", attrs["max_size_kb"])
					}
					if attrs["max_objects"] != "100" {
						return fmt.Errorf("expected max_objects 100, got %q", attrs["max_objects"])
					}
					return nil
				},
			},
			{
				ConfigVariables: testAccProviderConfig(),
				Config:          config,
				ResourceName:    "ceph_rgw_user_quota.test",
				ImportState:     true,
				ImportStateId:   uid + ":banana",
				ExpectError:     regexp.MustCompile(`Invalid Import ID`),
			},
		},
	})
}

func checkRGWUserQuota(t *testing.T, uid, scope string, check func(cephcli.RgwQuotaInfo) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		info, err := cephTestClusterCLI.RgwUserInfo(t.Context(), uid)
		if err != nil {
			return fmt.Errorf("failed to get rgw user info: %w", err)
		}
		if scope == "bucket" {
			return check(info.BucketQuota)
		}
		return check(info.UserQuota)
	}
}
