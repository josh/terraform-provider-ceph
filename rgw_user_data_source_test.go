package main

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCephRGWUserDataSource(t *testing.T) {
	testUID := "test-user-datasource"

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
					  uid = %q
					}
				`, testUID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "uid", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "user_id", testUID),
					resource.TestCheckResourceAttr("data.ceph_rgw_user.test", "display_name", "Test User DataSource"),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_user.test", "max_buckets"),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_user.test", "system"),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_user.test", "admin"),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_user.test", "keys.#"),
				),
			},
		},
	})
}

func TestAccCephRGWUserDataSource_nonExistent(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_rgw_user" "nonexistent" {
					  uid = "nonexistent-user-12345"
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)unable to get rgw user from ceph api`),
			},
		},
	})
}

func createTestRGWUser(t *testing.T, uid, displayName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client := &CephAPIClient{}
	endpoint, err := url.Parse(testDashboardURL)
	if err != nil {
		t.Fatalf("Failed to parse test dashboard URL: %v", err)
	}

	if err := client.Configure(ctx, []*url.URL{endpoint}, "admin", "password", ""); err != nil {
		t.Fatalf("Failed to configure client: %v", err)
	}

	_ = client.RGWDeleteUser(ctx, uid)

	req := CephAPIRGWUserCreateRequest{
		UID:         uid,
		DisplayName: displayName,
	}

	_, err = client.RGWCreateUser(ctx, req)
	if err != nil {
		t.Fatalf("Failed to create test RGW user: %v", err)
	}

	t.Logf("Created test RGW user: %s", uid)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		if err := client.RGWDeleteUser(cleanupCtx, uid); err != nil {
			t.Logf("Warning: Failed to cleanup test RGW user %s: %v", uid, err)
		} else {
			t.Logf("Cleaned up test RGW user: %s", uid)
		}
	})
}
