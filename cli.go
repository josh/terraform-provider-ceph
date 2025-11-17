package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

type CephCLI struct {
	confPath string
}

func NewCephCLI(confPath string) *CephCLI {
	return &CephCLI{confPath: confPath}
}

type CephAuthInfo struct {
	Key  string            `json:"key"`
	Caps map[string]string `json:"caps"`
}

func (c *CephCLI) AuthGet(ctx context.Context, entity string) (*CephAuthInfo, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "auth", "get", entity, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth for %s: %w", entity, err)
	}

	var authInfo []CephAuthInfo
	if err := json.Unmarshal(output, &authInfo); err != nil {
		return nil, fmt.Errorf("failed to parse auth output: %w", err)
	}

	if len(authInfo) == 0 {
		return nil, fmt.Errorf("no auth info found for entity %s", entity)
	}

	return &authInfo[0], nil
}

func (c *CephCLI) AuthSetCaps(ctx context.Context, entity string, caps map[string]string) error {
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

	return nil
}

func (c *CephCLI) ConfigSet(ctx context.Context, scope, key, value string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "config", "set", scope, key, value)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set config %s=%s for scope %s: %w", key, value, scope, err)
	}

	return nil
}

func (c *CephCLI) ConfigGet(ctx context.Context, scope, key string) (string, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "config", "get", scope, key)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get config %s for scope %s: %w", key, scope, err)
	}

	return strings.TrimSpace(string(output)), nil
}

func (c *CephCLI) ConfigRemove(ctx context.Context, scope, key string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "config", "rm", scope, key)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove config %s for scope %s: %w", key, scope, err)
	}

	return nil
}

type CephCrushRule struct {
	RuleName string `json:"rule_name"`
}

func (c *CephCLI) CrushRuleCreateReplicated(ctx context.Context, name, root, failureDomain string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "crush", "rule", "create-replicated", name, root, failureDomain)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create replicated crush rule %s: %w", name, err)
	}

	return nil
}

func (c *CephCLI) CrushRuleCreateSimple(ctx context.Context, name, root, failureDomain string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "crush", "rule", "create-simple", name, root, failureDomain)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create simple crush rule %s: %w", name, err)
	}

	return nil
}

func (c *CephCLI) CrushRuleCreateErasure(ctx context.Context, name, profile string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "crush", "rule", "create-erasure", name, profile)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create erasure crush rule %s: %w", name, err)
	}

	return nil
}

func (c *CephCLI) CrushRuleDump(ctx context.Context, name string) (*CephCrushRule, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "crush", "rule", "dump", name, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to dump crush rule %s: %w", name, err)
	}

	var rule CephCrushRule
	if err := json.Unmarshal(output, &rule); err != nil {
		return nil, fmt.Errorf("failed to parse crush rule output: %w", err)
	}

	return &rule, nil
}

func (c *CephCLI) CrushRuleList(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "crush", "rule", "ls", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list crush rules: %w", err)
	}

	var rules []string
	if err := json.Unmarshal(output, &rules); err != nil {
		return nil, fmt.Errorf("failed to parse crush rule list: %w", err)
	}

	return rules, nil
}

func (c *CephCLI) CrushRuleRemove(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "crush", "rule", "rm", name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove crush rule %s: %w", name, err)
	}

	return nil
}

func (c *CephCLI) ErasureCodeProfileSet(ctx context.Context, name string, params map[string]string) error {
	args := []string{"--conf", c.confPath, "osd", "erasure-code-profile", "set", name}

	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		args = append(args, fmt.Sprintf("%s=%s", key, params[key]))
	}

	cmd := exec.CommandContext(ctx, "ceph", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set erasure code profile %s: %w", name, err)
	}

	return nil
}

func (c *CephCLI) ErasureCodeProfileGet(ctx context.Context, name string) (map[string]string, error) {
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

func (c *CephCLI) ErasureCodeProfileList(ctx context.Context) ([]string, error) {
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

func (c *CephCLI) ErasureCodeProfileRemove(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "erasure-code-profile", "rm", name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove erasure code profile %s: %w", name, err)
	}

	return nil
}

type RgwS3Key struct {
	AccessKey string `json:"access_key"`
}

type RgwUserInfo struct {
	DisplayName string     `json:"display_name"`
	Email       string     `json:"email"`
	Suspended   int        `json:"suspended"`
	MaxBuckets  int        `json:"max_buckets"`
	Keys        []RgwS3Key `json:"keys"`
}

type RgwUserCreateOptions struct {
	AccessKey string
	SecretKey string
}

type RgwUserModifyOptions struct {
	DisplayName string
	MaxBuckets  *int
	Admin       *bool
}

type RgwSubuserCreateOptions struct {
	Access string
}

type RgwKeyCreateOptions struct {
	Subuser   string
	KeyType   string
	AccessKey string
	SecretKey string
}

func (c *CephCLI) RgwUserCreate(ctx context.Context, uid, displayName string, opts *RgwUserCreateOptions) (*RgwUserInfo, error) {
	args := []string{"--conf", c.confPath, "--format=json", "user", "create", "--uid=" + uid, "--display-name=" + displayName}

	if opts != nil {
		if opts.AccessKey != "" {
			args = append(args, "--access-key="+opts.AccessKey)
		}
		if opts.SecretKey != "" {
			args = append(args, "--secret-key="+opts.SecretKey)
		}
	}

	cmd := exec.CommandContext(ctx, "radosgw-admin", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to create rgw user %s: %w", uid, err)
	}

	var userInfo RgwUserInfo
	if err := json.Unmarshal(output, &userInfo); err != nil {
		return nil, fmt.Errorf("failed to parse rgw user create output: %w", err)
	}

	return &userInfo, nil
}

func (c *CephCLI) RgwUserInfo(ctx context.Context, uid string) (*RgwUserInfo, error) {
	cmd := exec.CommandContext(ctx, "radosgw-admin", "--conf", c.confPath, "--format=json", "user", "info", "--uid="+uid)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get rgw user info for %s: %w", uid, err)
	}

	var userInfo RgwUserInfo
	if err := json.Unmarshal(output, &userInfo); err != nil {
		return nil, fmt.Errorf("failed to parse rgw user info output: %w", err)
	}

	return &userInfo, nil
}

func (c *CephCLI) RgwUserModify(ctx context.Context, uid string, opts *RgwUserModifyOptions) (*RgwUserInfo, error) {
	args := []string{"--conf", c.confPath, "--format=json", "user", "modify", "--uid=" + uid}

	if opts != nil {
		if opts.DisplayName != "" {
			args = append(args, "--display-name="+opts.DisplayName)
		}
		if opts.MaxBuckets != nil {
			args = append(args, fmt.Sprintf("--max-buckets=%d", *opts.MaxBuckets))
		}
		if opts.Admin != nil {
			if *opts.Admin {
				args = append(args, "--admin")
			} else {
				args = append(args, "--admin=0")
			}
		}
	}

	cmd := exec.CommandContext(ctx, "radosgw-admin", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to modify rgw user %s: %w", uid, err)
	}

	var userInfo RgwUserInfo
	if err := json.Unmarshal(output, &userInfo); err != nil {
		return nil, fmt.Errorf("failed to parse rgw user modify output: %w", err)
	}

	return &userInfo, nil
}

func (c *CephCLI) RgwUserRemove(ctx context.Context, uid string, purgeData bool) error {
	args := []string{"--conf", c.confPath, "user", "rm", "--uid=" + uid}
	if purgeData {
		args = append(args, "--purge-data")
	}

	cmd := exec.CommandContext(ctx, "radosgw-admin", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove rgw user %s: %w", uid, err)
	}

	return nil
}

func (c *CephCLI) RgwUserSuspend(ctx context.Context, uid string, suspend bool) error {
	var subcommand string
	if suspend {
		subcommand = "suspend"
	} else {
		subcommand = "enable"
	}

	args := []string{"--conf", c.confPath, "user", subcommand, "--uid=" + uid}
	cmd := exec.CommandContext(ctx, "radosgw-admin", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to %s rgw user %s: %w", subcommand, uid, err)
	}

	return nil
}

func (c *CephCLI) RgwSubuserCreate(ctx context.Context, uid, subuser string, opts *RgwSubuserCreateOptions) (*RgwUserInfo, error) {
	args := []string{"--conf", c.confPath, "--format=json", "subuser", "create", "--uid=" + uid, "--subuser=" + subuser}

	if opts != nil {
		if opts.Access != "" {
			args = append(args, "--access="+opts.Access)
		}
	}

	cmd := exec.CommandContext(ctx, "radosgw-admin", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to create rgw subuser %s for %s: %w", subuser, uid, err)
	}

	var userInfo RgwUserInfo
	if err := json.Unmarshal(output, &userInfo); err != nil {
		return nil, fmt.Errorf("failed to parse rgw subuser create output: %w", err)
	}

	return &userInfo, nil
}

func (c *CephCLI) RgwKeyCreate(ctx context.Context, uid string, opts *RgwKeyCreateOptions) ([]RgwS3Key, error) {
	args := []string{"--conf", c.confPath, "--format=json", "key", "create", "--uid=" + uid}

	if opts != nil {
		if opts.Subuser != "" {
			args = append(args, "--subuser="+opts.Subuser)
		}
		if opts.KeyType != "" {
			args = append(args, "--key-type="+opts.KeyType)
		}
		if opts.AccessKey != "" {
			args = append(args, "--access-key="+opts.AccessKey)
		}
		if opts.SecretKey != "" {
			args = append(args, "--secret-key="+opts.SecretKey)
		}
	}

	cmd := exec.CommandContext(ctx, "radosgw-admin", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to create rgw key for %s: %w", uid, err)
	}

	var userInfo RgwUserInfo
	if err := json.Unmarshal(output, &userInfo); err != nil {
		return nil, fmt.Errorf("failed to parse rgw key create output: %w", err)
	}

	return userInfo.Keys, nil
}

func (c *CephCLI) RgwKeyRemove(ctx context.Context, uid, accessKey string) error {
	args := []string{"--conf", c.confPath, "key", "rm", "--uid=" + uid, "--access-key=" + accessKey}

	cmd := exec.CommandContext(ctx, "radosgw-admin", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove rgw key %s for %s: %w", accessKey, uid, err)
	}

	return nil
}

func (c *CephCLI) PoolCreate(ctx context.Context, poolName string, pgNum int, poolType string) error {
	args := []string{"--conf", c.confPath, "osd", "pool", "create", poolName, fmt.Sprintf("%d", pgNum)}
	if poolType != "" {
		args = append(args, poolType)
	}

	cmd := exec.CommandContext(ctx, "ceph", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create pool %s: %w", poolName, err)
	}

	return nil
}

func (c *CephCLI) PoolDelete(ctx context.Context, poolName string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "pool", "delete", poolName, poolName, "--yes-i-really-really-mean-it")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete pool %s: %w", poolName, err)
	}

	return nil
}

func (c *CephCLI) PoolGet(ctx context.Context, poolName, key string) (string, error) {
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

func (c *CephCLI) PoolSet(ctx context.Context, poolName, key, value string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "pool", "set", poolName, key, value)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set pool %s property %s=%s: %w", poolName, key, value, err)
	}

	return nil
}

func (c *CephCLI) PoolApplicationEnable(ctx context.Context, poolName, application string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "pool", "application", "enable", poolName, application)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable application %s on pool %s: %w", application, poolName, err)
	}

	return nil
}

type RgwBucketInfo struct {
	Bucket string `json:"bucket"`
	Owner  string `json:"owner"`
	ID     string `json:"id"`
}

func (c *CephCLI) RgwBucketInfo(ctx context.Context, bucket string) (*RgwBucketInfo, error) {
	cmd := exec.CommandContext(ctx, "radosgw-admin", "--conf", c.confPath, "--format=json", "bucket", "stats", "--bucket="+bucket)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get rgw bucket info for %s: %w", bucket, err)
	}

	var bucketInfo RgwBucketInfo
	if err := json.Unmarshal(output, &bucketInfo); err != nil {
		return nil, fmt.Errorf("failed to parse rgw bucket info output: %w", err)
	}

	return &bucketInfo, nil
}
