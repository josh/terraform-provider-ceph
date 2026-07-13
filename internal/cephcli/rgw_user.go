package cephcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var ErrRGWUserNotFound = errors.New("rgw user not found")

type RgwS3Key struct {
	User      string `json:"user"`
	AccessKey string `json:"access_key"`
}

type RgwUserInfo struct {
	DisplayName string       `json:"display_name"`
	Email       string       `json:"email"`
	Suspended   int          `json:"suspended"`
	MaxBuckets  int          `json:"max_buckets"`
	Keys        []RgwS3Key   `json:"keys"`
	Admin       bool         `json:"admin"`
	UserQuota   RgwQuotaInfo `json:"user_quota"`
	BucketQuota RgwQuotaInfo `json:"bucket_quota"`
	Caps        []RgwUserCap `json:"caps"`
}

type RgwUserCap struct {
	Type string `json:"type"`
	Perm string `json:"perm"`
}

type RgwQuotaInfo struct {
	Enabled    bool  `json:"enabled"`
	MaxSize    int64 `json:"max_size"`
	MaxSizeKB  int64 `json:"max_size_kb"`
	MaxObjects int64 `json:"max_objects"`
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

func (c *CLI) RgwUserCreate(ctx context.Context, uid, displayName string, opts *RgwUserCreateOptions) error {
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
		return fmt.Errorf("failed to create rgw user %s: %w", uid, err)
	}

	var userInfo RgwUserInfo
	if err := json.Unmarshal(output, &userInfo); err != nil {
		return fmt.Errorf("failed to parse rgw user create output: %w", err)
	}

	_, err = c.RgwUserInfo(ctx, uid)
	if err != nil {
		return fmt.Errorf("failed to verify user creation: %w", err)
	}

	return nil
}

func (c *CLI) RgwUserInfo(ctx context.Context, uid string) (*RgwUserInfo, error) {
	cmd := exec.CommandContext(ctx, "radosgw-admin", "--conf", c.confPath, "--format=json", "user", "info", "--uid="+uid)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if strings.Contains(stderr, "could not fetch user info") {
				return nil, fmt.Errorf("failed to get rgw user info for %s: %w", uid, ErrRGWUserNotFound)
			}
		}
		return nil, fmt.Errorf("failed to get rgw user info for %s: %w", uid, err)
	}

	var userInfo RgwUserInfo
	if err := json.Unmarshal(output, &userInfo); err != nil {
		return nil, fmt.Errorf("failed to parse rgw user info output: %w", err)
	}

	return &userInfo, nil
}

func (c *CLI) RgwUserModify(ctx context.Context, uid string, opts *RgwUserModifyOptions) error {
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
		return fmt.Errorf("failed to modify rgw user %s: %w", uid, err)
	}

	var userInfo RgwUserInfo
	if err := json.Unmarshal(output, &userInfo); err != nil {
		return fmt.Errorf("failed to parse rgw user modify output: %w", err)
	}

	verifiedInfo, err := c.RgwUserInfo(ctx, uid)
	if err != nil {
		return fmt.Errorf("failed to verify user modification: %w", err)
	}
	if opts != nil {
		if opts.DisplayName != "" && verifiedInfo.DisplayName != opts.DisplayName {
			return fmt.Errorf("display name not updated: expected %q, got %q", opts.DisplayName, verifiedInfo.DisplayName)
		}
		if opts.MaxBuckets != nil && verifiedInfo.MaxBuckets != *opts.MaxBuckets {
			return fmt.Errorf("max buckets not updated: expected %d, got %d", *opts.MaxBuckets, verifiedInfo.MaxBuckets)
		}
		if opts.Admin != nil && verifiedInfo.Admin != *opts.Admin {
			return fmt.Errorf("admin flag not updated: expected %v, got %v", *opts.Admin, verifiedInfo.Admin)
		}
	}

	return nil
}

func (c *CLI) RgwUserRemove(ctx context.Context, uid string, purgeData bool) error {
	args := []string{"--conf", c.confPath, "user", "rm", "--uid=" + uid}
	if purgeData {
		args = append(args, "--purge-data")
	}

	cmd := exec.CommandContext(ctx, "radosgw-admin", args...)
	_, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if strings.Contains(stderr, "user does not exist") {
				return fmt.Errorf("failed to remove rgw user %s: %w", uid, ErrRGWUserNotFound)
			}
		}
		return fmt.Errorf("failed to remove rgw user %s: %w", uid, err)
	}

	_, err = c.RgwUserInfo(ctx, uid)
	if err == nil {
		return fmt.Errorf("user still exists after removal: %s", uid)
	}
	if errors.Is(err, ErrRGWUserNotFound) {
		return nil
	}
	return fmt.Errorf("unexpected error verifying user removal: %w", err)
}

func (c *CLI) RgwUserSuspend(ctx context.Context, uid string, suspend bool) error {
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

	userInfo, err := c.RgwUserInfo(ctx, uid)
	if err != nil {
		return fmt.Errorf("failed to verify user suspension state: %w", err)
	}
	expectedSuspended := 0
	if suspend {
		expectedSuspended = 1
	}
	if userInfo.Suspended != expectedSuspended {
		return fmt.Errorf("user suspension state not updated: expected %d, got %d", expectedSuspended, userInfo.Suspended)
	}
	return nil
}

func (c *CLI) RgwSubuserCreate(ctx context.Context, uid, subuser string, opts *RgwSubuserCreateOptions) error {
	args := []string{"--conf", c.confPath, "--format=json", "subuser", "create", "--uid=" + uid, "--subuser=" + subuser}

	if opts != nil {
		if opts.Access != "" {
			args = append(args, "--access="+opts.Access)
		}
	}

	cmd := exec.CommandContext(ctx, "radosgw-admin", args...)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to create rgw subuser %s for %s: %w", subuser, uid, err)
	}

	var userInfo RgwUserInfo
	if err := json.Unmarshal(output, &userInfo); err != nil {
		return fmt.Errorf("failed to parse rgw subuser create output: %w", err)
	}

	_, err = c.RgwUserInfo(ctx, uid)
	if err != nil {
		return fmt.Errorf("failed to verify subuser creation: %w", err)
	}

	return nil
}

func (c *CLI) RgwKeyCreate(ctx context.Context, uid string, opts *RgwKeyCreateOptions) error {
	args := []string{"--conf", c.confPath, "--format=json", "key", "create", "--uid=" + uid}

	var expectedAccessKey string
	if opts != nil {
		if opts.Subuser != "" {
			args = append(args, "--subuser="+opts.Subuser)
		}
		if opts.KeyType != "" {
			args = append(args, "--key-type="+opts.KeyType)
		}
		if opts.AccessKey != "" {
			args = append(args, "--access-key="+opts.AccessKey)
			expectedAccessKey = opts.AccessKey
		}
		if opts.SecretKey != "" {
			args = append(args, "--secret-key="+opts.SecretKey)
		}
	}

	cmd := exec.CommandContext(ctx, "radosgw-admin", args...)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to create rgw key for %s: %w", uid, err)
	}

	var userInfo RgwUserInfo
	if err := json.Unmarshal(output, &userInfo); err != nil {
		return fmt.Errorf("failed to parse rgw key create output: %w", err)
	}

	verifiedInfo, err := c.RgwUserInfo(ctx, uid)
	if err != nil {
		return fmt.Errorf("failed to verify key creation: %w", err)
	}

	if expectedAccessKey != "" {
		found := false
		expectedUser := uid
		if opts != nil && opts.Subuser != "" {
			expectedUser = opts.Subuser
		}

		for _, key := range verifiedInfo.Keys {
			if key.AccessKey == expectedAccessKey && key.User == expectedUser {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("key with access key %s not found for user %s", expectedAccessKey, expectedUser)
		}
	} else if len(verifiedInfo.Keys) == 0 {
		return fmt.Errorf("no keys found for user")
	}

	return nil
}

func (c *CLI) RgwKeyRemove(ctx context.Context, uid, accessKey string) error {
	args := []string{"--conf", c.confPath, "key", "rm", "--uid=" + uid, "--access-key=" + accessKey}

	cmd := exec.CommandContext(ctx, "radosgw-admin", args...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove rgw key %s for %s: %w", accessKey, uid, err)
	}

	userInfo, err := c.RgwUserInfo(ctx, uid)
	if err != nil {
		return fmt.Errorf("failed to verify key removal: %w", err)
	}
	for _, key := range userInfo.Keys {
		if key.AccessKey == accessKey {
			return fmt.Errorf("key still exists after removal: %s", accessKey)
		}
	}
	return nil
}

func (c *CLI) RgwQuotaSet(ctx context.Context, uid, scope string, maxSizeBytes, maxObjects int64) error {
	cmd := exec.CommandContext(ctx, "radosgw-admin", "--conf", c.confPath, "quota", "set",
		"--quota-scope="+scope, "--uid="+uid,
		fmt.Sprintf("--max-size=%d", maxSizeBytes), fmt.Sprintf("--max-objects=%d", maxObjects))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to set rgw quota for %s: %w, output: %s", uid, err, string(output))
	}

	cmd = exec.CommandContext(ctx, "radosgw-admin", "--conf", c.confPath, "quota", "enable",
		"--quota-scope="+scope, "--uid="+uid)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable rgw quota for %s: %w, output: %s", uid, err, string(output))
	}
	return nil
}

func (c *CLI) RgwCapsAdd(ctx context.Context, uid, caps string) error {
	cmd := exec.CommandContext(ctx, "radosgw-admin", "--conf", c.confPath, "caps", "add", "--uid="+uid, "--caps="+caps)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to add rgw caps for %s: %w, output: %s", uid, err, string(output))
	}
	return nil
}

func (c *CLI) RgwCapsRm(ctx context.Context, uid, caps string) error {
	cmd := exec.CommandContext(ctx, "radosgw-admin", "--conf", c.confPath, "caps", "rm", "--uid="+uid, "--caps="+caps)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove rgw caps for %s: %w, output: %s", uid, err, string(output))
	}
	return nil
}
