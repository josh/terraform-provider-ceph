package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCephRGWUserDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-user-ds")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUser(t, testUID, "Test User DataSource")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_user" "test" {
					  user_id = %q
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "user_id", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "display_name", "Test User DataSource"),
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "email", ""),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_user.test", "max_buckets"),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_user.test", "system"),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_user.test", "suspended"),
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "tenant", ""),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_user.test", "admin"),
				),
			},
		},
	})
}

func TestAccCephRGWUserDataSource_nonExistent(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_rgw_user" "nonexistent" {
					  user_id = "nonexistent-user-12345"
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)unable to get rgw user from ceph api`),
			},
		},
	})
}

func TestAccCephRGWUserDataSource_adminFlagOutOfBand(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-admin-flag")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUser(t, testUID, "Test Admin Flag User")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_user" "test" {
					  user_id = %q
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "user_id", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "admin", "false"),
				),
			},
			{
				PreConfig: func() {
					admin := true
					err := cephTestClusterCLI.RgwUserModify(t.Context(), testUID, &RgwUserModifyOptions{
						Admin: &admin,
					})
					if err != nil {
						t.Fatalf("Failed to set admin flag (required for test): %v", err)
					}
					t.Logf("Set admin flag to true for user: %s", testUID)
				},
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_user" "test" {
					  user_id = %q
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "user_id", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "admin", "true"),
				),
			},
			{
				PreConfig: func() {
					admin := false
					err := cephTestClusterCLI.RgwUserModify(t.Context(), testUID, &RgwUserModifyOptions{
						Admin: &admin,
					})
					if err != nil {
						t.Fatalf("Failed to set admin flag (required for test): %v", err)
					}
					t.Logf("Set admin flag to false for user: %s", testUID)
				},
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_user" "test" {
					  user_id = %q
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "user_id", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "admin", "false"),
				),
			},
		},
	})
}

func TestAccCephRGWUserDataSource_deletedOutOfBand(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-deleted-oob")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUser(t, testUID, "Test Deleted OOB User")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_user" "test" {
					  user_id = %q
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "user_id", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "display_name", "Test Deleted OOB User"),
				),
			},
			{
				PreConfig: func() {
					if err := cephTestClusterCLI.RgwUserRemove(t.Context(), testUID, true); err != nil {
						t.Fatalf("Failed to delete user out of band: %v", err)
					}
					t.Logf("Deleted user %s out of band", testUID)
				},
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_user" "test" {
					  user_id = %q
					}
				`, testUID),
				ExpectError: regexp.MustCompile(`(?i)unable to get rgw user from ceph api`),
			},
		},
	})
}

func createTestRGWUser(t *testing.T, uid, displayName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	err := cephTestClusterCLI.RgwUserCreate(ctx, uid, displayName, nil)
	if err != nil {
		t.Fatalf("Failed to create test RGW user: %v", err)
	}

	t.Logf("Created test RGW user: %s", uid)

	testCleanup(t, func(ctx context.Context) {
		if err := cephTestClusterCLI.RgwUserRemove(ctx, uid, true); err != nil && !errors.Is(err, ErrRGWUserNotFound) {
			t.Fatalf("Failed to cleanup RGW user %s: %v", uid, err)
		}
	})
}
