package cephcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
)

func (c *CLI) ErasureCodeProfileSet(ctx context.Context, name string, params map[string]string) error {
	return c.erasureCodeProfileSet(ctx, name, params, false)
}

// ErasureCodeProfileSetForce overwrites an existing profile in place, which
// Ceph otherwise refuses (-EPERM); the flags are required to simulate
// out-of-band mutation of a profile that is already present.
func (c *CLI) ErasureCodeProfileSetForce(ctx context.Context, name string, params map[string]string) error {
	return c.erasureCodeProfileSet(ctx, name, params, true)
}

func (c *CLI) erasureCodeProfileSet(ctx context.Context, name string, params map[string]string, force bool) error {
	args := []string{"--conf", c.confPath, "osd", "erasure-code-profile", "set", name}

	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		args = append(args, fmt.Sprintf("%s=%s", key, params[key]))
	}

	if force {
		args = append(args, "--force", "--yes-i-really-mean-it")
	}

	cmd := exec.CommandContext(ctx, "ceph", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set erasure code profile %s: %w", name, err)
	}

	profile, err := c.ErasureCodeProfileGet(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to verify erasure code profile: %w", err)
	}
	for key, expectedValue := range params {
		actualValue, exists := profile[key]
		if !exists {
			return fmt.Errorf("profile missing key %s", key)
		}
		if actualValue != expectedValue {
			return fmt.Errorf("profile key %s: expected %q, got %q", key, expectedValue, actualValue)
		}
	}
	return nil
}

func (c *CLI) ErasureCodeProfileGet(ctx context.Context, name string) (map[string]string, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "erasure-code-profile", "get", name, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get erasure code profile %s: %w", name, err)
	}

	var profile map[string]string
	if err := json.Unmarshal(output, &profile); err != nil {
		return nil, fmt.Errorf("failed to parse erasure code profile output: %w", err)
	}

	return profile, nil
}

func (c *CLI) ErasureCodeProfileList(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "erasure-code-profile", "ls", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list erasure code profiles: %w", err)
	}

	var profiles []string
	if err := json.Unmarshal(output, &profiles); err != nil {
		return nil, fmt.Errorf("failed to parse erasure code profile list: %w", err)
	}

	return profiles, nil
}

func (c *CLI) ErasureCodeProfileRemove(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "erasure-code-profile", "rm", name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove erasure code profile %s: %w", name, err)
	}

	_, err := c.ErasureCodeProfileGet(ctx, name)
	if err == nil {
		return fmt.Errorf("erasure code profile still exists after removal: %s", name)
	}
	return nil
}
