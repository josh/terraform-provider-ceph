package cephvalues

import "testing"

func TestMgrModuleString(t *testing.T) {
	tests := []struct {
		prior string
		val   any
		want  string
	}{
		{"1", true, "1"},
		{"true", true, "true"},
		{"True", true, "True"},
		{"on", true, "on"},
		{"yes", true, "yes"},
		{"0", false, "0"},
		{"off", false, "off"},
		{"no", false, "no"},
		{"TRUE", true, "true"},
		{"TRUE", false, "false"},
		{"1", false, "false"},
		{"yes", false, "false"},

		{"8080", float64(8080), "8080"},
		{"2.0", float64(2), "2.0"},
		{"08080", float64(8080), "08080"},
		{"2.5", float64(2.5), "2.5"},
		{"3", float64(2), "2"},
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
