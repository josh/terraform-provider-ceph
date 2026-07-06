package cephcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

type RgwBucketInfo struct {
	Owner string `json:"owner"`
}

func (c *CLI) RgwBucketInfo(ctx context.Context, bucket string) (*RgwBucketInfo, error) {
	cmd := exec.CommandContext(ctx, "radosgw-admin", "--conf", c.confPath, "--format=json", "bucket", "stats", "--bucket="+bucket)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get rgw bucket info for %s: %w", bucket, err)
	}

	var bucketInfo RgwBucketInfo
	if err := json.Unmarshal(output, &bucketInfo); err != nil {
		return nil, fmt.Errorf("failed to parse rgw bucket info output: %w", err)
	}

	return &bucketInfo, nil
}

func (c *CLI) RgwBucketRemove(ctx context.Context, bucket string, purgeObjects bool) error {
	args := []string{"--conf", c.confPath, "bucket", "rm", "--bucket=" + bucket}
	if purgeObjects {
		args = append(args, "--purge-objects")
	}

	cmd := exec.CommandContext(ctx, "radosgw-admin", args...)
	_, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to remove rgw bucket %s: %w", bucket, err)
	}

	return nil
}
