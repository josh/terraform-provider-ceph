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
)

type CephAPIClient struct {
	endpoint *url.URL
	token    string
	client   *http.Client
}

func (c *CephAPIClient) Configure(ctx context.Context, endpoints []*url.URL, username, password, token string) error {
	endpoint, err := queryEndpoints(ctx, endpoints)
	if err != nil {
		return fmt.Errorf("unable to query endpoints: %w", err)
	}

	c.endpoint = endpoint

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

		httpResp, err := client.Do(httpReq)
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
	jsonPayload := []byte("{}")
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return false, fmt.Errorf("unable to create check request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.client.Do(httpReq)
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

	httpResp, err := c.client.Do(httpReq)
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

	httpResp, err := c.client.Do(httpReq)
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

	httpResp, err := c.client.Do(httpReq)
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

	httpResp, err := c.client.Do(httpReq)
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

	httpResp, err := c.client.Do(httpReq)
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

	httpResp, err := c.client.Do(httpReq)
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

	httpResp, err := c.client.Do(httpReq)
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

	httpResp, err := c.client.Do(httpReq)
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

	httpResp, err := c.client.Do(httpReq)
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

	httpResp, err := c.client.Do(httpReq)
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

type CephAPIRGWUserKey struct {
	User      string `json:"user"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Active    bool   `json:"active"`
}

type CephAPIRGWUser struct {
	UserID      string              `json:"user_id"`
	DisplayName string              `json:"display_name"`
	MaxBuckets  int                 `json:"max_buckets"`
	Keys        []CephAPIRGWUserKey `json:"keys"`
	System      bool                `json:"system"`
	Admin       bool                `json:"admin"`
	CreateDate  string              `json:"create_date"`
	UID         string              `json:"uid"`
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

	httpResp, err := c.client.Do(httpReq)
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
	Admin       *bool  `json:"admin,omitempty"`
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

	httpResp, err := c.client.Do(httpReq)
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
	Admin       *bool   `json:"admin,omitempty"`
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

	httpResp, err := c.client.Do(httpReq)
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

	httpResp, err := c.client.Do(httpReq)
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
