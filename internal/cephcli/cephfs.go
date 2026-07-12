package cephcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

type CephFSListEntry struct {
	Name           string   `json:"name"`
	MetadataPool   string   `json:"metadata_pool"`
	MetadataPoolID int      `json:"metadata_pool_id"`
	DataPools      []string `json:"data_pools"`
	DataPoolIDs    []int    `json:"data_pool_ids"`
}

func (c *CLI) CephFSList(ctx context.Context) ([]CephFSListEntry, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "fs", "ls", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list CephFS filesystems: %w", err)
	}

	var filesystems []CephFSListEntry
	if err := json.Unmarshal(output, &filesystems); err != nil {
		return nil, fmt.Errorf("failed to parse CephFS list output: %w", err)
	}

	return filesystems, nil
}

func (c *CLI) CephFSExists(ctx context.Context, name string) (bool, error) {
	filesystems, err := c.CephFSList(ctx)
	if err != nil {
		return false, err
	}

	for _, fs := range filesystems {
		if fs.Name == name {
			return true, nil
		}
	}

	return false, nil
}

func (c *CLI) CephFSVolumeCreate(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "fs", "volume", "create", name)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create CephFS volume %s: %w, output: %s", name, err, string(output))
	}
	return nil
}

func (c *CLI) CephFSVolumeDelete(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "fs", "volume", "rm", name, "--yes-i-really-mean-it")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete CephFS volume %s: %w, output: %s", name, err, string(output))
	}
	return nil
}

func (c *CLI) CephFSSubvolumeCreate(ctx context.Context, volName string, subvolName string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "fs", "subvolume", "create", volName, subvolName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create CephFS subvolume %s/%s: %w, output: %s", volName, subvolName, err, string(output))
	}
	return nil
}

func (c *CLI) CephFSSubvolumeDelete(ctx context.Context, volName string, subvolName string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "fs", "subvolume", "rm", volName, subvolName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete CephFS subvolume %s/%s: %w, output: %s", volName, subvolName, err, string(output))
	}
	return nil
}

type CephFSSubvolumeListEntry struct {
	Name string `json:"name"`
}

func (c *CLI) CephFSSubvolumeList(ctx context.Context, volName string) ([]CephFSSubvolumeListEntry, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "fs", "subvolume", "ls", volName, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list CephFS subvolumes for %s: %w", volName, err)
	}

	var entries []CephFSSubvolumeListEntry
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse CephFS subvolume list: %w", err)
	}

	return entries, nil
}

func (c *CLI) CephFSSubvolumeExists(ctx context.Context, volName string, subvolName string) (bool, error) {
	entries, err := c.CephFSSubvolumeList(ctx, volName)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if entry.Name == subvolName {
			return true, nil
		}
	}

	return false, nil
}

func (c *CLI) CephFSSubvolumeSnapshotCreate(ctx context.Context, volName string, subvolName string, snapName string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "fs", "subvolume", "snapshot", "create", volName, subvolName, snapName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create CephFS subvolume snapshot %s/%s/%s: %w, output: %s", volName, subvolName, snapName, err, string(output))
	}
	return nil
}

func (c *CLI) CephFSSubvolumeSnapshotDelete(ctx context.Context, volName string, subvolName string, snapName string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "fs", "subvolume", "snapshot", "rm", volName, subvolName, snapName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete CephFS subvolume snapshot %s/%s/%s: %w, output: %s", volName, subvolName, snapName, err, string(output))
	}
	return nil
}

type CephFSSubvolumeSnapshotListEntry struct {
	Name string `json:"name"`
}

func (c *CLI) CephFSSubvolumeSnapshotList(ctx context.Context, volName string, subvolName string) ([]CephFSSubvolumeSnapshotListEntry, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "fs", "subvolume", "snapshot", "ls", volName, subvolName, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list CephFS subvolume snapshots for %s/%s: %w", volName, subvolName, err)
	}

	var entries []CephFSSubvolumeSnapshotListEntry
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse CephFS subvolume snapshot list: %w", err)
	}

	return entries, nil
}

func (c *CLI) CephFSSubvolumeSnapshotExists(ctx context.Context, volName string, subvolName string, snapName string) (bool, error) {
	entries, err := c.CephFSSubvolumeSnapshotList(ctx, volName, subvolName)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if entry.Name == snapName {
			return true, nil
		}
	}

	return false, nil
}

func (c *CLI) CephFSSubvolumeGroupCreate(ctx context.Context, volName string, groupName string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "fs", "subvolumegroup", "create", volName, groupName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create CephFS subvolume group %s/%s: %w, output: %s", volName, groupName, err, string(output))
	}
	return nil
}

func (c *CLI) CephFSSubvolumeGroupDelete(ctx context.Context, volName string, groupName string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "fs", "subvolumegroup", "rm", volName, groupName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete CephFS subvolume group %s/%s: %w, output: %s", volName, groupName, err, string(output))
	}
	return nil
}

type CephFSSubvolumeGroupListEntry struct {
	Name string `json:"name"`
}

func (c *CLI) CephFSSubvolumeGroupList(ctx context.Context, volName string) ([]CephFSSubvolumeGroupListEntry, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "fs", "subvolumegroup", "ls", volName, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list CephFS subvolume groups for %s: %w", volName, err)
	}

	var entries []CephFSSubvolumeGroupListEntry
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse CephFS subvolume group list: %w", err)
	}

	return entries, nil
}

func (c *CLI) CephFSSubvolumeGroupExists(ctx context.Context, volName string, groupName string) (bool, error) {
	entries, err := c.CephFSSubvolumeGroupList(ctx, volName)
	if err != nil {
		return false, err
	}

	for _, entry := range entries {
		if entry.Name == groupName {
			return true, nil
		}
	}

	return false, nil
}
