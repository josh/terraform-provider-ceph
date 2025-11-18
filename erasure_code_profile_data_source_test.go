package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccCephErasureCodeProfileDataSource_k2m1(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	profileName := fmt.Sprintf("test-profile-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.ErasureCodeProfileSet(ctx, profileName, map[string]string{
				"k":                    "2",
				"m":                    "1",
				"crush-failure-domain": "osd",
			}); err != nil {
				t.Fatalf("Failed to create erasure code profile: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()

				if err := cephTestClusterCLI.ErasureCodeProfileRemove(cleanupCtx, profileName); err != nil {
					t.Errorf("Failed to cleanup erasure code profile %s: %v", profileName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_erasure_code_profile" "test" {
						name = %q
					}
				`, profileName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_erasure_code_profile.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(profileName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_erasure_code_profile.test",
						tfjsonpath.New("k"),
						knownvalue.Int64Exact(2),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_erasure_code_profile.test",
						tfjsonpath.New("m"),
						knownvalue.Int64Exact(1),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_erasure_code_profile.test",
						tfjsonpath.New("crush_failure_domain"),
						knownvalue.StringExact("osd"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_erasure_code_profile.test",
						tfjsonpath.New("plugin"),
						knownvalue.NotNull(),
					),
				},
			},
		},
	})
}

func TestAccCephErasureCodeProfileDataSource_k3m2(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	profileName := fmt.Sprintf("test-profile-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()

			if err := cephTestClusterCLI.ErasureCodeProfileSet(ctx, profileName, map[string]string{
				"k":                    "3",
				"m":                    "2",
				"crush-failure-domain": "host",
			}); err != nil {
				t.Fatalf("Failed to create erasure code profile: %v", err)
			}

			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cleanupCancel()

				if err := cephTestClusterCLI.ErasureCodeProfileRemove(cleanupCtx, profileName); err != nil {
					t.Errorf("Failed to cleanup erasure code profile %s: %v", profileName, err)
				}
			})
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_erasure_code_profile" "test" {
						name = %q
					}
				`, profileName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_erasure_code_profile.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(profileName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_erasure_code_profile.test",
						tfjsonpath.New("k"),
						knownvalue.Int64Exact(3),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_erasure_code_profile.test",
						tfjsonpath.New("m"),
						knownvalue.Int64Exact(2),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_erasure_code_profile.test",
						tfjsonpath.New("crush_failure_domain"),
						knownvalue.StringExact("host"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_erasure_code_profile.test",
						tfjsonpath.New("plugin"),
						knownvalue.NotNull(),
					),
				},
			},
		},
	})
}
