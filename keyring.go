package main

import (
	"fmt"
	"regexp"
	"strings"
)

type CephCaps struct {
	MDS string `json:"mds,omitempty"`
	MGR string `json:"mgr,omitempty"`
	MON string `json:"mon,omitempty"`
	OSD string `json:"osd,omitempty"`
}

func (c CephCaps) Map() map[string]string {
	result := make(map[string]string, 4)

	if c.MDS != "" {
		result["mds"] = c.MDS
	}
	if c.MGR != "" {
		result["mgr"] = c.MGR
	}
	if c.MON != "" {
		result["mon"] = c.MON
	}
	if c.OSD != "" {
		result["osd"] = c.OSD
	}

	return result
}

func NewCephCapsFromMap(capabilities map[string]string) (CephCaps, error) {
	var caps CephCaps

	for capType, capValue := range capabilities {
		lower := strings.ToLower(capType)

		switch lower {
		case "mds":
			caps.MDS = capValue
		case "mgr":
			caps.MGR = capValue
		case "mon":
			caps.MON = capValue
		case "osd":
			caps.OSD = capValue
		default:
			return CephCaps{}, fmt.Errorf("caps attribute contains unsupported capability type %q", capType)
		}
	}

	return caps, nil
}

func MustCephCapsFromMap(capabilities map[string]string) CephCaps {
	caps, err := NewCephCapsFromMap(capabilities)
	if err != nil {
		panic(err)
	}
	return caps
}

type CephUser struct {
	Entity string   `json:"entity"`
	Key    string   `json:"key"`
	Caps   CephCaps `json:"caps"`
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
				Caps:   CephCaps{},
			}
		} else if cur != nil {
			if matches := keyRegex.FindStringSubmatch(line); matches != nil {
				cur.Key = strings.TrimSpace(matches[1])
			} else if matches := capsRegex.FindStringSubmatch(line); matches != nil {
				capType := matches[1]
				capsValue := strings.Trim(strings.TrimSpace(matches[2]), `"`)

				lower := strings.ToLower(capType)
				switch lower {
				case "mds":
					cur.Caps.MDS = capsValue
				case "mgr":
					cur.Caps.MGR = capsValue
				case "mon":
					cur.Caps.MON = capsValue
				case "osd":
					cur.Caps.OSD = capsValue
				default:
					return nil, fmt.Errorf("parse error:%d:%s (unsupported capability type %q)", i+1, originalLine, capType)
				}
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

func formatCephKeyring(users []CephUser) string {
	var result strings.Builder

	for i, user := range users {
		if i > 0 {
			result.WriteString("\n")
		}

		result.WriteString(fmt.Sprintf("[%s]\n", user.Entity))
		result.WriteString(fmt.Sprintf("\tkey = %s\n", user.Key))

		if user.Caps.MDS != "" {
			result.WriteString(fmt.Sprintf("\tcaps mds = \"%s\"\n", user.Caps.MDS))
		}
		if user.Caps.MGR != "" {
			result.WriteString(fmt.Sprintf("\tcaps mgr = \"%s\"\n", user.Caps.MGR))
		}
		if user.Caps.MON != "" {
			result.WriteString(fmt.Sprintf("\tcaps mon = \"%s\"\n", user.Caps.MON))
		}
		if user.Caps.OSD != "" {
			result.WriteString(fmt.Sprintf("\tcaps osd = \"%s\"\n", user.Caps.OSD))
		}
	}

	return result.String()
}
