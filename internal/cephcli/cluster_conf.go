package cephcli

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
)

type ConfigDumpEntry struct {
	Section string `json:"section"`
	Mask    string `json:"mask"`
	Name    string `json:"name"`
	Value   string `json:"value"`
}

const floatComparisonEpsilon = 1e-9

func (c *CLI) ConfigSet(ctx context.Context, scope, key, value string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "config", "set", scope, key, value)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set config %s=%s for scope %s: %w", key, value, scope, err)
	}

	actualValue, err := c.ConfigGetFromDump(ctx, scope, key)
	if err != nil {
		return fmt.Errorf("failed to verify config: %w", err)
	}

	if value == actualValue {
		return nil
	}

	expectedFloat, expectedErr := strconv.ParseFloat(value, 64)
	actualFloat, actualErr := strconv.ParseFloat(actualValue, 64)
	if expectedErr == nil && actualErr == nil && math.Abs(expectedFloat-actualFloat) < floatComparisonEpsilon {
		return nil
	}

	return fmt.Errorf("config verification failed: expected %q, got %q", value, actualValue)
}

func (c *CLI) ConfigGet(ctx context.Context, scope, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "config", "get", scope, key)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get config %s for scope %s: %w", key, scope, err)
	}

	return strings.TrimSpace(string(output)), nil
}

func (c *CLI) ConfigKeyGet(ctx context.Context, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "config-key", "get", key)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get config-key %s: %w", key, err)
	}

	return strings.TrimSpace(string(output)), nil
}

func (c *CLI) ConfigGetFromDump(ctx context.Context, scope, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "config", "dump", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to dump config: %w", err)
	}

	var entries []ConfigDumpEntry
	if err := json.Unmarshal(output, &entries); err != nil {
		return "", fmt.Errorf("failed to parse config dump: %w", err)
	}

	parts := strings.SplitN(scope, "/", 2)
	section := parts[0]
	mask := ""
	if len(parts) > 1 {
		mask = parts[1]
	}

	for _, entry := range entries {
		if entry.Section == section && entry.Name == key && entry.Mask == mask {
			return entry.Value, nil
		}
	}

	return "", fmt.Errorf("config %s not found for scope %s", key, scope)
}

func (c *CLI) ConfigRemove(ctx context.Context, scope, key string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "config", "rm", scope, key)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove config %s for scope %s: %w", key, scope, err)
	}

	_, err := c.ConfigGetFromDump(ctx, scope, key)
	if err == nil {
		return fmt.Errorf("config still exists after removal: %s for scope %s", key, scope)
	}
	return nil
}

func (c *CLI) ConfigDump(ctx context.Context) ([]ConfigDumpEntry, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "config", "dump", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to dump config: %w", err)
	}

	var entries []ConfigDumpEntry
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse config dump: %w", err)
	}

	return entries, nil
}
