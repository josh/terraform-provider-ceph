package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func createRGWUserAndBucket(ctx context.Context, confPath string, username, bucketName string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", confPath, "rgw", "user", "create", "--uid", username, "--display-name", username)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating RGW user %s: %v\nOutput: %s\n", username, err, output)
		return fmt.Errorf("failed to create RGW user %s: %w", username, err)
	}

	cmd = exec.CommandContext(ctx, "ceph", "--conf", confPath, "osd", "pool", "create", bucketName, "8")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create RGW pool %s: %w", bucketName, err)
	}

	cmd = exec.CommandContext(ctx, "radosgw-admin", "--conf", confPath, "--uid", username, "bucket", "create", "--bucket", bucketName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create RGW bucket %s for user %s: %w", bucketName, username, err)
	}

	return nil
}

func waitForCephRGW(ctx context.Context, confPath string) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			cmd := exec.CommandContext(ctx, "ceph", "--conf", confPath, "rgw", "daemon", "list")
			output, err := cmd.CombinedOutput()
			if err == nil && len(output) > 2 { // Check for non-empty JSON array, e.g., "[]\n"
				return nil
			}
		}
	}
}

func TestAccCephRGWBucketDataSourceFoo(t *testing.T) {
	bucketName := "foo-bucket"
	username := "foo-user"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := waitForCephRGW(ctx, testConfPath); err != nil {
				t.Fatalf("Failed to wait for RGW service: %v", err)
			}

			if err := createRGWUserAndBucket(ctx, testConfPath, username, bucketName); err != nil {
				t.Fatalf("Failed to create RGW user and bucket: %v", err)
			}
		},
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + fmt.Sprintf(`
					data "ceph_rgw_bucket" "foo" {
					  bucket_name = "%s"
					}
				`, bucketName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_rgw_bucket.foo",
						tfjsonpath.New("bucket"),
						knownvalue.StringExact(bucketName),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_rgw_bucket.foo",
						tfjsonpath.New("owner"),
						knownvalue.StringExact(username),
					),
				},
			},
		},
	})
}

func TestAccCephRGWBucketDataSourceBar(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: testAccProviderConfig(),
				Config: testAccProviderConfigBlock + `
					data "ceph_rgw_bucket" "bar" {
					  bucket_name = "bar-bucket"
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)unable to get rgw bucket from ceph api`),
			},
		},
	})
}
