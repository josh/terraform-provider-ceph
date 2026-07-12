package cephcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type DashboardUser struct {
	Username          string   `json:"username"`
	Roles             []string `json:"roles"`
	Name              *string  `json:"name"`
	Email             *string  `json:"email"`
	Enabled           bool     `json:"enabled"`
	PwdExpirationDate *int64   `json:"pwdExpirationDate"`
	PwdUpdateRequired bool     `json:"pwdUpdateRequired"`
	LastUpdate        int64    `json:"lastUpdate"`
}

func (c *CLI) DashboardUserShow(ctx context.Context, username string) (*DashboardUser, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "dashboard", "ac-user-show", username)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to show dashboard user %s: %w", username, err)
	}

	var user DashboardUser
	if err := json.Unmarshal(output, &user); err != nil {
		return nil, fmt.Errorf("failed to parse dashboard user output: %w", err)
	}

	return &user, nil
}

func (c *CLI) DashboardUserExists(ctx context.Context, username string) (bool, error) {
	_, err := c.DashboardUserShow(ctx, username)
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}
	return false, err
}

func (c *CLI) DashboardUserCreate(ctx context.Context, username, password, role string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "dashboard", "ac-user-create", username, "-i", "/dev/stdin", role)
	cmd.Stdin = strings.NewReader(password)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create dashboard user %s: %w, output: %s", username, err, string(output))
	}
	return nil
}

func (c *CLI) DashboardUserDelete(ctx context.Context, username string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "dashboard", "ac-user-delete", username)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete dashboard user %s: %w, output: %s", username, err, string(output))
	}
	return nil
}
