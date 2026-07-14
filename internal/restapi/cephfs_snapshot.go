package restapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#post--api-cephfs--fs_id--snapshot>
//
// Snapshot creation needs the MDS "s" cap flag, which the mgr's
// "mds allow *" caps include.

type CephFSSnapshotInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Created string `json:"created"`
}

func (c *Client) CephFSGetSnapshot(ctx context.Context, fsID int, dirPath, name string) (*CephFSSnapshotInfo, error) {
	entries, err := c.cephFSListDir(ctx, fsID, path.Dir(dirPath))
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.Path != dirPath {
			continue
		}
		for _, snap := range entry.Snapshots {
			if snap.Name == name {
				return &snap, nil
			}
		}
		break
	}

	return nil, ErrNotFound
}

func (c *Client) CephFSMkSnapshot(ctx context.Context, fsID int, dirPath, name string) error {
	jsonPayload, err := json.Marshal(map[string]string{
		"path": dirPath,
		"name": name,
	})
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	tflog.Trace(ctx, "Ceph API request body", map[string]any{
		"request_body": string(jsonPayload),
	})

	url := c.endpoint.JoinPath("/api/cephfs", strconv.Itoa(fsID), "snapshot").String()
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(httpResp.Body)
		// The snapshot endpoints are not wrapped in the dashboard's
		// cephfs error handler, so a missing directory escapes as an
		// opaque HTTP 500.
		if c.cephFSSnapshotDirMissing(ctx, fsID, dirPath, httpResp.StatusCode, body) {
			return ErrNotFound
		}
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

// <https://docs.ceph.com/en/latest/mgr/ceph_api/#delete--api-cephfs--fs_id--snapshot>

func (c *Client) CephFSRmSnapshot(ctx context.Context, fsID int, dirPath, name string) error {
	endpoint := c.endpoint.JoinPath("/api/cephfs", strconv.Itoa(fsID), "snapshot")
	query := url.Values{}
	query.Add("path", dirPath)
	query.Add("name", name)
	endpoint.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("unable to create request: %w", err)
	}

	httpReq.Header.Set("Accept", "application/vnd.ceph.api.v1.0+json")
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	logRequest := logAPIRequest(ctx, httpReq)
	httpResp, err := c.client.Do(httpReq)
	logRequest(httpResp, err)
	if err != nil {
		return fmt.Errorf("unable to make request to Ceph API: %w", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck

	if httpResp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}

	if httpResp.StatusCode != http.StatusOK && httpResp.StatusCode != http.StatusAccepted && httpResp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(httpResp.Body)
		// A missing snapshot escapes as an opaque HTTP 500; probe to
		// distinguish it from other failures.
		if httpResp.StatusCode == http.StatusInternalServerError {
			if bytes.Contains(body, []byte("ObjectNotFound")) ||
				bytes.Contains(body, []byte("No such file or directory")) {
				return ErrNotFound
			}
			if _, err := c.CephFSGetSnapshot(ctx, fsID, dirPath, name); errors.Is(err, ErrNotFound) {
				return ErrNotFound
			}
		}
		return fmt.Errorf("ceph API returned status %d: %s", httpResp.StatusCode, string(body))
	}

	return nil
}

func (c *Client) cephFSSnapshotDirMissing(ctx context.Context, fsID int, dirPath string, statusCode int, body []byte) bool {
	if statusCode != http.StatusInternalServerError {
		return false
	}
	if bytes.Contains(body, []byte("ObjectNotFound")) ||
		bytes.Contains(body, []byte("No such file or directory")) {
		return true
	}
	_, err := c.CephFSGetDirectory(ctx, fsID, dirPath)
	return errors.Is(err, ErrNotFound)
}
