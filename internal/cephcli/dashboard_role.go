package cephcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

type DashboardRole struct {
	Name              string              `json:"name"`
	Description       *string             `json:"description"`
	ScopesPermissions map[string][]string `json:"scopes_permissions"`
}

func (c *CLI) DashboardRoleShow(ctx context.Context, name string) (*DashboardRole, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "dashboard", "ac-role-show", name)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to show dashboard role %s: %w", name, err)
	}

	var role DashboardRole
	if err := json.Unmarshal(output, &role); err != nil {
		return nil, fmt.Errorf("failed to parse dashboard role output: %w", err)
	}

	return &role, nil
}

func (c *CLI) DashboardRoleExists(ctx context.Context, name string) (bool, error) {
	_, err := c.DashboardRoleShow(ctx, name)
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}

func (c *CLI) DashboardRoleCreate(ctx context.Context, name, description string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "dashboard", "ac-role-create", name, description)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create dashboard role %s: %w, output: %s", name, err, string(output))
	}
	return nil
}

func (c *CLI) DashboardRoleAddScopePerms(ctx context.Context, name, scope string, perms []string) error {
	args := []string{"--conf", c.confPath, "dashboard", "ac-role-add-scope-perms", name, scope}
	args = append(args, perms...)
	cmd := exec.CommandContext(ctx, "ceph", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add scope perms to dashboard role %s: %w, output: %s", name, err, string(output))
	}
	return nil
}

func (c *CLI) DashboardRoleDelete(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "dashboard", "ac-role-delete", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete dashboard role %s: %w, output: %s", name, err, string(output))
	}
	return nil
}
