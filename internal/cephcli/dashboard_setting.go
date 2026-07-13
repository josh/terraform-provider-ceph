package cephcli

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func dashboardSettingCommand(action, name string) string {
	return action + "-" + strings.ReplaceAll(strings.ToLower(name), "_", "-")
}

func (c *CLI) DashboardSettingGet(ctx context.Context, name string) (string, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "dashboard", dashboardSettingCommand("get", name))
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get dashboard setting %s: %w", name, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *CLI) DashboardSettingSet(ctx context.Context, name, value string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "dashboard", dashboardSettingCommand("set", name), value)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set dashboard setting %s: %w, output: %s", name, err, string(output))
	}
	return nil
}

func (c *CLI) DashboardSettingReset(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "dashboard", dashboardSettingCommand("reset", name))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to reset dashboard setting %s: %w, output: %s", name, err, string(output))
	}
	return nil
}
