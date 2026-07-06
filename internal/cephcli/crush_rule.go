package cephcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

type CrushRule struct {
	RuleName string `json:"rule_name"`
}

func (c *CLI) CrushRuleCreateReplicated(ctx context.Context, name, root, failureDomain string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "crush", "rule", "create-replicated", name, root, failureDomain)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create replicated crush rule %s: %w", name, err)
	}

	rule, err := c.CrushRuleDump(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to verify crush rule: %w", err)
	}
	if rule.RuleName != name {
		return fmt.Errorf("crush rule name mismatch: expected %s, got %s", name, rule.RuleName)
	}
	return nil
}

func (c *CLI) CrushRuleCreateSimple(ctx context.Context, name, root, failureDomain string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "crush", "rule", "create-simple", name, root, failureDomain)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create simple crush rule %s: %w", name, err)
	}

	rule, err := c.CrushRuleDump(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to verify crush rule: %w", err)
	}
	if rule.RuleName != name {
		return fmt.Errorf("crush rule name mismatch: expected %s, got %s", name, rule.RuleName)
	}
	return nil
}

func (c *CLI) CrushRuleCreateErasure(ctx context.Context, name, profile string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "crush", "rule", "create-erasure", name, profile)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create erasure crush rule %s: %w", name, err)
	}

	rule, err := c.CrushRuleDump(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to verify crush rule: %w", err)
	}
	if rule.RuleName != name {
		return fmt.Errorf("crush rule name mismatch: expected %s, got %s", name, rule.RuleName)
	}
	return nil
}

func (c *CLI) CrushRuleDump(ctx context.Context, name string) (*CrushRule, error) {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "crush", "rule", "dump", name, "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to dump crush rule %s: %w", name, err)
	}

	var rule CrushRule
	if err := json.Unmarshal(output, &rule); err != nil {
		return nil, fmt.Errorf("failed to parse crush rule output: %w", err)
	}

	return &rule, nil
}

func (c *CLI) CrushRuleList(ctx context.Context) ([]string, error) {
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

func (c *CLI) CrushRuleRemove(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "ceph", "--conf", c.confPath, "osd", "crush", "rule", "rm", name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove crush rule %s: %w", name, err)
	}

	_, err := c.CrushRuleDump(ctx, name)
	if err == nil {
		return fmt.Errorf("crush rule still exists after removal: %s", name)
	}
	return nil
}
