package cephvalues

import (
	"fmt"
	"strconv"
	"strings"
)

// FormatMgrModuleValue renders a typed mgr module option value (as decoded
// from the dashboard's JSON response) back into its string spelling.
func FormatMgrModuleValue(val any) (string, error) {
	switch v := val.(type) {
	case nil:
		return "", nil
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v)), nil
		}
		return strconv.FormatFloat(v, 'g', -1, 64), nil
	case float32:
		if v == float32(int32(v)) {
			return fmt.Sprintf("%d", int32(v)), nil
		}
		return strconv.FormatFloat(float64(v), 'g', -1, 32), nil
	case int, int8, int16, int32, int64:
		return fmt.Sprintf("%d", v), nil
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v), nil
	case bool:
		return fmt.Sprintf("%t", v), nil
	case string:
		return v, nil
	default:
		return "", fmt.Errorf("unsupported config value type: %T", v)
	}
}

// The mgr stores module options as raw strings and coerces them typed at
// read time (src/mgr/PyUtil.cc), so the read-back rarely matches the user's
// literal spelling (e.g. "1" reads back as bool true). MgrModuleString
// keeps the prior spelling when it means the same value so applies stay
// consistent and plans converge. The bool spellings mirror the mgr's
// exact-case true list; false spellings are safe because anything outside
// that list coerces to false.
func MgrModuleString(prior string, val any) (string, error) {
	formatted, err := FormatMgrModuleValue(val)
	if err != nil || prior == formatted {
		return formatted, err
	}

	equal := false
	switch v := val.(type) {
	case bool:
		switch strings.TrimSpace(prior) {
		case "1", "true", "True", "on", "yes":
			equal = v
		case "0", "false", "False", "off", "no":
			equal = !v
		}
	case float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		p, perr := strconv.ParseFloat(strings.TrimSpace(prior), 64)
		f, ferr := strconv.ParseFloat(formatted, 64)
		equal = perr == nil && ferr == nil && p == f
	}

	if equal {
		return prior, nil
	}
	return formatted, nil
}
