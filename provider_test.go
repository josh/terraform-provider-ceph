package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/config"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

var (
	testDashboardURL string
	testClusterWG    *sync.WaitGroup
	testConfPath     string
	testTimeout      = flag.Duration("timeout", 0, "test timeout")
	cephDaemonLogs   *LogDemux
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"ceph": providerserver.NewProtocol6WithError(providerFunc()),
}

func TestMain(m *testing.M) {
	flag.Parse()

	var code int

	if os.Getenv("TF_ACC") != "" {
		timeout := 10 * time.Minute
		if *testTimeout > 0 {
			timeout = *testTimeout
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		tmpDir, err := os.MkdirTemp("", "ceph-test-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
			os.Exit(1)
		}

		cephDaemonLogs = &LogDemux{}
		var confPath string
		var setupBuffer bytes.Buffer
		detachLogs := cephDaemonLogs.Attach(&setupBuffer)
		testDashboardURL, confPath, testClusterWG, err = startCephCluster(ctx, tmpDir, cephDaemonLogs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to start ceph cluster: %v\n", err)
			fmt.Fprintln(os.Stderr, "\n=== Ceph cluster setup logs ===")
			if _, err := io.Copy(os.Stderr, &setupBuffer); err != nil {
				fmt.Fprintf(os.Stderr, "failed to flush setup log: %v\n", err)
			}
			if err := os.RemoveAll(tmpDir); err != nil {
				fmt.Fprintf(os.Stderr, "failed to clean up temp dir: %v\n", err)
			}
			os.Exit(1)
		}
		detachLogs()
		testConfPath = confPath

		code = m.Run()

		cancel()
		testClusterWG.Wait()
		if err := os.RemoveAll(tmpDir); err != nil {
			fmt.Fprintf(os.Stderr, "failed to clean up temp dir: %v\n", err)
		}
	} else {
		code = m.Run()
	}

	os.Exit(code)
}

func startCephCluster(ctx context.Context, tmpDir string, out io.Writer) (string, string, *sync.WaitGroup, error) {
	startupCtx, startupCancel := context.WithTimeout(ctx, 90*time.Second)
	defer startupCancel()

	confPath, err := setupCephDir(startupCtx, tmpDir, out)
	if err != nil {
		return "", "", nil, err
	}

	var wg sync.WaitGroup

	if err := startCephMon(&wg, ctx, confPath, out); err != nil {
		return "", "", nil, err
	}

	if err := waitForCephMon(startupCtx, confPath); err != nil {
		return "", "", nil, err
	}

	if err := startCephOsd(&wg, ctx, confPath, tmpDir, out); err != nil {
		return "", "", nil, err
	}

	if err := waitForCephOsd(startupCtx, confPath); err != nil {
		return "", "", nil, err
	}

	if err := startCephMgr(&wg, ctx, confPath, out); err != nil {
		return "", "", nil, err
	}

	if err := waitForCephMgr(startupCtx, confPath); err != nil {
		return "", "", nil, err
	}

	if err := startCephRgw(&wg, ctx, confPath, out); err != nil {
		return "", "", nil, err
	}

	if err := waitForCephRgw(startupCtx, confPath); err != nil {
		return "", "", nil, err
	}

	dashboardURL, err := enableCephDashboard(startupCtx, confPath, out)
	if err != nil {
		return "", "", nil, err
	}

	return dashboardURL, confPath, &wg, nil
}

func setupCephDir(ctx context.Context, tmpDir string, out io.Writer) (string, error) {
	fsid := "6bb5784d-86b1-4b48-aff7-04d5dd22ef07"
	confPath := filepath.Join(tmpDir, "ceph.conf")

	cephConfig := map[string]map[string]string{
		"global": {
			"fsid":                                  fsid,
			"mon_host":                              "v1:127.0.0.1:6789/0",
			"public_network":                        "127.0.0.1/32",
			"auth_cluster_required":                 "cephx",
			"auth_service_required":                 "cephx",
			"auth_client_required":                  "cephx",
			"auth_allow_insecure_global_id_reclaim": "true",
			"pid_file":                              filepath.Join(tmpDir, "$type.$id.pid"),
			"admin_socket":                          filepath.Join(tmpDir, "$name.$pid.asok"),
			"keyring":                               filepath.Join(tmpDir, "keyring"),
			"log_to_file":                           "false",
			"log_to_stderr":                         "true",
			"debug_ms":                              "0",
			"osd_pool_default_size":                 "1",
			"osd_pool_default_min_size":             "1",
			"osd_crush_chooseleaf_type":             "0",
			"mon_allow_pool_size_one":               "true",
		},
		"mon": {
			"mon_initial_members":       "mon1",
			"mon_data":                  filepath.Join(tmpDir, "mon", "ceph-$id"),
			"mon_cluster_log_to_file":   "false",
			"mon_cluster_log_to_stderr": "true",
			"mon_allow_pool_delete":     "true",
			"debug_mon":                 "0",
		},
		"mgr": {
			"mgr_data":  filepath.Join(tmpDir, "mgr", "ceph-$id"),
			"debug_mgr": "0",
		},
		"osd": {
			"osd_data":        filepath.Join(tmpDir, "osd", "ceph-$id"),
			"osd_objectstore": "memstore",
			"debug_osd":       "0",
		},
		"client.rgw.rgw1": {
			"rgw_data":      filepath.Join(tmpDir, "rgw", "ceph-rgw1"),
			"rgw_frontends": "beast port=7480",
			"debug_rgw":     "0",
		},
	}

	keyringConfig := map[string]map[string]string{
		"mon.": {
			"key":      "AQBDm89oNP7bAxAA6TgZ1toOkhDjUNEkRL18Gg==",
			"caps mon": "allow *",
		},
		"client.admin": {
			"key":      "AQB5m89objcKIxAAda2ULz/l3NH+mv9XzKePHQ==",
			"caps mon": "allow *",
			"caps mds": "allow *",
			"caps osd": "allow *",
			"caps mgr": "allow *",
		},
		"mgr.mgr1": {
			"key":      "AQCDm89oNP7bAxAA6TgZ1toOkhDjUNEkRL18Gg==",
			"caps mon": "allow *",
			"caps osd": "allow *",
			"caps mds": "allow *",
		},
		"osd.0": {
			"key":      "AQCzsPFolNPNNhAAkglWKcr2qZB4lCK/u9A1Zw==",
			"caps mon": "allow profile osd",
			"caps mgr": "allow profile osd",
			"caps osd": "allow *",
		},
		"client.rgw.rgw1": {
			"key":      "AQDRm89oNP7bAxAA6TgZ1toOkhDjUNEkRL18Gg==",
			"caps mon": "allow rw",
			"caps osd": "allow rwx",
			"caps mgr": "allow rw",
		},
	}

	err := os.MkdirAll(filepath.Join(tmpDir, "mon"), 0o755)
	if err != nil {
		return confPath, err
	}

	err = os.MkdirAll(filepath.Join(tmpDir, "mgr", "ceph-mgr1"), 0o755)
	if err != nil {
		return confPath, err
	}

	err = os.MkdirAll(filepath.Join(tmpDir, "osd", "ceph-0"), 0o755)
	if err != nil {
		return confPath, err
	}

	err = os.MkdirAll(filepath.Join(tmpDir, "rgw", "ceph-rgw1"), 0o755)
	if err != nil {
		return confPath, err
	}

	confContent := generateINIConfig(cephConfig)
	err = os.WriteFile(confPath, []byte(confContent), 0o644)
	if err != nil {
		return confPath, err
	}

	keyringContent := generateINIConfig(keyringConfig)
	err = os.WriteFile(filepath.Join(tmpDir, "keyring"), []byte(keyringContent), 0o644)
	if err != nil {
		return confPath, err
	}

	monmapPath := filepath.Join(tmpDir, "monmap")
	cmd := exec.CommandContext(ctx, "monmaptool", "--conf", confPath, monmapPath, "--create", "--fsid", fsid)
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return confPath, fmt.Errorf("failed to create monitor map: %w", err)
	}

	cmd = exec.CommandContext(ctx, "monmaptool", "--conf", confPath, monmapPath, "--add", "mon1", "127.0.0.1:6789")
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return confPath, fmt.Errorf("failed to add monitor to map: %w", err)
	}

	cmd = exec.CommandContext(ctx, "ceph-mon", "--conf", confPath, "--mkfs", "--id", "mon1", "--monmap", monmapPath, "--keyring", filepath.Join(tmpDir, "keyring"))
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return confPath, fmt.Errorf("failed to initialize monitor filesystem: %w", err)
	}

	err = os.Remove(monmapPath)
	if err != nil {
		return confPath, err
	}

	return confPath, nil
}

func generateINIConfig(config map[string]map[string]string) string {
	var result strings.Builder

	sections := make([]string, 0, len(config))
	for section := range config {
		sections = append(sections, section)
	}
	sort.Strings(sections)

	for i, section := range sections {
		if i > 0 {
			result.WriteString("\n")
		}
		result.WriteString(fmt.Sprintf("[%s]\n", section))

		keys := make([]string, 0, len(config[section]))
		for key := range config[section] {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		for _, key := range keys {
			result.WriteString(fmt.Sprintf("%s = %s\n", key, config[section][key]))
		}
	}

	return result.String()
}

func startCephMon(wg *sync.WaitGroup, ctx context.Context, confPath string, out io.Writer) error {
	cmd := exec.CommandContext(ctx, "ceph-mon", "--conf", confPath, "--id", "mon1", "--foreground")
	cmd.Stdout = out
	cmd.Stderr = out

	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to spawn ceph-mon: %w", err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = cmd.Wait()
	}()

	return nil
}

func waitForCephMon(ctx context.Context, confPath string) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if status, err := checkCephStatus(ctx, confPath); err == nil && status.Monmap.NumMons > 0 {
				return nil
			}
		}
	}
}

func startCephOsd(wg *sync.WaitGroup, ctx context.Context, confPath string, tmpDir string, out io.Writer) error {
	cmd := exec.CommandContext(ctx, "ceph-osd", "--conf", confPath, "--id", "0", "--mkfs")
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to initialize OSD filesystem: %w", err)
	}

	cmd = exec.CommandContext(ctx, "ceph-osd", "--conf", confPath, "--id", "0", "--foreground")
	cmd.Stdout = out
	cmd.Stderr = out

	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start OSD: %w", err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = cmd.Wait()
	}()

	return nil
}

func waitForCephOsd(ctx context.Context, confPath string) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if status, err := checkCephStatus(ctx, confPath); err == nil && status.Osdmap.NumUpOsds > 0 {
				return nil
			}
		}
	}
}

func startCephMgr(wg *sync.WaitGroup, ctx context.Context, confPath string, out io.Writer) error {
	cmd := exec.CommandContext(ctx, "ceph-mgr", "--conf", confPath, "--id", "mgr1", "--foreground")
	cmd.Stdout = out
	cmd.Stderr = out

	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start MGR: %w", err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = cmd.Wait()
	}()

	return nil
}

func waitForCephMgr(ctx context.Context, confPath string) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if status, err := checkCephStatus(ctx, confPath); err == nil && status.Mgrmap.Available {
				return nil
			}
		}
	}
}

func startCephRgw(wg *sync.WaitGroup, ctx context.Context, confPath string, out io.Writer) error {
	cmd := exec.CommandContext(ctx, "radosgw", "--conf", confPath, "--id", "rgw.rgw1", "--foreground")
	cmd.Stdout = out
	cmd.Stderr = out

	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start RGW: %w", err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = cmd.Wait()
	}()

	return nil
}

func waitForCephRgw(ctx context.Context, confPath string) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	client := &http.Client{Timeout: 500 * time.Millisecond}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			resp, err := client.Head("http://127.0.0.1:7480/")
			if resp != nil {
				_ = resp.Body.Close()
			}
			if err == nil {
				return nil
			}
		}
	}
}

func enableCephDashboard(ctx context.Context, confPath string, out io.Writer) (string, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", confPath, "mgr", "module", "enable", "dashboard")
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to enable dashboard module: %w", err)
	}

	cmd = exec.CommandContext(ctx, "ceph", "--conf", confPath, "config", "set", "mgr", "mgr/dashboard/ssl", "false")
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to disable dashboard SSL: %w", err)
	}

	cmd = exec.CommandContext(ctx, "ceph", "--conf", confPath, "dashboard", "ac-user-create", "admin", "-i", "/dev/stdin", "administrator")
	cmd.Stdin = strings.NewReader("password")
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to create dashboard user: %w", err)
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			status, err := checkCephStatus(ctx, confPath)
			if err != nil {
				continue
			}
			if url, ok := status.Mgrmap.Services["dashboard"]; ok {
				return url, nil
			}
		}
	}
}

type cephStatus struct {
	Mgrmap cephStatusMgrmap `json:"mgrmap"`
	Monmap cephStatusMonmap `json:"monmap"`
	Osdmap cephStatusOsdmap `json:"osdmap"`
}

type cephStatusMonmap struct {
	NumMons int `json:"num_mons"`
}

type cephStatusMgrmap struct {
	Available bool              `json:"available"`
	Services  map[string]string `json:"services"`
}

type cephStatusOsdmap struct {
	NumUpOsds int `json:"num_up_osds"`
}

func checkCephStatus(ctx context.Context, confPath string) (cephStatus, error) {
	statusCmd := exec.CommandContext(ctx, "ceph", "--conf", confPath, "status", "--format", "json")
	output, err := statusCmd.Output()
	if err != nil {
		return cephStatus{}, err
	}

	var status cephStatus
	err = json.Unmarshal(output, &status)
	if err != nil {
		return cephStatus{}, err
	}

	return status, err
}

type TestWriter struct {
	t *testing.T
}

func (tw *TestWriter) Write(p []byte) (n int, err error) {
	tw.t.Helper()
	tw.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}

type LogDemux struct {
	mu   sync.Mutex
	outs sync.Map
}

func (log *LogDemux) Write(p []byte) (n int, err error) {
	log.mu.Lock()
	defer log.mu.Unlock()

	var writeErr error
	log.outs.Range(func(key, _ interface{}) bool {
		if writer, ok := key.(io.Writer); ok {
			if written, err := writer.Write(p); err != nil {
				writeErr = err
				return false
			} else if written != len(p) {
				writeErr = fmt.Errorf("short write: expected %d, got %d", len(p), written)
				return false
			}
		}
		return true
	})

	if writeErr != nil {
		return 0, writeErr
	}
	return len(p), nil
}

func (log *LogDemux) Attach(writer io.Writer) func() {
	log.outs.Store(writer, struct{}{})
	return func() {
		log.outs.Delete(writer)
	}
}

func (log *LogDemux) AttachTestFunction(t *testing.T) func() {
	w := &TestWriter{t: t}
	log.outs.Store(w, struct{}{})
	return func() {
		log.outs.Delete(w)
	}
}

func TestAccProvider_missingAuthentication(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: config.Variables{
					"endpoint": config.StringVariable(testDashboardURL),
				},
				Config: `
					variable "endpoint" {
					  type = string
					}

					provider "ceph" {
					  endpoint = var.endpoint
					}

					data "ceph_auth" "test" {
					  entity = "client.admin"
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)either token or both username and password must be configured`),
			},
		},
	})
}

func TestAccProvider_missingEndpoint(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					provider "ceph" {
					  username = "admin"
					  password = "password"
					}

					data "ceph_auth" "test" {
					  entity = "client.admin"
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)a provider endpoint must be configured`),
			},
		},
	})
}

func TestAccProvider_invalidEndpointURL(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					provider "ceph" {
					  endpoint = "://invalid-url"
					  username = "admin"
					  password = "password"
					}

					data "ceph_auth" "test" {
					  entity = "client.admin"
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)unable to parse endpoint url`),
			},
		},
	})
}

func TestAccProvider_endpointWithApiSuffix(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					provider "ceph" {
					  endpoint = "https://ceph.example.com/api"
					  username = "admin"
					  password = "password"
					}

					data "ceph_auth" "test" {
					  entity = "client.admin"
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)endpoint should not end with '/api'`),
			},
		},
	})
}

func TestAccProvider_authenticationFailure(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				ConfigVariables: config.Variables{
					"endpoint": config.StringVariable(testDashboardURL),
				},
				Config: `
					variable "endpoint" {
					  type = string
					}

					provider "ceph" {
					  endpoint = var.endpoint
					  username = "admin"
					  password = "wrongpassword"
					}

					data "ceph_auth" "test" {
					  entity = "client.admin"
					}
				`,
				ExpectError: regexp.MustCompile(`(?i)failed to configure ceph api client`),
			},
		},
	})
}

func TestAccProvider_tokenAuthentication(t *testing.T) {
	detachLogs := cephDaemonLogs.AttachTestFunction(t)
	defer detachLogs()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck: func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			client := &CephAPIClient{}
			endpoint, err := url.Parse(testDashboardURL)
			if err != nil {
				t.Fatalf("Failed to parse test dashboard URL: %v", err)
			}

			if err := client.Configure(ctx, []*url.URL{endpoint}, "admin", "password", ""); err != nil {
				t.Fatalf("Failed to configure client: %v", err)
			}

			authToken := client.token
			if authToken == "" {
				t.Fatal("Failed to obtain auth token")
			}

			t.Setenv("TF_VAR_endpoint", testDashboardURL)
			t.Setenv("TF_VAR_token", authToken)
		},
		Steps: []resource.TestStep{
			{
				Config: `
					variable "endpoint" {
					  type = string
					}

					variable "token" {
					  type = string
					}

					provider "ceph" {
					  endpoint = var.endpoint
					  token    = var.token
					}

					data "ceph_auth" "test" {
					  entity = "client.admin"
					}
				`,
			},
		},
	})
}
