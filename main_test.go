package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

var (
	testDashboardURL string
	testClusterWG    *sync.WaitGroup
	testConfPath     string
	testTimeout      = flag.Duration("timeout", 0, "test timeout")
)

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

		var confPath string
		testDashboardURL, confPath, testClusterWG, err = startCephCluster(ctx, tmpDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to start ceph cluster: %v\n", err)
			os.RemoveAll(tmpDir)
			os.Exit(1)
		}
		testConfPath = confPath

		code = m.Run()

		cancel()
		testClusterWG.Wait()
		os.RemoveAll(tmpDir)
	} else {
		code = m.Run()
	}

	os.Exit(code)
}

func TestAccCephAuthDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"ceph": providerserver.NewProtocol6WithError(providerFunc()),
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					provider "ceph" {
					  endpoint = %q
					  username = "admin"
					  password = "password"
					}

					data "ceph_auth" "client_admin" {
					  entity = "client.admin"
					}
				`, testDashboardURL),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ceph_auth.client_admin",
						tfjsonpath.New("entity"),
						knownvalue.StringExact("client.admin"),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_auth.client_admin",
						tfjsonpath.New("key"),
						knownvalue.StringExact("AQB5m89objcKIxAAda2ULz/l3NH+mv9XzKePHQ=="),
					),
					statecheck.ExpectKnownValue(
						"data.ceph_auth.client_admin",
						tfjsonpath.New("caps"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"mon": knownvalue.StringExact("allow *"),
							"mds": knownvalue.StringExact("allow *"),
							"osd": knownvalue.StringExact("allow *"),
							"mgr": knownvalue.StringExact("allow *"),
						}),
					),
				},
			},
		},
	})
}

func TestAccCephAuthResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: map[string]func() (tfprotov6.ProviderServer, error){
			"ceph": providerserver.NewProtocol6WithError(providerFunc()),
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					provider "ceph" {
					  endpoint = %q
					  username = "admin"
					  password = "password"
					}

					resource "ceph_auth" "test" {
					  entity = "client.foo"
					  caps = {
					    mon = "allow r"
					    osd = "allow rw pool=foo"
					  }
					}
				`, testDashboardURL),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_auth.test",
						tfjsonpath.New("entity"),
						knownvalue.StringExact("client.foo"),
					),
					statecheck.ExpectKnownValue(
						"ceph_auth.test",
						tfjsonpath.New("caps"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"mon": knownvalue.StringExact("allow r"),
							"osd": knownvalue.StringExact("allow rw pool=foo"),
						}),
					),
					statecheck.ExpectKnownValue(
						"ceph_auth.test",
						tfjsonpath.New("key"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ceph_auth.test",
						tfjsonpath.New("keyring"),
						knownvalue.NotNull(),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephAuthExists(t, "client.foo"),
					checkCephAuthHasCaps(t, "client.foo", map[string]string{
						"mon": "allow r",
						"osd": "allow rw pool=foo",
					}),
				),
			},
			{
				Config: fmt.Sprintf(`
					provider "ceph" {
					  endpoint = %q
					  username = "admin"
					  password = "password"
					}

					resource "ceph_auth" "test" {
					  entity = "client.foo"
					  caps = {
					    mon = "allow rw"
					    osd = "allow rw pool=bar"
					    mds = "allow rw"
					  }
					}
				`, testDashboardURL),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ceph_auth.test",
						tfjsonpath.New("entity"),
						knownvalue.StringExact("client.foo"),
					),
					statecheck.ExpectKnownValue(
						"ceph_auth.test",
						tfjsonpath.New("caps"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"mon": knownvalue.StringExact("allow rw"),
							"osd": knownvalue.StringExact("allow rw pool=bar"),
							"mds": knownvalue.StringExact("allow rw"),
						}),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					checkCephAuthExists(t, "client.foo"),
					checkCephAuthHasCaps(t, "client.foo", map[string]string{
						"mon": "allow rw",
						"osd": "allow rw pool=bar",
						"mds": "allow rw",
					}),
				),
			},
			{
				Config: fmt.Sprintf(`
					provider "ceph" {
					  endpoint = %q
					  username = "admin"
					  password = "password"
					}
				`, testDashboardURL),
				Check: checkCephAuthNotExists(t, "client.foo"),
			},
		},
	})
}

func startCephCluster(ctx context.Context, tmpDir string) (string, string, *sync.WaitGroup, error) {
	startupCtx, startupCancel := context.WithTimeout(ctx, 30*time.Second)
	defer startupCancel()

	confPath, err := setupCephDir(startupCtx, tmpDir)
	if err != nil {
		return "", "", nil, err
	}

	var wg sync.WaitGroup

	if err := startCephMon(&wg, ctx, confPath); err != nil {
		return "", "", nil, err
	}

	if err := waitForCephMon(startupCtx, confPath); err != nil {
		return "", "", nil, err
	}

	if err := startCephMgr(&wg, ctx, confPath); err != nil {
		return "", "", nil, err
	}

	if err := waitForCephMgr(startupCtx, confPath); err != nil {
		return "", "", nil, err
	}

	dashboardURL, err := enableCephDashboard(startupCtx, confPath)
	if err != nil {
		return "", "", nil, err
	}

	return dashboardURL, confPath, &wg, nil
}

func setupCephDir(ctx context.Context, tmpDir string) (string, error) {
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
		},
		"mgr": {
			"mgr_data": filepath.Join(tmpDir, "mgr", "ceph-$id"),
		},
		"osd": {
			"osd_data":        filepath.Join(tmpDir, "osd", "ceph-$id"),
			"osd_objectstore": "memstore",
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
	if err := cmd.Run(); err != nil {
		return confPath, fmt.Errorf("failed to create monitor map: %w", err)
	}

	cmd = exec.CommandContext(ctx, "monmaptool", "--conf", confPath, monmapPath, "--add", "mon1", "127.0.0.1:6789")
	if err := cmd.Run(); err != nil {
		return confPath, fmt.Errorf("failed to add monitor to map: %w", err)
	}

	cmd = exec.CommandContext(ctx, "ceph-mon", "--conf", confPath, "--mkfs", "--id", "mon1", "--monmap", monmapPath, "--keyring", filepath.Join(tmpDir, "keyring"))
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

func startCephMon(wg *sync.WaitGroup, ctx context.Context, confPath string) error {
	cmd := exec.CommandContext(ctx, "ceph-mon", "--conf", confPath, "--id", "mon1", "--foreground")

	if testing.Verbose() {
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
	}

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

func startCephMgr(wg *sync.WaitGroup, ctx context.Context, confPath string) error {
	cmd := exec.CommandContext(ctx, "ceph-mgr", "--conf", confPath, "--id", "mgr1", "--foreground")

	if testing.Verbose() {
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
	}

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

func enableCephDashboard(ctx context.Context, confPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", confPath, "mgr", "module", "enable", "dashboard")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to enable dashboard module: %w", err)
	}

	cmd = exec.CommandContext(ctx, "ceph", "--conf", confPath, "config", "set", "mgr", "mgr/dashboard/ssl", "false")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to disable dashboard SSL: %w", err)
	}

	cmd = exec.CommandContext(ctx, "ceph", "--conf", confPath, "dashboard", "ac-user-create", "admin", "-i", "/dev/stdin", "administrator")
	cmd.Stdin = strings.NewReader("password")
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

func checkCephAuthExists(t *testing.T, entity string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "ceph", "--conf", testConfPath, "auth", "get", entity, "--format", "json")
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("auth entity %s does not exist: %w", entity, err)
		}

		var authData []struct {
			Entity string            `json:"entity"`
			Key    string            `json:"key"`
			Caps   map[string]string `json:"caps"`
		}
		if err := json.Unmarshal(output, &authData); err != nil {
			return fmt.Errorf("failed to parse auth output: %w", err)
		}

		if len(authData) == 0 {
			return fmt.Errorf("auth entity %s not found in output", entity)
		}

		t.Logf("Verified auth entity %s exists with caps: %v", entity, authData[0].Caps)
		return nil
	}
}

func checkCephAuthHasCaps(t *testing.T, entity string, expectedCaps map[string]string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "ceph", "--conf", testConfPath, "auth", "get", entity, "--format", "json")
		output, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("auth entity %s does not exist: %w", entity, err)
		}

		var authData []struct {
			Entity string            `json:"entity"`
			Key    string            `json:"key"`
			Caps   map[string]string `json:"caps"`
		}
		if err := json.Unmarshal(output, &authData); err != nil {
			return fmt.Errorf("failed to parse auth output: %w", err)
		}

		if len(authData) == 0 {
			return fmt.Errorf("auth entity %s not found in output", entity)
		}

		actualCaps := authData[0].Caps
		for capType, expectedCap := range expectedCaps {
			if actualCap, ok := actualCaps[capType]; !ok {
				return fmt.Errorf("expected cap %s not found for entity %s", capType, entity)
			} else if actualCap != expectedCap {
				return fmt.Errorf("cap %s mismatch for entity %s: expected %q, got %q", capType, entity, expectedCap, actualCap)
			}
		}

		t.Logf("Verified auth entity %s has correct caps: %v", entity, actualCaps)
		return nil
	}
}

func checkCephAuthNotExists(t *testing.T, entity string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "ceph", "--conf", testConfPath, "auth", "get", entity, "--format", "json")
		output, err := cmd.Output()
		if err == nil {
			return fmt.Errorf("auth entity %s still exists (output: %s)", entity, string(output))
		}

		t.Logf("Verified auth entity %s does not exist", entity)
		return nil
	}
}
