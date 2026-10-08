package main

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephAuthCiphersDataSource(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	// The harness creates the monmap with --auth-allowed-ciphers aes,aes256k
	// and monmaptool defaults the other two to aes256k.
	var allowed, preferred, service knownvalue.Check = knownvalue.Null(), knownvalue.Null(), knownvalue.Null()
	if monmaptoolSupportsAuthCiphers(t.Context()) {
		allowed = knownvalue.ListExact([]knownvalue.Check{knownvalue.StringExact("aes"), knownvalue.StringExact("aes256k")})
		preferred = knownvalue.StringExact("aes256k")
		service = knownvalue.StringExact("aes256k")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_auth_ciphers" "cluster" {}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("data.ceph_auth_ciphers.cluster", tfjsonpath.New("allowed_ciphers"), allowed),
					statecheck.ExpectKnownValue("data.ceph_auth_ciphers.cluster", tfjsonpath.New("preferred_cipher"), preferred),
					statecheck.ExpectKnownValue("data.ceph_auth_ciphers.cluster", tfjsonpath.New("service_cipher"), service),
				},
			},
		},
	})
}
