package cephvalues

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// FormatMgrModuleValue renders a typed mgr module option value (as decoded
// from the dashboard's JSON response) back into a string.
func FormatMgrModuleValue(val any) (string, error) {
	switch v := val.(type) {
	case nil:
		return "", nil
	case json.Number:
		return string(v), nil
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

// MgrModuleString keeps the prior raw value of an mgr module option when
// it means the same value as the typed read-back, so applies stay
// consistent and plans converge. The mon validates and normalizes the
// value at set time, but the mgr serves the raw value from its local
// cache until the next config refresh and coerces whichever string it
// holds typed at read time (src/mgr/PyUtil.cc). A raw value is therefore
// preserved only when the raw and normalized readings agree: bools via
// the read-time exact-case true list intersected with the mon's parse,
// integers via exact base-10 comparison without leading zeros (PyLong
// base 0 rejects them), floats rounded to the mon's six-decimal
// normalized form.
func MgrModuleString(prior string, val any) (string, error) {
	formatted, err := FormatMgrModuleValue(val)
	if err != nil || prior == formatted {
		return formatted, err
	}

	equal := false
	switch v := val.(type) {
	case bool:
		switch prior {
		case "1", "true", "True":
			equal = v
		default:
			if pv, ok := parseBool(prior); ok && !pv {
				equal = !v
			}
		}
	case json.Number:
		if strings.ContainsAny(string(v), ".eE") {
			equal = floatValuesEqual(prior, string(v))
		} else {
			equal = intValuesEqual(prior, string(v))
		}
	case float32, float64:
		equal = floatValuesEqual(prior, formatted)
	}

	if equal {
		return prior, nil
	}
	return formatted, nil
}

func intValuesEqual(a, b string) bool {
	digits := strings.TrimLeft(strings.TrimSpace(a), "+-")
	if len(digits) > 1 && digits[0] == '0' {
		return false
	}

	av, aerr := strconv.ParseInt(strings.TrimSpace(a), 10, 64)
	bv, berr := strconv.ParseInt(b, 10, 64)
	return aerr == nil && berr == nil && av == bv
}

func floatValuesEqual(a, b string) bool {
	av, aerr := strconv.ParseFloat(strings.TrimSpace(a), 64)
	bv, berr := strconv.ParseFloat(b, 64)
	return aerr == nil && berr == nil &&
		strconv.FormatFloat(av, 'f', 6, 64) == strconv.FormatFloat(bv, 'f', 6, 64)
}
