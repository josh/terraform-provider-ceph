package cephcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"sort"
)

type AuthInfo struct {
	Key  string            `json:"key"`
	Caps map[string]string `json:"caps"`
}

func (c *CLI) AuthGet(ctx context.Context, entity string) (*AuthInfo, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "auth", "get", entity, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth for %s: %w", entity, err)
	}

	var authInfo []AuthInfo
	if err := json.Unmarshal(output, &authInfo); err != nil {
		return nil, fmt.Errorf("failed to parse auth output: %w", err)
	}

	if len(authInfo) == 0 {
		return nil, fmt.Errorf("no auth info found for entity %s", entity)
	}

	return &authInfo[0], nil
}

func (c *CLI) AuthSetCaps(ctx context.Context, entity string, caps map[string]string) error {
	args := []string{"--conf", c.confPath, "auth", "caps", entity}

	capTypes := make([]string, 0, len(caps))
	for capType := range caps {
		capTypes = append(capTypes, capType)
	}
	sort.Strings(capTypes)

	for _, capType := range capTypes {
		args = append(args, capType, caps[capType])
	}

	cmd := exec.CommandContext(ctx, "ceph", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set caps for %s: %w", entity, err)
	}

	authInfo, err := c.AuthGet(ctx, entity)
	if err != nil {
		return fmt.Errorf("failed to verify caps: %w", err)
	}
	if !reflect.DeepEqual(caps, authInfo.Caps) {
		return fmt.Errorf("caps verification failed: expected %v, got %v", caps, authInfo.Caps)
	}
	return nil
}

func (c *CLI) AuthDel(ctx context.Context, entity string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "auth", "del", entity)
	_, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("ceph auth del failed: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("ceph auth del failed: %w", err)
	}
	return nil
}
