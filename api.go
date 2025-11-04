package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

type CephAPIClient struct {
	endpoint *url.URL
	token    string
	client   *http.Client
}

func logAPIRequest(ctx context.Context, req *http.Request) func(*http.Response, error) {
	startTime := time.Now()
	requestURL := req.URL.String()
	host := req.URL.Host
	path := req.URL.Path

	return func(resp *http.Response, err error) {
		duration := time.Since(startTime)
		fields := map[string]interface{}{
			"method":      req.Method,
			"url":         requestURL,
			"host":        host,
			"path":        path,
			"duration_ms": duration.Milliseconds(),
		}

		if resp != nil {
			fields["status"] = resp.StatusCode
		}

		if err != nil {
			fields["error"] = err.Error()
			tflog.Error(ctx, "Ceph API request failed", fields)
			return
		}

		tflog.Info(ctx, "Ceph API request completed", fields)
	}
}

func (c *CephAPIClient) Configure(ctx context.Context, endpoints []*url.URL, username, password, token string) error {
	endpoint, err := queryEndpoints(ctx, endpoints)
	if err != nil {
		return fmt.Errorf("unable to query endpoints: %w", err)
	}

	c.endpoint = endpoint
	tflog.Info(ctx, "Using ceph mgr endpoint", map[string]interface{}{
		"endpoint": endpoint.String(),
	})

	if c.client == nil {
		c.client = &http.Client{
			Timeout: 10 * time.Second,
		}
	}

	if token != "" {
		c.token = token

		valid, err := c.AuthCheck(ctx)
		if err != nil {
			return fmt.Errorf("failed to validate token: %w", err)
		} else if !valid {
			return fmt.Errorf("provided token is invalid or expired")
		}
	} else if username != "" && password != "" {
		authToken, err := c.Auth(ctx, username, password)
		if err != nil {
			return fmt.Errorf("failed to authenticate with credentials: %w", err)
		}

		c.token = authToken
	} else {
		return fmt.Errorf("either token or username/password must be provided")
	}

	return nil
}

func queryEndpoints(ctx context.Context, endpoints []*url.URL) (*url.URL, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	for _, endpoint := range endpoints {
		httpReq, err := http.NewRequestWithContext(ctx, "GET", endpoint.String(), nil)
		if err != nil {
			continue
		}

		done := logAPIRequest(ctx, httpReq)
		httpResp, err := client.Do(httpReq)
		done(httpResp, err)
		if err != nil {
			continue
		}

		if httpResp.StatusCode == http.StatusServiceUnavailable {
			continue
		}

		return endpoint, nil
	}

	return nil, errors.New("no available endpoints found")
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-auth-check>

func (c *CephAPIClient) AuthCheck(ctx context.Context) (bool, error) {
	url := c.endpoint.JoinPath("/api/auth/check").String() + "?token=" + c.token
	ctx = tflog.MaskLogStrings(ctx, c.token)
	jsonPayload := []byte("{}")
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return false, fmt.Errorf("unable to create check request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")

	done := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	done(httpResp, err)
	if err != nil {
		return false, fmt.Errorf("unable to make check request: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	switch httpResp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted:
		return true, nil
	case http.StatusUnauthorized:
		return false, fmt.Errorf("token is invalid or expired")
	default:
		body, _ := io.ReadAll(httpResp.Body)
		return false, fmt.Errorf("unknown error [%d]: %s", httpResp.StatusCode, string(body))
	}
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-auth>

type CephAPIAuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type CephAPIAuthResponse struct {
	Token             string              `json:"token"`
	Username          string              `json:"username"`
	Permissions       map[string][]string `json:"permissions,omitempty"`
	PwdExpirationDate *string             `json:"pwdExpirationDate,omitempty"`
	SSO               bool                `json:"sso"`
	PwdUpdateRequired bool                `json:"pwdUpdateRequired"`
}

func (c *CephAPIClient) Auth(ctx context.Context, username string, password string) (string, error) {
	ctx = tflog.MaskLogStrings(ctx, password)

	requestBody := CephAPIAuthRequest{
		Username: username,
		Password: password,
	}

	jsonPayload, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("unable to encode authentication request: %w", err)
	}

	url := c.endpoint.JoinPath("/api/auth").String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("unable to create authentication request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")

	done := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	done(httpResp, err)
	if err != nil {
		return "", fmt.Errorf("unable to make authentication request: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(httpResp.Body)
		return "", fmt.Errorf("authentication failed with status %d: %s", httpResp.StatusCode, string(body))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read authentication response: %w", err)
	}

	var authResp CephAPIAuthResponse
	err = json.Unmarshal(body, &authResp)
	if err != nil {
		return "", fmt.Errorf("unable to decode authentication response: %w", err)
	}

	if authResp.Token == "" {
		return "", fmt.Errorf("authentication response did not contain a token")
	}

	return authResp.Token, nil
}

// https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-cluster-user-export

type CephAPIClusterUserExportRequest struct {
	Entities []string `json:"entities"`
}

func (c *CephAPIClient) ClusterExportUser(ctx context.Context, entity string) (string, error) {
	requestBody := CephAPIClusterUserExportRequest{
		Entities: []string{entity},
	}

	jsonPayload, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint.JoinPath("/api/cluster/user/export").String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return "", fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ceph API returned status %d", httpResp.StatusCode)
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read response body: %w", err)
	}

	var keyringRaw string
	err = json.Unmarshal(body, &keyringRaw)
	if err != nil {
		return "", fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return keyringRaw, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-cluster-user>

type CephAPIClusterUser struct {
	Entity string            `json:"entity"`
	Caps   map[string]string `json:"caps"`
	Key    string            `json:"key"`
}

func (c *CephAPIClient) ClusterListUsers(ctx context.Context) ([]CephAPIClusterUser, error) {
	url := c.endpoint.JoinPath("/api/cluster/user").String()
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return nil, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ceph API returned status %d", httpResp.StatusCode)
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	var users []CephAPIClusterUser
	err = json.Unmarshal(body, &users)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return users, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-cluster-user>

type CephAPIClusterUserCapability struct {
	Entity string `json:"entity"`
	Cap    string `json:"cap"`
}

type CephAPIClusterUserCreateRequest struct {
	UserEntity   string                         `json:"user_entity,omitempty"`
	Capabilities []CephAPIClusterUserCapability `json:"capabilities,omitempty"`
	ImportData   string                         `json:"import_data,omitempty"`
}

func (c CephCaps) asClusterCapabilities() []CephAPIClusterUserCapability {
	capabilitySlice := make([]CephAPIClusterUserCapability, 0, 4)

	if c.MDS != "" {
		capabilitySlice = append(capabilitySlice, CephAPIClusterUserCapability{Entity: "mds", Cap: c.MDS})
	}

	if c.MGR != "" {
		capabilitySlice = append(capabilitySlice, CephAPIClusterUserCapability{Entity: "mgr", Cap: c.MGR})
	}

	if c.MON != "" {
		capabilitySlice = append(capabilitySlice, CephAPIClusterUserCapability{Entity: "mon", Cap: c.MON})
	}

	if c.OSD != "" {
		capabilitySlice = append(capabilitySlice, CephAPIClusterUserCapability{Entity: "osd", Cap: c.OSD})
	}

	return capabilitySlice
}

func (c *CephAPIClient) ClusterCreateUser(ctx context.Context, entity string, capabilities CephCaps) error {
	capabilitySlice := capabilities.asClusterCapabilities()

	requestBody := CephAPIClusterUserCreateRequest{
		UserEntity:   entity,
		Capabilities: capabilitySlice,
	}

	jsonPayload, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint.JoinPath("/api/cluster/user").String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

func (c *CephAPIClient) ClusterImportUser(ctx context.Context, importData string) error {
	requestBody := CephAPIClusterUserCreateRequest{
		ImportData: importData,
	}

	jsonPayload, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint.JoinPath("/api/cluster/user").String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#put--api-cluster-user>

type CephAPIClusterUserUpdateRequest struct {
	UserEntity   string                         `json:"user_entity"`
	Capabilities []CephAPIClusterUserCapability `json:"capabilities"`
}

func (c *CephAPIClient) ClusterUpdateUser(ctx context.Context, entity string, capabilities CephCaps) error {
	capabilitySlice := capabilities.asClusterCapabilities()

	requestBody := CephAPIClusterUserUpdateRequest{
		UserEntity:   entity,
		Capabilities: capabilitySlice,
	}

	jsonPayload, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint.JoinPath("/api/cluster/user").String()
	httpReq, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-cluster-user-user_entities>

func (c *CephAPIClient) ClusterDeleteUser(ctx context.Context, userEntities string) error {
	url := c.endpoint.JoinPath("/api/cluster/user", userEntities).String()
	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusAccepted && httpResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-rgw-bucket>

func (c *CephAPIClient) RGWListBucketNames(ctx context.Context) ([]string, error) {
	url := c.endpoint.JoinPath("/api/rgw/bucket").String()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return nil, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ceph API returned status %d", httpResp.StatusCode)
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	var buckets []string
	err = json.Unmarshal(body, &buckets)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return buckets, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-rgw-bucket-bucket>

type CephAPIRGWBucket struct {
	Bucket        string `json:"bucket"`
	Zonegroup     string `json:"zonegroup"`
	PlacementRule string `json:"placement_rule"`
	ID            string `json:"id"`
	Owner         string `json:"owner"`
	CreationTime  string `json:"creation_time"`
	ACL           string `json:"acl"`
	Bid           string `json:"bid"`
}

func (c *CephAPIClient) RGWGetBucket(ctx context.Context, bucketName string) (CephAPIRGWBucket, error) {
	url := c.endpoint.JoinPath("/api/rgw/bucket", bucketName).String()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return CephAPIRGWBucket{}, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return CephAPIRGWBucket{}, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK {
		return CephAPIRGWBucket{}, fmt.Errorf("ceph API returned status %d", httpResp.StatusCode)
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return CephAPIRGWBucket{}, fmt.Errorf("unable to read response body: %w", err)
	}

	var bucket CephAPIRGWBucket
	err = json.Unmarshal(body, &bucket)
	if err != nil {
		return CephAPIRGWBucket{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return bucket, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-rgw-user>

func (c *CephAPIClient) RGWListUserNames(ctx context.Context) ([]string, error) {
	url := c.endpoint.JoinPath("/api/rgw/user").String()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return nil, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ceph API returned status %d", httpResp.StatusCode)
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	var users []string
	err = json.Unmarshal(body, &users)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return users, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-rgw-user-ratelimit>

type CephAPIRGWS3Key struct {
	User       string `json:"user"`
	AccessKey  string `json:"access_key"`
	SecretKey  string `json:"secret_key"`
	Active     bool   `json:"active"`
	CreateDate string `json:"create_date"`
}

type CephAPIRGWSwiftKey struct {
	User       string `json:"user"`
	SecretKey  string `json:"secret_key"`
	Active     bool   `json:"active"`
	CreateDate string `json:"create_date"`
}

type CephAPIRGWSubuser struct {
	ID          string `json:"id"`
	Permissions string `json:"permissions"`
}

type CephAPIRGWCaps struct {
	Type string `json:"type"`
	Perm string `json:"perm"`
}

type CephAPIRGWQuota struct {
	Enabled    bool  `json:"enabled"`
	CheckOnRaw bool  `json:"check_on_raw"`
	MaxSize    int64 `json:"max_size"`
	MaxSizeKB  int64 `json:"max_size_kb"`
	MaxObjects int64 `json:"max_objects"`
}

type CephAPIRGWUserStats struct {
	Size           int64 `json:"size"`
	SizeActual     int64 `json:"size_actual"`
	SizeUtilized   int64 `json:"size_utilized"`
	SizeKB         int64 `json:"size_kb"`
	SizeKBActual   int64 `json:"size_kb_actual"`
	SizeKBUtilized int64 `json:"size_kb_utilized"`
	NumObjects     int64 `json:"num_objects"`
}

type CephAPIRGWUser struct {
	Tenant              string               `json:"tenant"`
	UserID              string               `json:"user_id"`
	DisplayName         string               `json:"display_name"`
	Email               string               `json:"email"`
	Suspended           int                  `json:"suspended"`
	MaxBuckets          int                  `json:"max_buckets"`
	Subusers            []CephAPIRGWSubuser  `json:"subusers"`
	Keys                []CephAPIRGWS3Key    `json:"keys"`
	SwiftKeys           []CephAPIRGWSwiftKey `json:"swift_keys"`
	Caps                []CephAPIRGWCaps     `json:"caps"`
	OpMask              string               `json:"op_mask"`
	System              bool                 `json:"system"`
	Admin               bool                 `json:"admin"`
	DefaultPlacement    string               `json:"default_placement"`
	DefaultStorageClass string               `json:"default_storage_class"`
	PlacementTags       []string             `json:"placement_tags"`
	BucketQuota         CephAPIRGWQuota      `json:"bucket_quota"`
	UserQuota           CephAPIRGWQuota      `json:"user_quota"`
	TempURLKeys         []json.RawMessage    `json:"temp_url_keys"`
	Type                string               `json:"type"`
	MFAIDs              []string             `json:"mfa_ids"`
	Stats               *CephAPIRGWUserStats `json:"stats,omitempty"`
}

func (c *CephAPIClient) RGWGetUser(ctx context.Context, uid string) (CephAPIRGWUser, error) {
	url := c.endpoint.JoinPath("/api/rgw/user", uid).String()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return CephAPIRGWUser{}, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return CephAPIRGWUser{}, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK {
		return CephAPIRGWUser{}, fmt.Errorf("ceph API returned status %d", httpResp.StatusCode)
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return CephAPIRGWUser{}, fmt.Errorf("unable to read response body: %w", err)
	}

	var user CephAPIRGWUser
	err = json.Unmarshal(body, &user)
	if err != nil {
		return CephAPIRGWUser{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return user, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-rgw-user>

type CephAPIRGWUserCreateRequest struct {
	UID         string `json:"uid"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email,omitempty"`
	MaxBuckets  *int   `json:"max_buckets,omitempty"`
	Suspended   *int   `json:"suspended,omitempty"`
	System      *bool  `json:"system,omitempty"`
	GenerateKey bool   `json:"generate_key"`
}

func (c *CephAPIClient) RGWCreateUser(ctx context.Context, req CephAPIRGWUserCreateRequest) (CephAPIRGWUser, error) {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return CephAPIRGWUser{}, fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint.JoinPath("/api/rgw/user").String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return CephAPIRGWUser{}, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return CephAPIRGWUser{}, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(httpResp.Body)
		return CephAPIRGWUser{}, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return CephAPIRGWUser{}, fmt.Errorf("unable to read response body: %w", err)
	}

	var user CephAPIRGWUser
	err = json.Unmarshal(body, &user)
	if err != nil {
		return CephAPIRGWUser{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return user, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#put--api-rgw-user-uid>

type CephAPIRGWUserUpdateRequest struct {
	DisplayName *string `json:"display_name,omitempty"`
	Email       *string `json:"email,omitempty"`
	MaxBuckets  *int    `json:"max_buckets,omitempty"`
	Suspended   *int    `json:"suspended,omitempty"`
	System      *bool   `json:"system,omitempty"`
}

func (c *CephAPIClient) RGWUpdateUser(ctx context.Context, uid string, req CephAPIRGWUserUpdateRequest) (CephAPIRGWUser, error) {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return CephAPIRGWUser{}, fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint.JoinPath("/api/rgw/user", uid).String()
	httpReq, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return CephAPIRGWUser{}, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return CephAPIRGWUser{}, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(httpResp.Body)
		return CephAPIRGWUser{}, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return CephAPIRGWUser{}, fmt.Errorf("unable to read response body: %w", err)
	}

	var user CephAPIRGWUser
	err = json.Unmarshal(body, &user)
	if err != nil {
		return CephAPIRGWUser{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return user, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-rgw-user-uid>

func (c *CephAPIClient) RGWDeleteUser(ctx context.Context, uid string) error {
	url := c.endpoint.JoinPath("/api/rgw/user", uid).String()
	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted && httpResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-rgw-user-uid-key>

type rgwS3KeyCreateRequest struct {
	UID         string  `json:"uid"`
	KeyType     string  `json:"key_type"`
	SubUser     *string `json:"subuser,omitempty"`
	AccessKey   *string `json:"access_key,omitempty"`
	SecretKey   *string `json:"secret_key,omitempty"`
	GenerateKey *bool   `json:"generate_key,omitempty"`
}

func (c *CephAPIClient) RGWCreateS3Key(ctx context.Context, uid string, subuser *string, accessKey *string, secretKey *string, generateKey *bool) ([]CephAPIRGWS3Key, error) {
	payload := rgwS3KeyCreateRequest{
		UID:         uid,
		KeyType:     "s3",
		SubUser:     subuser,
		AccessKey:   accessKey,
		SecretKey:   secretKey,
		GenerateKey: generateKey,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint.JoinPath("/api/rgw/user", uid, "key").String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return nil, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	var keys []CephAPIRGWS3Key
	err = json.Unmarshal(body, &keys)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return keys, nil
}

type rgwSwiftKeyCreateRequest struct {
	UID         string  `json:"uid"`
	KeyType     string  `json:"key_type"`
	SubUser     *string `json:"subuser,omitempty"`
	SecretKey   *string `json:"secret_key,omitempty"`
	GenerateKey *bool   `json:"generate_key,omitempty"`
}

func (c *CephAPIClient) RGWCreateSwiftKey(ctx context.Context, uid string, subuser *string, secretKey *string, generateKey *bool) ([]CephAPIRGWSwiftKey, error) {
	payload := rgwSwiftKeyCreateRequest{
		UID:         uid,
		KeyType:     "swift",
		SubUser:     subuser,
		SecretKey:   secretKey,
		GenerateKey: generateKey,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint.JoinPath("/api/rgw/user", uid, "key").String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return nil, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	var keys []CephAPIRGWSwiftKey
	err = json.Unmarshal(body, &keys)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return keys, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-rgw-user-uid-key>

func (c *CephAPIClient) RGWDeleteS3Key(ctx context.Context, uid string, accessKey string, subuser *string) error {
	endpoint := c.endpoint.JoinPath("/api/rgw/user", uid, "key")
	query := url.Values{}
	query.Add("key_type", "s3")
	query.Add("access_key", accessKey)
	if subuser != nil {
		query.Add("subuser", *subuser)
	}
	endpoint.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted && httpResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

func (c *CephAPIClient) RGWDeleteSwiftKey(ctx context.Context, uid string, secretKey string, subuser *string) error {
	endpoint := c.endpoint.JoinPath("/api/rgw/user", uid, "key")
	query := url.Values{}
	query.Add("key_type", "swift")
	query.Add("secret_key", secretKey)
	if subuser != nil {
		query.Add("subuser", *subuser)
	}
	endpoint.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted && httpResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-cluster_conf

type CephAPIClusterConfValue struct {
	Section string `json:"section"`
	Value   string `json:"value"`
}

type CephAPIClusterConf struct {
	Name               string                    `json:"name"`
	Type               string                    `json:"type"`
	Level              string                    `json:"level"`
	Desc               string                    `json:"desc"`
	LongDesc           string                    `json:"long_desc"`
	Default            interface{}               `json:"default"`
	DaemonDefault      interface{}               `json:"daemon_default"`
	Min                interface{}               `json:"min"`
	Max                interface{}               `json:"max"`
	CanUpdateAtRuntime bool                      `json:"can_update_at_runtime"`
	SeeAlso            []string                  `json:"see_also"`
	EnumValues         []string                  `json:"enum_values"`
	Tags               []string                  `json:"tags"`
	Services           []string                  `json:"services"`
	Flags              []string                  `json:"flags"`
	Source             string                    `json:"source,omitempty"`
	Value              []CephAPIClusterConfValue `json:"value,omitempty"`
}

func (c *CephAPIClient) ClusterListConf(ctx context.Context) ([]CephAPIClusterConf, error) {
	url := c.endpoint.JoinPath("/api/cluster_conf").String()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return nil, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	var configs []CephAPIClusterConf
	err = json.Unmarshal(body, &configs)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return configs, nil
}

// https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-cluster_conf-name

func (c *CephAPIClient) ClusterGetConf(ctx context.Context, name string) (CephAPIClusterConf, error) {
	url := c.endpoint.JoinPath("/api/cluster_conf", name).String()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return CephAPIClusterConf{}, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return CephAPIClusterConf{}, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return CephAPIClusterConf{}, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return CephAPIClusterConf{}, fmt.Errorf("unable to read response body: %w", err)
	}

	var config CephAPIClusterConf
	err = json.Unmarshal(body, &config)
	if err != nil {
		return CephAPIClusterConf{}, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return config, nil
}

// https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-cluster_conf

func (c *CephAPIClient) ClusterUpdateConf(ctx context.Context, name string, section string, value string) error {
	requestBody := map[string]interface{}{
		"name": name,
		"value": []map[string]string{
			{
				"section": section,
				"value":   value,
			},
		},
	}

	jsonPayload, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint.JoinPath("/api/cluster_conf").String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-cluster_conf-name

func (c *CephAPIClient) ClusterDeleteConf(ctx context.Context, name string, section string) error {
	endpoint := c.endpoint.JoinPath("/api/cluster_conf", name)
	query := url.Values{}
	query.Add("section", section)
	endpoint.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusAccepted && httpResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-mgr-module>

type CephAPIMgrModule struct {
	Name     string                            `json:"name"`
	Enabled  bool                              `json:"enabled"`
	AlwaysOn bool                              `json:"always_on"`
	Options  map[string]CephAPIMgrModuleOption `json:"options"`
}

type CephAPIMgrModuleOption struct {
	Name         string      `json:"name"`
	Type         string      `json:"type"`
	Level        string      `json:"level"`
	Flags        int         `json:"flags"`
	DefaultValue interface{} `json:"default_value"`
	Min          interface{} `json:"min"`
	Max          interface{} `json:"max"`
	EnumAllowed  []string    `json:"enum_allowed"`
	Desc         string      `json:"desc"`
	LongDesc     string      `json:"long_desc"`
	Tags         []string    `json:"tags"`
	SeeAlso      []string    `json:"see_also"`
}

func (c *CephAPIClient) MgrListModules(ctx context.Context) ([]CephAPIMgrModule, error) {
	url := c.endpoint.JoinPath("/api/mgr/module").String()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return nil, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	var modules []CephAPIMgrModule
	err = json.Unmarshal(body, &modules)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return modules, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-mgr-module-module_name>

type CephAPIMgrModuleConfig map[string]interface{}

func (c *CephAPIClient) MgrGetModuleConfig(ctx context.Context, moduleName string) (CephAPIMgrModuleConfig, error) {
	url := c.endpoint.JoinPath("/api/mgr/module", moduleName).String()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return nil, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	var config CephAPIMgrModuleConfig
	err = json.Unmarshal(body, &config)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return config, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#put--api-mgr-module-module_name>

type CephAPIMgrModuleConfigRequest struct {
	Config CephAPIMgrModuleConfig `json:"config"`
}

func (c *CephAPIClient) MgrSetModuleConfig(ctx context.Context, moduleName string, config CephAPIMgrModuleConfig) error {
	requestBody := CephAPIMgrModuleConfigRequest{
		Config: config,
	}

	jsonPayload, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint.JoinPath("/api/mgr/module", moduleName).String()
	httpReq, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-mgr-module-module_name-disable>

func (c *CephAPIClient) MgrDisableModule(ctx context.Context, moduleName string) error {
	url := c.endpoint.JoinPath("/api/mgr/module", moduleName, "disable").String()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-mgr-module-module_name-enable>

func (c *CephAPIClient) MgrEnableModule(ctx context.Context, moduleName string) error {
	url := c.endpoint.JoinPath("/api/mgr/module", moduleName, "enable").String()

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-mgr-module-module_name-options>

func (c *CephAPIClient) MgrGetModuleOptions(ctx context.Context, moduleName string) (map[string]CephAPIMgrModuleOption, error) {
	url := c.endpoint.JoinPath("/api/mgr/module", moduleName, "options").String()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return nil, fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %w", err)
	}

	var options map[string]CephAPIMgrModuleOption
	err = json.Unmarshal(body, &options)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return options, nil
}
