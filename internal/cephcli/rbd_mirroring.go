package cephcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type RBDMirrorPoolStatus struct {
	Mode     string          `json:"mode"`
	SiteName string          `json:"site_name,omitempty"`
	Peers    []RBDMirrorPeer `json:"peers"`
}

func (c *CLI) RBDMirrorPoolInfo(ctx context.Context, pool string) (*RBDMirrorPoolStatus, error) {
	cmd := exec.CommandContext(ctx, "rbd", "--conf", c.confPath, "mirror", "pool", "info", pool, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get rbd mirror pool info for %s: %w", pool, err)
	}

	var info RBDMirrorPoolStatus
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, fmt.Errorf("failed to parse rbd mirror pool info output: %w", err)
	}

	return &info, nil
}

func (c *CLI) RBDMirrorPoolMode(ctx context.Context, pool string) (string, error) {
	info, err := c.RBDMirrorPoolInfo(ctx, pool)
	if err != nil {
		return "", err
	}
	return info.Mode, nil
}

func (c *CLI) RBDMirrorPoolEnable(ctx context.Context, pool, mode string) error {
	cmd := exec.CommandContext(ctx, "rbd", "--conf", c.confPath, "mirror", "pool", "enable", pool, mode)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable rbd mirroring on pool %s: %w, output: %s", pool, err, string(output))
	}

	current, err := c.RBDMirrorPoolMode(ctx, pool)
	if err != nil {
		return fmt.Errorf("failed to verify rbd mirroring mode: %w", err)
	}
	if current != mode {
		return fmt.Errorf("rbd mirroring mode on pool %s is %q after enabling %q", pool, current, mode)
	}
	return nil
}

func (c *CLI) RBDMirrorPoolDisable(ctx context.Context, pool string) error {
	cmd := exec.CommandContext(ctx, "rbd", "--conf", c.confPath, "mirror", "pool", "disable", pool)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to disable rbd mirroring on pool %s: %w, output: %s", pool, err, string(output))
	}

	current, err := c.RBDMirrorPoolMode(ctx, pool)
	if err != nil {
		return fmt.Errorf("failed to verify rbd mirroring mode: %w", err)
	}
	if current != "disabled" {
		return fmt.Errorf("rbd mirroring mode on pool %s is %q after disabling", pool, current)
	}
	return nil
}

type RBDMirrorPeer struct {
	UUID       string `json:"uuid"`
	Direction  string `json:"direction"`
	SiteName   string `json:"site_name"`
	MirrorUUID string `json:"mirror_uuid"`
	ClientName string `json:"client_name"`
}

func (c *CLI) RBDMirrorPoolPeerAdd(ctx context.Context, pool, clientName, clusterName string) (string, error) {
	cmd := exec.CommandContext(ctx, "rbd", "--conf", c.confPath, "mirror", "pool", "peer", "add", pool, clientName+"@"+clusterName)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("failed to add rbd mirror peer on pool %s: %w, stderr: %s", pool, err, string(exitErr.Stderr))
		}
		return "", fmt.Errorf("failed to add rbd mirror peer on pool %s: %w", pool, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *CLI) RBDMirrorPoolPeerRemove(ctx context.Context, pool, uuid string) error {
	cmd := exec.CommandContext(ctx, "rbd", "--conf", c.confPath, "mirror", "pool", "peer", "remove", pool, uuid)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove rbd mirror peer %s on pool %s: %w, output: %s", uuid, pool, err, string(output))
	}
	return nil
}
