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

func (c *CLI) CephFSAddDataPool(ctx context.Context, volName string, poolName string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "fs", "add_data_pool", volName, poolName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add data pool %s to CephFS %s: %w, output: %s", poolName, volName, err, string(output))
	}
	return nil
}

func (c *CLI) CephFSRemoveDataPool(ctx context.Context, volName string, poolName string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "fs", "rm_data_pool", volName, poolName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove data pool %s from CephFS %s: %w, output: %s", poolName, volName, err, string(output))
	}
	return nil
}

func groupNameArgs(groupName string) []string {
	if groupName == "" {
		return nil
	}
	return []string{"--group_name", groupName}
}

func (c *CLI) CephFSSubvolumeCreate(ctx context.Context, volName string, subvolName string, groupName string) error {
	cmd := exec.CommandContext(ctx, "ceph", append([]string{"--conf", c.confPath, "fs", "subvolume", "create", volName, subvolName}, groupNameArgs(groupName)...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create CephFS subvolume %s/%s: %w, output: %s", volName, subvolName, err, string(output))
	}
	return nil
}

func (c *CLI) CephFSSubvolumeDelete(ctx context.Context, volName string, subvolName string, groupName string) error {
	cmd := exec.CommandContext(ctx, "ceph", append([]string{"--conf", c.confPath, "fs", "subvolume", "rm", volName, subvolName}, groupNameArgs(groupName)...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete CephFS subvolume %s/%s: %w, output: %s", volName, subvolName, err, string(output))
	}
	return nil
}

type CephFSSubvolumeInfoEntry struct {
	Path          string `json:"path"`
	BytesQuota    any    `json:"bytes_quota"`
	Mode          int    `json:"mode"`
	UID           int    `json:"uid"`
	GID           int    `json:"gid"`
	DataPool      string `json:"data_pool"`
	PoolNamespace string `json:"pool_namespace"`
	Earmark       string `json:"earmark"`
}

func (e *CephFSSubvolumeInfoEntry) BytesQuotaInt64() (int64, bool) {
	switch v := e.BytesQuota.(type) {
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

func (c *CLI) CephFSSubvolumeInfo(ctx context.Context, volName string, subvolName string, groupName string) (*CephFSSubvolumeInfoEntry, error) {
	cmd := exec.CommandContext(ctx, "ceph", append(append([]string{"--conf", c.confPath, "fs", "subvolume", "info", volName, subvolName}, groupNameArgs(groupName)...), "--format", "json")...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get CephFS subvolume info for %s/%s: %w", volName, subvolName, err)
	}

	var info CephFSSubvolumeInfoEntry
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, fmt.Errorf("failed to parse CephFS subvolume info: %w", err)
	}

	return &info, nil
}

func (c *CLI) CephFSSubvolumeResize(ctx context.Context, volName string, subvolName string, newSize string, groupName string) error {
	cmd := exec.CommandContext(ctx, "ceph", append([]string{"--conf", c.confPath, "fs", "subvolume", "resize", volName, subvolName, newSize}, groupNameArgs(groupName)...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to resize CephFS subvolume %s/%s: %w, output: %s", volName, subvolName, err, string(output))
	}
	return nil
}

type CephFSSubvolumeListEntry struct {
	Name string `json:"name"`
}

func (c *CLI) CephFSSubvolumeList(ctx context.Context, volName string, groupName string) ([]CephFSSubvolumeListEntry, error) {
	cmd := exec.CommandContext(ctx, "ceph", append(append([]string{"--conf", c.confPath, "fs", "subvolume", "ls", volName}, groupNameArgs(groupName)...), "--format", "json")...)
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

func (c *CLI) CephFSSubvolumeExists(ctx context.Context, volName string, subvolName string, groupName string) (bool, error) {
	entries, err := c.CephFSSubvolumeList(ctx, volName, groupName)
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

func (c *CLI) CephFSSubvolumeSnapshotCreate(ctx context.Context, volName string, subvolName string, snapName string, groupName string) error {
	cmd := exec.CommandContext(ctx, "ceph", append([]string{"--conf", c.confPath, "fs", "subvolume", "snapshot", "create", volName, subvolName, snapName}, groupNameArgs(groupName)...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create CephFS subvolume snapshot %s/%s/%s: %w, output: %s", volName, subvolName, snapName, err, string(output))
	}
	return nil
}

func (c *CLI) CephFSSubvolumeSnapshotDelete(ctx context.Context, volName string, subvolName string, snapName string, groupName string) error {
	cmd := exec.CommandContext(ctx, "ceph", append([]string{"--conf", c.confPath, "fs", "subvolume", "snapshot", "rm", volName, subvolName, snapName}, groupNameArgs(groupName)...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete CephFS subvolume snapshot %s/%s/%s: %w, output: %s", volName, subvolName, snapName, err, string(output))
	}
	return nil
}

type CephFSSubvolumeSnapshotListEntry struct {
	Name string `json:"name"`
}

func (c *CLI) CephFSSubvolumeSnapshotList(ctx context.Context, volName string, subvolName string, groupName string) ([]CephFSSubvolumeSnapshotListEntry, error) {
	cmd := exec.CommandContext(ctx, "ceph", append(append([]string{"--conf", c.confPath, "fs", "subvolume", "snapshot", "ls", volName, subvolName}, groupNameArgs(groupName)...), "--format", "json")...)
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

func (c *CLI) CephFSSubvolumeSnapshotExists(ctx context.Context, volName string, subvolName string, snapName string, groupName string) (bool, error) {
	entries, err := c.CephFSSubvolumeSnapshotList(ctx, volName, subvolName, groupName)
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

type CephFSSubvolumeGroupSnapshotListEntry struct {
	Name string `json:"name"`
}

func (c *CLI) CephFSSubvolumeGroupSnapshotList(ctx context.Context, volName string, groupName string) ([]CephFSSubvolumeGroupSnapshotListEntry, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "fs", "subvolumegroup", "snapshot", "ls", volName, groupName, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list CephFS subvolume group snapshots for %s/%s: %w", volName, groupName, err)
	}

	var entries []CephFSSubvolumeGroupSnapshotListEntry
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse CephFS subvolume group snapshot list: %w", err)
	}

	return entries, nil
}

func (c *CLI) CephFSSubvolumeGroupSnapshotExists(ctx context.Context, volName string, groupName string, snapName string) (bool, error) {
	entries, err := c.CephFSSubvolumeGroupSnapshotList(ctx, volName, groupName)
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

func (c *CLI) CephFSSubvolumeGroupSnapshotDelete(ctx context.Context, volName string, groupName string, snapName string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "fs", "subvolumegroup", "snapshot", "rm", volName, groupName, snapName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete CephFS subvolume group snapshot %s/%s/%s: %w, output: %s", volName, groupName, snapName, err, string(output))
	}
	return nil
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
