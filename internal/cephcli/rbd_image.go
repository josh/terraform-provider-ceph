package cephcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

var ErrRBDImageNotFound = errors.New("rbd image not found")

func rbdImageSpec(pool, namespace, name string) string {
	if namespace != "" {
		return pool + "/" + namespace + "/" + name
	}
	return pool + "/" + name
}

type RBDImageInfo struct {
	Name            string   `json:"name"`
	ID              string   `json:"id"`
	Size            int64    `json:"size"`
	ObjectSize      int64    `json:"object_size"`
	BlockNamePrefix string   `json:"block_name_prefix"`
	Features        []string `json:"features"`
}

func (c *CLI) RBDCreate(ctx context.Context, pool, namespace, name string, sizeMB int64) error {
	cmd := exec.CommandContext(ctx, "rbd", "--conf", c.confPath, "create", rbdImageSpec(pool, namespace, name), "--size", strconv.FormatInt(sizeMB, 10))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create rbd image %s/%s: %w", pool, name, err)
	}

	_, err := c.RBDInfo(ctx, pool, namespace, name)
	if err != nil {
		return fmt.Errorf("failed to verify rbd image creation: %w", err)
	}
	return nil
}

func (c *CLI) RBDInfo(ctx context.Context, pool, namespace, name string) (*RBDImageInfo, error) {
	cmd := exec.CommandContext(ctx, "rbd", "--conf", c.confPath, "info", rbdImageSpec(pool, namespace, name), "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if strings.Contains(stderr, "No such file or directory") {
				return nil, fmt.Errorf("failed to get rbd image info for %s/%s: %w", pool, name, ErrRBDImageNotFound)
			}
		}
		return nil, fmt.Errorf("failed to get rbd image info for %s/%s: %w", pool, name, err)
	}

	var info RBDImageInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, fmt.Errorf("failed to parse rbd image info output: %w", err)
	}

	return &info, nil
}

func (c *CLI) RBDExists(ctx context.Context, pool, namespace, name string) (bool, error) {
	_, err := c.RBDInfo(ctx, pool, namespace, name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrRBDImageNotFound) {
		return false, nil
	}
	return false, err
}

func (c *CLI) RBDRemove(ctx context.Context, pool, namespace, name string) error {
	cmd := exec.CommandContext(ctx, "rbd", "--conf", c.confPath, "rm", rbdImageSpec(pool, namespace, name))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove rbd image %s/%s: %w", pool, name, err)
	}

	exists, err := c.RBDExists(ctx, pool, namespace, name)
	if err != nil {
		return fmt.Errorf("failed to verify rbd image removal: %w", err)
	}
	if exists {
		return fmt.Errorf("rbd image still exists after removal: %s/%s", pool, name)
	}
	return nil
}
