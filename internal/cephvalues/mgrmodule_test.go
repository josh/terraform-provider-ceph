package cephvalues

import (
	"encoding/json"
	"testing"
)

func TestMgrModuleString(t *testing.T) {
	tests := []struct {
		prior string
		val   any
		want  string
	}{
		{"1", true, "1"},
		{"true", true, "true"},
		{"True", true, "True"},

		// The mgr's read-time coercion treats these as false until the
		// mon's normalized value replaces the cached raw value, so they
		// must not be preserved as true values.
		{"TRUE", true, "true"},
		{"on", true, "true"},
		{"yes", true, "true"},
		{" 1", true, "true"},
		{"2", true, "true"},

		{"0", false, "0"},
		{"false", false, "false"},
		{"False", false, "False"},
		{"FALSE", false, "FALSE"},
		{"off", false, "false"},
		{"no", false, "false"},
		{"TRUE", false, "false"},
		{"1", false, "false"},

		{"8080", json.Number("8080"), "8080"},
		{"+8080", json.Number("8080"), "+8080"},
		{"9007199254740993", json.Number("9007199254740993"), "9007199254740993"},
		{"9007199254740993", json.Number("9007199254740992"), "9007199254740992"},
		// PyLong base 0 rejects leading zeros, so the cached raw value
		// fails the mgr's read-time coercion.
		{"08080", json.Number("8080"), "8080"},
		{"1K", json.Number("1000"), "1000"},
		{"3", json.Number("2"), "2"},

		{"2.0", json.Number("2.0"), "2.0"},
		{"2", json.Number("2.0"), "2"},
		{"0.1234567", json.Number("0.1234567"), "0.1234567"},
		{"0.1234567", json.Number("0.123457"), "0.1234567"},
		{"1e3", json.Number("1000.0"), "1e3"},
		{"2.5", json.Number("2.51"), "2.51"},

		{"2.0", float64(2), "2.0"},
		{"0.1234567", float64(0.123457), "0.1234567"},
		{"abc", float64(2), "2"},

		{"hello", "hello", "hello"},
		{"other", "hello", "hello"},
		{"", nil, ""},
	}

	for _, tt := range tests {
		got, err := MgrModuleString(tt.prior, tt.val)
		if err != nil {
			t.Errorf("MgrModuleString(%q, %v) returned error: %v", tt.prior, tt.val, err)
			continue
		}
		if got != tt.want {
			t.Errorf("MgrModuleString(%q, %v) = %q, want %q", tt.prior, tt.val, got, tt.want)
		}
	}

	if _, err := MgrModuleString("x", struct{}{}); err == nil {
		t.Error("MgrModuleString with unsupported type should return an error")
	}
}

func TestFormatMgrModuleValue(t *testing.T) {
	tests := []struct {
		val  any
		want string
	}{
		{nil, ""},
		{true, "true"},
		{false, "false"},
		{json.Number("8080"), "8080"},
		{json.Number("9007199254740993"), "9007199254740993"},
		{json.Number("2.5"), "2.5"},
		{float64(8080), "8080"},
		{float64(2.5), "2.5"},
		{int64(-3), "-3"},
		{uint32(7), "7"},
		{"text", "text"},
	}

	for _, tt := range tests {
		got, err := FormatMgrModuleValue(tt.val)
		if err != nil {
			t.Errorf("FormatMgrModuleValue(%v) returned error: %v", tt.val, err)
			continue
		}
		if got != tt.want {
			t.Errorf("FormatMgrModuleValue(%v) = %q, want %q", tt.val, got, tt.want)
		}
	}
}
