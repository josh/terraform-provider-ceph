package main

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCephRGWRoleDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	roleName := acctest.RandomWithPrefix("test-role-ds")
	assumePolicy := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam:::user/someuser"},"Action":"sts:AssumeRole"}]}`

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			createTestRGWRoleDirectly(t, roleName, "/", assumePolicy)
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_role" "test" {
					  name = %q
					}
				`, roleName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.ceph_rgw_role.test", "name", roleName),
					resource.TestCheckResourceAttr("data.ceph_rgw_role.test", "id", roleName),
					resource.TestCheckResourceAttr("data.ceph_rgw_role.test", "path", "/"),
					resource.TestCheckResourceAttr("data.ceph_rgw_role.test", "max_session_duration", "3600"),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_role.test", "arn"),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_role.test", "role_id"),
					resource.TestCheckResourceAttrSet("data.ceph_rgw_role.test", "assume_role_policy_document"),
					resource.TestCheckResourceAttr("data.ceph_rgw_role.test", "permission_policies", "[]"),
					resource.TestCheckResourceAttr("data.ceph_rgw_role.test", "description", ""),
				),
			},
		},
	})
}

func TestAccCephRGWRoleDataSource_nonExistent(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_rgw_role" "nonexistent" {
					  name = "nonexistent-role-12345"
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)unable to get rgw role from ceph api`),
			},
		},
	})
}
