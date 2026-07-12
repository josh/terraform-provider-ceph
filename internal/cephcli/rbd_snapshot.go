package cephcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type RBDSnapInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Protected string `json:"protected"`
	Timestamp string `json:"timestamp"`
}

func (c *CLI) RBDSnapList(ctx context.Context, pool, image string) ([]RBDSnapInfo, error) {
	cmd := exec.CommandContext(ctx, "rbd", "--conf", c.confPath, "snap", "ls", pool+"/"+image, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if strings.Contains(stderr, "No such file or directory") {
				return nil, fmt.Errorf("failed to list rbd snapshots for %s/%s: %w", pool, image, ErrRBDImageNotFound)
			}
		}
		return nil, fmt.Errorf("failed to list rbd snapshots for %s/%s: %w", pool, image, err)
	}

	var snaps []RBDSnapInfo
	if err := json.Unmarshal(output, &snaps); err != nil {
		return nil, fmt.Errorf("failed to parse rbd snapshot list output: %w", err)
	}

	return snaps, nil
}

func (c *CLI) RBDSnapExists(ctx context.Context, pool, image, snap string) (bool, error) {
	snaps, err := c.RBDSnapList(ctx, pool, image)
	if err != nil {
		if errors.Is(err, ErrRBDImageNotFound) {
			return false, nil
		}
		return false, err
	}

	for _, s := range snaps {
		if s.Name == snap {
			return true, nil
		}
	}
	return false, nil
}

func (c *CLI) RBDSnapCreate(ctx context.Context, pool, image, snap string) error {
	cmd := exec.CommandContext(ctx, "rbd", "--conf", c.confPath, "snap", "create", pool+"/"+image+"@"+snap)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create rbd snapshot %s/%s@%s: %w, output: %s", pool, image, snap, err, string(output))
	}
	return nil
}

func (c *CLI) RBDSnapRemove(ctx context.Context, pool, image, snap string) error {
	cmd := exec.CommandContext(ctx, "rbd", "--conf", c.confPath, "snap", "rm", pool+"/"+image+"@"+snap)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove rbd snapshot %s/%s@%s: %w, output: %s", pool, image, snap, err, string(output))
	}
	return nil
}

func (c *CLI) RBDSnapProtect(ctx context.Context, pool, image, snap string) error {
	cmd := exec.CommandContext(ctx, "rbd", "--conf", c.confPath, "snap", "protect", pool+"/"+image+"@"+snap)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to protect rbd snapshot %s/%s@%s: %w, output: %s", pool, image, snap, err, string(output))
	}
	return nil
}

func (c *CLI) RBDSnapUnprotect(ctx context.Context, pool, image, snap string) error {
	cmd := exec.CommandContext(ctx, "rbd", "--conf", c.confPath, "snap", "unprotect", pool+"/"+image+"@"+snap)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to unprotect rbd snapshot %s/%s@%s: %w, output: %s", pool, image, snap, err, string(output))
	}
	return nil
}
