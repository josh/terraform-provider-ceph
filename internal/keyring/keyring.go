package keyring

import (
	"fmt"
	"regexp"
	"strings"
)

type Caps struct {
	MDS string `json:"mds,omitempty"`
	MGR string `json:"mgr,omitempty"`
	MON string `json:"mon,omitempty"`
	OSD string `json:"osd,omitempty"`
}

func (c Caps) Map() map[string]string {
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

func CapsFromMap(capabilities map[string]string) (Caps, error) {
	var caps Caps

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
			return Caps{}, fmt.Errorf("caps attribute contains unsupported capability type %q", capType)
		}
	}

	return caps, nil
}

func MustCapsFromMap(capabilities map[string]string) Caps {
	caps, err := CapsFromMap(capabilities)
	if err != nil {
		panic(err)
	}
	return caps
}

type User struct {
	Entity string `json:"entity"`
	Key    string `json:"key"`
	Caps   Caps   `json:"caps"`
}

func Parse(content string) ([]User, error) {
	users := []User{}
	var cur *User

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
			cur = &User{
				Entity: matches[1],
				Key:    "",
				Caps:   Caps{},
			}
		} else if cur != nil {
			if matches := keyRegex.FindStringSubmatch(line); matches != nil {
				cur.Key = strings.TrimSpace(matches[1])
			} else if matches := capsRegex.FindStringSubmatch(line); matches != nil {
				capType := matches[1]
				capsValue := strings.TrimSpace(matches[2])
				if len(capsValue) >= 2 && strings.HasPrefix(capsValue, `"`) && strings.HasSuffix(capsValue, `"`) {
					capsValue = capsValue[1 : len(capsValue)-1]
				}

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

func Format(users []User) string {
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
