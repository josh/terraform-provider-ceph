package main

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCephRGWSwiftKeyDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-swift-key-ds-user")
	testSubuser := "swiftsub"
	testSubuserID := fmt.Sprintf("%s:%s", testUID, testSubuser)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUserWithSubuser(t, testUID, "Test Swift Key DS User", testSubuser, "full")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_swift_key" "test" {
					  user = %q
					}
				`, testSubuserID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_swift_key.test", "user", testSubuserID),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_swift_key.test", "secret_key"),
					resource.TestCheckResourceAttr("data.ceph_rgw_swift_key.test", "active", "true"),
				),
			},
		},
	})
}

func TestAccCephRGWSwiftKeyDataSource_nonExistent(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	testUID := acctest.RandomWithPrefix("test-swift-key-ds-nonexist")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWUser(t, testUID, "Test Swift Key DS User NonExist")
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_swift_key" "nonexistent" {
					  user = %q
					}
				`, testUID+":nonexistent"),
				ExpectError: regexp.MustCompile(`(?i)swift key not found`),
			},
		},
	})
}

func TestAccCephRGWSwiftKeyDataSource_invalidFormat(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_rgw_swift_key" "invalid" {
					  user = "invalid_format_without_colon"
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)parent_user:subuser`),
			},
		},
	})
}
