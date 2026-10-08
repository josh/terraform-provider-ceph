package keyring

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	KeyTypeAES     = "aes"
	KeyTypeAES256K = "aes256k"
)

// A cephx key is base64 of Ceph's CryptoKey encoding: u16 type, u32 sec,
// u32 nsec, u16 secret length, secret bytes; all little-endian.
func GenerateKey(keyType string) (string, error) {
	var typeID uint16
	var secretLen int
	switch keyType {
	case KeyTypeAES:
		typeID, secretLen = 1, 16
	case KeyTypeAES256K:
		typeID, secretLen = 2, 32
	default:
		return "", fmt.Errorf("unsupported cephx key type %q", keyType)
	}

	secret := make([]byte, secretLen)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("unable to generate cephx key: %w", err)
	}

	now := time.Now()
	buf := binary.LittleEndian.AppendUint16(nil, typeID)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(now.Unix()))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(now.Nanosecond()))
	buf = binary.LittleEndian.AppendUint16(buf, uint16(secretLen))
	buf = append(buf, secret...)
	return base64.StdEncoding.EncodeToString(buf), nil
}

func KeyType(key string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return "", fmt.Errorf("invalid cephx key: %w", err)
	}
	if len(raw) < 12 {
		return "", fmt.Errorf("invalid cephx key: %d bytes is too short", len(raw))
	}
	switch typeID := binary.LittleEndian.Uint16(raw); typeID {
	case 0:
		return "none", nil
	case 1:
		return KeyTypeAES, nil
	case 2:
		return KeyTypeAES256K, nil
	default:
		return "", fmt.Errorf("unsupported cephx key type %d", typeID)
	}
}

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
					// ceph auth export escapes embedded quotes.
					capsValue = strings.ReplaceAll(capsValue, `\"`, `"`)
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

	// Escape embedded quotes the same way ceph auth export does.
	escape := func(caps string) string {
		return strings.ReplaceAll(caps, `"`, `\"`)
	}

	for i, user := range users {
		if i > 0 {
			result.WriteString("\n")
		}

		result.WriteString(fmt.Sprintf("[%s]\n", user.Entity))
		result.WriteString(fmt.Sprintf("\tkey = %s\n", user.Key))

		if user.Caps.MDS != "" {
			result.WriteString(fmt.Sprintf("\tcaps mds = \"%s\"\n", escape(user.Caps.MDS)))
		}
		if user.Caps.MGR != "" {
			result.WriteString(fmt.Sprintf("\tcaps mgr = \"%s\"\n", escape(user.Caps.MGR)))
		}
		if user.Caps.MON != "" {
			result.WriteString(fmt.Sprintf("\tcaps mon = \"%s\"\n", escape(user.Caps.MON)))
		}
		if user.Caps.OSD != "" {
			result.WriteString(fmt.Sprintf("\tcaps osd = \"%s\"\n", escape(user.Caps.OSD)))
		}
	}

	return result.String()
}
