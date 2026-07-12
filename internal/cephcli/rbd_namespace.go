package cephcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

type RBDNamespaceInfo struct {
	Name string `json:"name"`
}

func (c *CLI) RBDNamespaceList(ctx context.Context, pool string) ([]RBDNamespaceInfo, error) {
	cmd := exec.CommandContext(ctx, "rbd", "--conf", c.confPath, "namespace", "ls", pool, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list rbd namespaces in pool %s: %w", pool, err)
	}

	var namespaces []RBDNamespaceInfo
	if err := json.Unmarshal(output, &namespaces); err != nil {
		return nil, fmt.Errorf("failed to parse rbd namespace list output: %w", err)
	}

	return namespaces, nil
}

func (c *CLI) RBDNamespaceExists(ctx context.Context, pool, namespace string) (bool, error) {
	namespaces, err := c.RBDNamespaceList(ctx, pool)
	if err != nil {
		return false, err
	}

	for _, ns := range namespaces {
		if ns.Name == namespace {
			return true, nil
		}
	}
	return false, nil
}

func (c *CLI) RBDNamespaceCreate(ctx context.Context, pool, namespace string) error {
	cmd := exec.CommandContext(ctx, "rbd", "--conf", c.confPath, "namespace", "create", pool+"/"+namespace)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create rbd namespace %s/%s: %w", pool, namespace, err)
	}

	exists, err := c.RBDNamespaceExists(ctx, pool, namespace)
	if err != nil {
		return fmt.Errorf("failed to verify rbd namespace creation: %w", err)
	}
	if !exists {
		return fmt.Errorf("rbd namespace not found after creation: %s/%s", pool, namespace)
	}
	return nil
}

func (c *CLI) RBDNamespaceRemove(ctx context.Context, pool, namespace string) error {
	cmd := exec.CommandContext(ctx, "rbd", "--conf", c.confPath, "namespace", "remove", pool+"/"+namespace)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove rbd namespace %s/%s: %w", pool, namespace, err)
	}

	exists, err := c.RBDNamespaceExists(ctx, pool, namespace)
	if err != nil {
		return fmt.Errorf("failed to verify rbd namespace removal: %w", err)
	}
	if exists {
		return fmt.Errorf("rbd namespace still exists after removal: %s/%s", pool, namespace)
	}
	return nil
}
