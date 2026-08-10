package cephcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

var ErrRGWRoleNotFound = errors.New("rgw role not found")

type RgwRoleInfo struct {
	RoleID                   string `json:"RoleId"`
	RoleName                 string `json:"RoleName"`
	Path                     string `json:"Path"`
	Arn                      string `json:"Arn"`
	CreateDate               string `json:"CreateDate"`
	MaxSessionDuration       int64  `json:"MaxSessionDuration"`
	AssumeRolePolicyDocument string `json:"AssumeRolePolicyDocument"`
	AccountID                string `json:"AccountId"`
}

type rgwRoleGetOutput struct {
	Role RgwRoleInfo `json:"role"`
}

func (c *CLI) RGWRoleCreate(ctx context.Context, accountID, name, path, assumePolicyDoc string) error {
	args := []string{"--conf", c.confPath, "--format=json", "role", "create",
		"--role-name=" + name, "--path=" + path, "--assume-role-policy-doc=" + assumePolicyDoc}
	if accountID != "" {
		args = append(args, "--account-id="+accountID)
	}

	cmd := exec.CommandContext(ctx, "radosgw-admin", args...)
	if _, err := cmd.Output(); err != nil {
		return fmt.Errorf("failed to create rgw role %s: %w", name, err)
	}

	if _, err := c.RGWRoleGet(ctx, accountID, name); err != nil {
		return fmt.Errorf("failed to verify rgw role creation: %w", err)
	}

	return nil
}

func (c *CLI) RGWRoleGet(ctx context.Context, accountID, name string) (*RgwRoleInfo, error) {
	args := []string{"--conf", c.confPath, "--format=json", "role", "get", "--role-name=" + name}
	if accountID != "" {
		args = append(args, "--account-id="+accountID)
	}

	cmd := exec.CommandContext(ctx, "radosgw-admin", args...)
	output, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("failed to get rgw role %s: %w", name, ErrRGWRoleNotFound)
		}
		return nil, fmt.Errorf("failed to get rgw role %s: %w", name, err)
	}

	var role RgwRoleInfo
	if err := json.Unmarshal(output, &role); err == nil && role.RoleName != "" {
		return &role, nil
	}

	var wrapped rgwRoleGetOutput
	if err := json.Unmarshal(output, &wrapped); err != nil {
		return nil, fmt.Errorf("failed to parse rgw role get output: %w", err)
	}

	return &wrapped.Role, nil
}

func (c *CLI) RGWRoleExists(ctx context.Context, accountID, name string) (bool, error) {
	_, err := c.RGWRoleGet(ctx, accountID, name)
	if err != nil {
		if errors.Is(err, ErrRGWRoleNotFound) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func (c *CLI) RGWRoleDelete(ctx context.Context, accountID, name string) error {
	args := []string{"--conf", c.confPath, "role", "delete", "--role-name=" + name}
	if accountID != "" {
		args = append(args, "--account-id="+accountID)
	}

	cmd := exec.CommandContext(ctx, "radosgw-admin", args...)
	if _, err := cmd.Output(); err != nil {
		return fmt.Errorf("failed to delete rgw role %s: %w", name, err)
	}

	return nil
}
