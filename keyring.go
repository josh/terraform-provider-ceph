package main

import (
	"fmt"
	"regexp"
	"strings"
)

type CephUser struct {
	Entity string            `json:"entity"`
	Key    string            `json:"key"`
	Caps   map[string]string `json:"caps"`
}

func parseCephKeyring(content string) ([]CephUser, error) {
	users := []CephUser{}
	var cur *CephUser

	entityRegex := regexp.MustCompile(`^\[([^\]]+)\]$`)
	keyRegex := regexp.MustCompile(`^key\s*=\s*(.*)$`)
	capsRegex := regexp.MustCompile(`^caps\s+(\w+)\s*=\s*(.*)$`)

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		originalLine := line
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if matches := entityRegex.FindStringSubmatch(line); matches != nil {
			if cur != nil {
				users = append(users, *cur)
			}
			cur = &CephUser{
				Entity: matches[1],
				Key:    "",
				Caps:   make(map[string]string),
			}
		} else if cur != nil {
			if matches := keyRegex.FindStringSubmatch(line); matches != nil {
				cur.Key = strings.TrimSpace(matches[1])
			} else if matches := capsRegex.FindStringSubmatch(line); matches != nil {
				capsValue := strings.Trim(strings.TrimSpace(matches[2]), `"`)
				cur.Caps[matches[1]] = capsValue
			}
		} else {
			return nil, fmt.Errorf("parse error:%d:%s", i+1, originalLine)
		}
	}

	if cur != nil {
		users = append(users, *cur)
	}

	if len(users) == 0 {
		return nil, fmt.Errorf("invalid keyring format: no valid entity sections found (expected format: [entity.name] followed by key and caps)")
	}

	return users, nil
}
