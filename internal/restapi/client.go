package restapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var ErrNotFound = errors.New("ceph API returned status 404: resource not found")

type dashboardError struct {
	Detail    string  `json:"detail"`
	Code      *string `json:"code"`
	Component *string `json:"component"`
	Status    *int    `json:"status"`
}

func parseDashboardError(body []byte) (*dashboardError, error) {
	var dashboardErr dashboardError
	if err := json.Unmarshal(body, &dashboardErr); err != nil {
		return nil, err
	}
	return &dashboardErr, nil
}

type rgwErrorResponse struct {
	Code      string `json:"Code"`
	RequestID string `json:"RequestId"`
	HostID    string `json:"HostId"`
}

var pythonByteStringJoinReplacer = strings.NewReplacer(
	"'\r\nb'", "",
	"'\nb'", "",
	"'\n", "'",
	"\r", "",
	"\n", "",
)

func (e *dashboardError) RGWError() (*rgwErrorResponse, bool) {
	if e.Component == nil || *e.Component != "rgw" {
		return nil, false
	}

	cleaned := pythonByteStringJoinReplacer.Replace(e.Detail)

	jsonStart := strings.Index(cleaned, "{")
	jsonEnd := strings.LastIndex(cleaned, "}")
	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		return nil, false
	}

	var rgwErr rgwErrorResponse
	if err := json.Unmarshal([]byte(cleaned[jsonStart:jsonEnd+1]), &rgwErr); err != nil {
		return nil, false
	}

	return &rgwErr, true
}

type Client struct {
	endpoint *url.URL
	token    string
	client   *http.Client

	rgwUserLocks sync.Map
}

// RGW mutations on one user all rewrite the same user metadata object, and
// racing writers lose with 409 ConcurrentModification (surfaced by the
// dashboard proxy as an opaque 500), so serialize them per uid.
func (c *Client) lockRGWUser(uid string) func() {
	mu, _ := c.rgwUserLocks.LoadOrStore(uid, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	return mu.(*sync.Mutex).Unlock
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

func (c *Client) Token() string {
	return c.token
}
