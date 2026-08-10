package cephcli

import (
	"context"
	"fmt"
	"os/exec"
)

func (c *CLI) RGWAccountCreate(ctx context.Context, accountID, accountName string) error {
	cmd := exec.CommandContext(ctx, "radosgw-admin", "--conf", c.confPath, "--format=json",
		"account", "create", "--account-name="+accountName, "--account-id="+accountID)
	if _, err := cmd.Output(); err != nil {
		return fmt.Errorf("failed to create rgw account %s: %w", accountID, err)
	}

	return nil
}

func (c *CLI) RGWAccountDelete(ctx context.Context, accountID string) error {
	cmd := exec.CommandContext(ctx, "radosgw-admin", "--conf", c.confPath,
		"account", "rm", "--account-id="+accountID)
	if _, err := cmd.Output(); err != nil {
		return fmt.Errorf("failed to delete rgw account %s: %w", accountID, err)
	}

	return nil
}
