package cephcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"
)

type PoolQuotaInfo struct {
	QuotaMaxObjects int64 `json:"quota_max_objects"`
	QuotaMaxBytes   int64 `json:"quota_max_bytes"`
}

func (c *CLI) PoolCreate(ctx context.Context, poolName string, pgNum int, poolType string) error {
	args := []string{"--conf", c.confPath, "osd", "pool", "create", poolName, fmt.Sprintf("%d", pgNum)}
	if poolType != "" {
		args = append(args, poolType)
	}

	cmd := exec.CommandContext(ctx, "ceph", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create pool %s: %w", poolName, err)
	}

	_, err := c.PoolGet(ctx, poolName, "size")
	if err != nil {
		return fmt.Errorf("failed to verify pool creation: %w", err)
	}
	return nil
}

func (c *CLI) PoolDelete(ctx context.Context, poolName string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "pool", "delete", poolName, poolName, "--yes-i-really-really-mean-it")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete pool %s: %w", poolName, err)
	}

	_, err := c.PoolGet(ctx, poolName, "size")
	if err == nil {
		return fmt.Errorf("pool still exists after deletion: %s", poolName)
	}
	return nil
}

func (c *CLI) PoolGet(ctx context.Context, poolName, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "pool", "get", poolName, key)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get pool %s property %s: %w", poolName, key, err)
	}

	text := strings.TrimSpace(string(output))
	prefix := key + ": "
	if !strings.HasPrefix(text, prefix) {
		return "", fmt.Errorf("unexpected output format: %s", text)
	}

	value := strings.TrimPrefix(text, prefix)
	return strings.TrimSpace(value), nil
}

func (c *CLI) PoolSet(ctx context.Context, poolName, key, value string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "pool", "set", poolName, key, value)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set pool %s property %s=%s: %w", poolName, key, value, err)
	}

	actualValue, err := c.PoolGet(ctx, poolName, key)
	if err != nil {
		return fmt.Errorf("failed to verify pool property: %w", err)
	}
	if actualValue != value {
		return fmt.Errorf("pool property %s not updated: expected %q, got %q", key, value, actualValue)
	}
	return nil
}

func (c *CLI) PoolSetWait(ctx context.Context, poolName, key, value string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "pool", "set", poolName, key, value)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set pool %s property %s=%s: %w", poolName, key, value, err)
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var lastValue string
	for {
		select {
		case <-ticker.C:
			actualValue, err := c.PoolGet(ctx, poolName, key)
			if err != nil {
				continue
			}
			lastValue = actualValue
			if actualValue == value {
				return nil
			}
		case <-ctx.Done():
			return fmt.Errorf("pool property %s not updated: expected %q, got %q: %w", key, value, lastValue, ctx.Err())
		}
	}
}

func (c *CLI) PoolSetQuota(ctx context.Context, poolName, field string, value int64) error {
	valueStr := strconv.FormatInt(value, 10)
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "pool", "set-quota", poolName, field, valueStr)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set pool %s quota %s=%v: %w", poolName, field, value, err)
	}

	actualValue, err := c.PoolGetQuota(ctx, poolName, field)
	if err != nil {
		return fmt.Errorf("failed to verify pool quota: %w", err)
	}

	if value != actualValue {
		return fmt.Errorf("pool quota %s not updated: expected %v, got %v", field, value, actualValue)
	}
	return nil
}

func (c *CLI) PoolGetQuota(ctx context.Context, poolName, field string) (int64, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "pool", "get-quota", poolName, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get pool %s quota: %w", poolName, err)
	}

	var quotaInfo PoolQuotaInfo
	if err := json.Unmarshal(output, &quotaInfo); err != nil {
		return 0, fmt.Errorf("failed to parse quota JSON: %w", err)
	}

	switch field {
	case "max_objects":
		return quotaInfo.QuotaMaxObjects, nil
	case "max_bytes":
		return quotaInfo.QuotaMaxBytes, nil
	default:
		return 0, fmt.Errorf("unknown quota field: %s", field)
	}
}

func (c *CLI) PoolApplicationGet(ctx context.Context, poolName string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "pool", "application", "get", poolName, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get pool %s applications: %w", poolName, err)
	}

	var apps map[string]any
	if err := json.Unmarshal(output, &apps); err != nil {
		return nil, fmt.Errorf("failed to parse pool applications: %w", err)
	}

	result := make([]string, 0, len(apps))
	for app := range apps {
		result = append(result, app)
	}
	return result, nil
}

func (c *CLI) PoolApplicationEnable(ctx context.Context, poolName, application string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "pool", "application", "enable", poolName, application)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable application %s on pool %s: %w", application, poolName, err)
	}

	apps, err := c.PoolApplicationGet(ctx, poolName)
	if err != nil {
		return fmt.Errorf("failed to verify application was enabled: %w", err)
	}

	if slices.Contains(apps, application) {
		return nil
	}
	return fmt.Errorf("application %s not found in pool %s applications after enabling", application, poolName)
}

func (c *CLI) PoolExists(ctx context.Context, poolName string) (bool, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "pool", "get", poolName, "size")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			stderr := strings.ToLower(string(output))
			if strings.Contains(stderr, "error enoent") || strings.Contains(stderr, "pool does not exist") || strings.Contains(stderr, "unrecognized pool") {
				return false, nil
			}
		}
		return false, fmt.Errorf("failed to check pool existence: %w", err)
	}
	return true, nil
}
