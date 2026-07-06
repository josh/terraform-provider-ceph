package cephcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

type healthStatus struct {
	Mgrmap healthStatusMgrmap `json:"mgrmap"`
	Monmap healthStatusMonmap `json:"monmap"`
	Osdmap healthStatusOsdmap `json:"osdmap"`
}

type healthStatusMonmap struct {
	NumMons int `json:"num_mons"`
}

type healthStatusMgrmap struct {
	Available bool `json:"available"`
}

type healthStatusOsdmap struct {
	NumUpOsds int `json:"num_up_osds"`
}

func (c *CLI) CheckHealth(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "status", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check cluster status: %w", err)
	}

	var status healthStatus
	if err := json.Unmarshal(output, &status); err != nil {
		return fmt.Errorf("failed to parse cluster status: %w", err)
	}

	if status.Monmap.NumMons == 0 {
		return fmt.Errorf("no monitors available")
	}
	if !status.Mgrmap.Available {
		return fmt.Errorf("manager not available")
	}
	if status.Osdmap.NumUpOsds == 0 {
		return fmt.Errorf("no OSDs available")
	}

	return nil
}

type PGStateCount struct {
	Name string `json:"name"`
	Num  int    `json:"num"`
}

type PGSummary struct {
	NumPGByState []PGStateCount `json:"num_pg_by_state"`
	NumPGs       int            `json:"num_pgs"`
}

type PGStat struct {
	PGReady   bool      `json:"pg_ready"`
	PGSummary PGSummary `json:"pg_summary"`
}

type ProgressEvent struct {
	Message  string  `json:"message"`
	Progress float64 `json:"progress"`
}

type ProgressJSON struct {
	Events []ProgressEvent `json:"events"`
}

func (c *CLI) ProgressJSON(ctx context.Context) (*ProgressJSON, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "progress", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get progress: %w", err)
	}

	var progress ProgressJSON
	if err := json.Unmarshal(output, &progress); err != nil {
		return nil, fmt.Errorf("failed to parse progress JSON: %w", err)
	}

	return &progress, nil
}

func (c *CLI) PGStat(ctx context.Context) (*PGStat, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "pg", "stat", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get pg stat: %w", err)
	}

	var stat PGStat
	if err := json.Unmarshal(output, &stat); err != nil {
		return nil, fmt.Errorf("failed to parse pg stat: %w", err)
	}

	return &stat, nil
}

type PGStateInfo struct {
	Total       int
	ActiveClean int
	Unhealthy   int
	ByState     map[string]int
}

func (c *CLI) PGStateInfo(ctx context.Context) (*PGStateInfo, error) {
	stat, err := c.PGStat(ctx)
	if err != nil {
		return nil, err
	}

	info := &PGStateInfo{
		Total:   stat.PGSummary.NumPGs,
		ByState: make(map[string]int),
	}

	for _, s := range stat.PGSummary.NumPGByState {
		info.ByState[s.Name] = s.Num
		if s.Name == "active+clean" {
			info.ActiveClean = s.Num
		} else {
			info.Unhealthy += s.Num
		}
	}

	return info, nil
}
