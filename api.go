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
		fields := map[string]any{
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
	tflog.Info(ctx, "Using ceph mgr endpoint", map[string]any{
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
	Token string `json:"token"`
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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-cluster-user>

type CephAPIClusterUserCapability struct {
	Entity string `json:"entity"`
	Cap    string `json:"cap"`
}

type CephAPIClusterUserCreateRequest struct {
	UserEntity   *string                        `json:"user_entity,omitempty"`
	Capabilities []CephAPIClusterUserCapability `json:"capabilities,omitempty"`
	ImportData   *string                        `json:"import_data,omitempty"`
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

	requestBody := CephAPIClusterUserCreateRequest{}

	if entity != "" {
		requestBody.UserEntity = &entity
	}

	if len(capabilitySlice) > 0 {
		requestBody.Capabilities = capabilitySlice
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
	requestBody := CephAPIClusterUserCreateRequest{}

	if importData != "" {
		requestBody.ImportData = &importData
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

type CephAPIRGWBucketCreateRequest struct {
	Bucket    string  `json:"bucket"`
	UID       string  `json:"uid"`
	Zonegroup *string `json:"zonegroup,omitempty"`
}

func (c *CephAPIClient) RGWCreateBucket(ctx context.Context, req CephAPIRGWBucketCreateRequest) (CephAPIRGWBucket, error) {
	url := c.endpoint.JoinPath("/api/rgw/bucket").String()

	reqBody, err := json.Marshal(req)
	if err != nil {
		return CephAPIRGWBucket{}, fmt.Errorf("unable to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
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

	if httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return CephAPIRGWBucket{}, fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
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

func (c *CephAPIClient) RGWDeleteBucket(ctx context.Context, bucketName string) error {
	url := c.endpoint.JoinPath("/api/rgw/bucket", bucketName).String()

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

	if httpResp.StatusCode != http.StatusNoContent && httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
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

type CephAPIRGWUser struct {
	Tenant      string               `json:"tenant"`
	UserID      string               `json:"user_id"`
	DisplayName string               `json:"display_name"`
	Email       string               `json:"email"`
	Suspended   int                  `json:"suspended"`
	MaxBuckets  int                  `json:"max_buckets"`
	Subusers    []CephAPIRGWSubuser  `json:"subusers"`
	Keys        []CephAPIRGWS3Key    `json:"keys"`
	SwiftKeys   []CephAPIRGWSwiftKey `json:"swift_keys"`
	System      bool                 `json:"system"`
	Admin       bool                 `json:"admin"`
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
	UID         string  `json:"uid"`
	DisplayName string  `json:"display_name"`
	Email       *string `json:"email,omitempty"`
	MaxBuckets  *int    `json:"max_buckets,omitempty"`
	Suspended   *int    `json:"suspended,omitempty"`
	System      *bool   `json:"system,omitempty"`
	GenerateKey bool    `json:"generate_key"`
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

// https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-cluster_conf

type CephAPIClusterConfValue struct {
	Section string `json:"section"`
	Value   string `json:"value"`
}

type CephAPIClusterConf struct {
	Name               string                    `json:"name"`
	Level              string                    `json:"level"`
	CanUpdateAtRuntime bool                      `json:"can_update_at_runtime"`
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
	requestBody := map[string]any{
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

type CephAPIMgrModuleOption struct {
	DefaultValue any `json:"default_value"`
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-mgr-module-module_name>

type CephAPIMgrModuleConfig map[string]any

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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-pool>

type CephAPIPoolOptions struct {
	CompressionMode          string  `json:"compression_mode"`
	CompressionAlgorithm     string  `json:"compression_algorithm"`
	CompressionRequiredRatio float64 `json:"compression_required_ratio"`
	CompressionMinBlobSize   int     `json:"compression_min_blob_size"`
	CompressionMaxBlobSize   int     `json:"compression_max_blob_size"`
	TargetSizeRatio          float64 `json:"target_size_ratio"`
	TargetSizeBytes          int     `json:"target_size_bytes"`
	PGNumMin                 int     `json:"pg_num_min"`
	PGNumMax                 int     `json:"pg_num_max"`
}

type CephAPIPool struct {
	PoolName            string             `json:"pool_name"`
	Type                string             `json:"type"`
	PoolID              int                `json:"pool_id"`
	Size                int                `json:"size"`
	MinSize             int                `json:"min_size"`
	PGNum               int                `json:"pg_num"`
	PGPlacementNum      int                `json:"pg_placement_num"`
	CrushRule           string             `json:"crush_rule"`
	CrashReplayInterval int                `json:"crash_replay_interval"`
	PrimaryAffinity     float64            `json:"primary_affinity"`
	Application         string             `json:"application"`
	ApplicationMetadata []string           `json:"application_metadata"`
	Flags               int                `json:"flags"`
	ErasureCodeProfile  string             `json:"erasure_code_profile"`
	PGAutoscaleMode     string             `json:"pg_autoscale_mode"`
	TargetSizeRatioRel  float64            `json:"target_size_ratio_rel"`
	MinPGNum            int                `json:"min_pg_num"`
	PGAutoscalerProfile string             `json:"pg_autoscaler_profile"`
	Options             CephAPIPoolOptions `json:"options"`
}

func (c *CephAPIClient) ListPools(ctx context.Context) ([]CephAPIPool, error) {
	url := c.endpoint.JoinPath("/api/pool").String()

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

	var pools []CephAPIPool
	err = json.Unmarshal(body, &pools)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return pools, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-pool>

type CephAPIPoolCreateRequest struct {
	Pool                     string   `json:"pool"`
	PoolType                 *string  `json:"pool_type,omitempty"`
	PgNum                    *int     `json:"pg_num,omitempty"`
	PgpNum                   *int     `json:"pgp_num,omitempty"`
	CrushRule                *string  `json:"crush_rule,omitempty"`
	ErasureCodeProfile       *string  `json:"erasure_code_profile,omitempty"`
	ApplicationMetadata      []string `json:"application_metadata,omitempty"`
	MinSize                  *int     `json:"min_size,omitempty"`
	Size                     *int     `json:"size,omitempty"`
	PgAutoscaleMode          *string  `json:"pg_autoscale_mode,omitempty"`
	TargetSizeRatio          *float64 `json:"target_size_ratio,omitempty"`
	TargetSizeBytes          *int     `json:"target_size_bytes,omitempty"`
	CompressionMode          *string  `json:"compression_mode,omitempty"`
	CompressionAlgorithm     *string  `json:"compression_algorithm,omitempty"`
	CompressionRequiredRatio *float64 `json:"compression_required_ratio,omitempty"`
	CompressionMinBlobSize   *int     `json:"compression_min_blob_size,omitempty"`
	CompressionMaxBlobSize   *int     `json:"compression_max_blob_size,omitempty"`
}

func (c *CephAPIClient) CreatePool(ctx context.Context, req CephAPIPoolCreateRequest) error {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint.JoinPath("/api/pool").String()
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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-pool--pool_name>

func (c *CephAPIClient) DeletePool(ctx context.Context, poolName string) error {
	url := c.endpoint.JoinPath("/api/pool", poolName).String()
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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-pool--pool_name>

func (c *CephAPIClient) GetPool(ctx context.Context, poolName string) (*CephAPIPool, error) {
	url := c.endpoint.JoinPath("/api/pool", poolName).String()

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

	var pool CephAPIPool
	err = json.Unmarshal(body, &pool)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &pool, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#put--api-pool--pool_name>

type CephAPIPoolUpdateRequest struct {
	PgNum                    *int     `json:"pg_num,omitempty"`
	PgpNum                   *int     `json:"pgp_num,omitempty"`
	CrushRule                *string  `json:"crush_rule,omitempty"`
	Size                     *int     `json:"size,omitempty"`
	PgAutoscaleMode          *string  `json:"pg_autoscale_mode,omitempty"`
	CompressionMode          *string  `json:"compression_mode,omitempty"`
	CompressionAlgorithm     *string  `json:"compression_algorithm,omitempty"`
	CompressionRequiredRatio *float64 `json:"compression_required_ratio,omitempty"`
	CompressionMinBlobSize   *int     `json:"compression_min_blob_size,omitempty"`
	CompressionMaxBlobSize   *int     `json:"compression_max_blob_size,omitempty"`
}

func (c *CephAPIClient) UpdatePool(ctx context.Context, poolName string, req CephAPIPoolUpdateRequest) error {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint.JoinPath("/api/pool", poolName).String()
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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-pool--pool_name-configuration>

type CephAPIPoolConfigItem struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

type CephAPIPoolConfiguration []CephAPIPoolConfigItem

func (c *CephAPIClient) GetPoolConfiguration(ctx context.Context, poolName string) (CephAPIPoolConfiguration, error) {
	url := c.endpoint.JoinPath("/api/pool", poolName, "configuration").String()

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

	var config CephAPIPoolConfiguration
	err = json.Unmarshal(body, &config)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return config, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-crush_rule>

type CephAPICrushRuleStep struct {
	Op   string `json:"op"`
	Num  int    `json:"num"`
	Type string `json:"type"`
	Item int    `json:"item,omitempty"`
}

type CephAPICrushRule struct {
	RuleID   int                    `json:"rule_id"`
	RuleName string                 `json:"rule_name"`
	Ruleset  int                    `json:"ruleset"`
	Type     int                    `json:"type"`
	MinSize  int                    `json:"min_size"`
	MaxSize  int                    `json:"max_size"`
	Steps    []CephAPICrushRuleStep `json:"steps"`
}

func (c *CephAPIClient) ListCrushRules(ctx context.Context) ([]CephAPICrushRule, error) {
	url := c.endpoint.JoinPath("/api/crush_rule").String()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v2.0+json")
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

	var rules []CephAPICrushRule
	err = json.Unmarshal(body, &rules)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return rules, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-crush_rule>

type CephAPICrushRuleCreateRequest struct {
	Name          string  `json:"name"`
	PoolType      *string `json:"pool_type,omitempty"`
	FailureDomain string  `json:"failure_domain"`
	DeviceClass   *string `json:"device_class,omitempty"`
	Profile       *string `json:"profile,omitempty"`
	Root          *string `json:"root,omitempty"`
}

func (c *CephAPIClient) CreateCrushRule(ctx context.Context, req CephAPICrushRuleCreateRequest) error {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint.JoinPath("/api/crush_rule").String()
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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-crush_rule--name>

func (c *CephAPIClient) DeleteCrushRule(ctx context.Context, name string) error {
	url := c.endpoint.JoinPath("/api/crush_rule", name).String()
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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-crush_rule--name>

func (c *CephAPIClient) GetCrushRule(ctx context.Context, name string) (*CephAPICrushRule, error) {
	url := c.endpoint.JoinPath("/api/crush_rule", name).String()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v2.0+json")
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

	var rule CephAPICrushRule
	err = json.Unmarshal(body, &rule)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &rule, nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-erasure_code_profile>

type CephAPIErasureCodeProfile struct {
	Name               string `json:"name"`
	K                  int    `json:"k"`
	M                  int    `json:"m"`
	Plugin             string `json:"plugin"`
	CrushFailureDomain string `json:"crush-failure-domain"`
	Technique          string `json:"technique,omitempty"`
	CrushRoot          string `json:"crush-root,omitempty"`
	CrushDeviceClass   string `json:"crush-device-class,omitempty"`
	Directory          string `json:"directory,omitempty"`
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-erasure_code_profile>

type CephAPIErasureCodeProfileCreateRequest struct {
	Name               string  `json:"name"`
	K                  *string `json:"k,omitempty"`
	M                  *string `json:"m,omitempty"`
	Plugin             *string `json:"plugin,omitempty"`
	CrushFailureDomain *string `json:"crush-failure-domain,omitempty"`
	Technique          *string `json:"technique,omitempty"`
	CrushRoot          *string `json:"crush-root,omitempty"`
	CrushDeviceClass   *string `json:"crush-device-class,omitempty"`
	Directory          *string `json:"directory,omitempty"`
}

func (c *CephAPIClient) CreateErasureCodeProfile(ctx context.Context, req CephAPIErasureCodeProfileCreateRequest) error {
	jsonPayload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	url := c.endpoint.JoinPath("/api/erasure_code_profile").String()
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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-erasure_code_profile--name>

func (c *CephAPIClient) DeleteErasureCodeProfile(ctx context.Context, name string) error {
	url := c.endpoint.JoinPath("/api/erasure_code_profile", name).String()
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

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#get--api-erasure_code_profile--name>

func (c *CephAPIClient) GetErasureCodeProfile(ctx context.Context, name string) (*CephAPIErasureCodeProfile, error) {
	url := c.endpoint.JoinPath("/api/erasure_code_profile", name).String()

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

	var profile CephAPIErasureCodeProfile
	err = json.Unmarshal(body, &profile)
	if err != nil {
		return nil, fmt.Errorf("unable to decode JSON response: %w", err)
	}

	return &profile, nil
}
