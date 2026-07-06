package cephvalues

import "testing"

func TestClusterEqual(t *testing.T) {
	tests := []struct {
		optType string
		a, b    string
		want    bool
	}{
		{"bool", "1", "true", true},
		{"bool", "0", "false", true},
		{"bool", "2", "true", true},
		{"bool", "-1", "true", true},
		{"bool", "TRUE", "true", true},
		{"bool", "FALSE", "false", true},
		{"bool", "1", "false", false},
		{"bool", "yes", "true", false},
		{"bool", "maybe", "true", false},

		{"float", "0.5", "0.500000", true},
		{"float", "2.0", "2", true},
		{"float", "0.1234567", "0.123457", true},
		{"float", "1e-7", "0.000000", true},
		{"float", "0.5", "0.51", false},
		{"float", "abc", "0.5", false},

		{"int", "1000", "1000", true},
		{"int", "1K", "1000", true},
		{"int", "1K", "1024", false},
		{"int", "1Ki", "1024", false},
		{"int", "100B", "100", true},
		{"int", "-1K", "-1000", true},
		{"uint", "5M", "5000000", true},
		{"uint", "1E", "1000000000000000000", true},
		{"uint", "10E", "10000000000000000000", false},

		{"size", "256Mi", "268435456", true},
		{"size", "256M", "268435456", true},
		{"size", "256MB", "268435456", true},
		{"size", "256MiB", "268435456", true},
		{"size", "1K", "1024", true},
		{"size", "1K", "1000", false},
		{"size", "100B", "100", true},
		{"size", "1G", "1073741824", true},
		{"size", "1G", "1000000000", false},
		{"size", "128Mi", "268435456", false},
		{"size", "1Bi", "1", false},

		{"secs", "1h", "3600", true},
		{"secs", "90s", "90", true},
		{"secs", "5m", "300", true},
		{"secs", "90min", "5400", true},
		{"secs", "1hr", "3600", true},
		{"secs", "1h 30m", "5400", true},
		{"secs", "1h30m", "5400", true},
		{"secs", "1d", "86400", true},
		{"secs", "1wk", "604800", true},
		{"secs", "1mo", "2592000", true},
		{"secs", "1y", "31536000", true},
		{"secs", "1h", "3601", false},
		{"secs", "1parsec", "1", false},

		{"millisecs", "500", "500", true},
		{"millisecs", "2s", "2", true},
		{"millisecs", "500ms", "500", true},
		{"millisecs", "2s", "2000", false},

		{"uuid", "A0EEBC99-9C0B-4EF8-BB6D-6BB9BD380A11", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", true},
		{"uuid", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12", false},

		{"addr", "1.2.3.4", "v2:1.2.3.4:0/0", true},
		{"addr", "1.2.3.4:6789", "v2:1.2.3.4:6789/0", true},
		{"addr", "v2:1.2.3.4:0/0", "v2:1.2.3.4:0/0", true},
		{"addr", "v1:1.2.3.4", "v2:1.2.3.4:0/0", false},
		{"addr", "1.2.3.4", "v2:1.2.3.5:0/0", false},

		{"str", "abc", "abc", true},
		{"str", "abc", "ABC", false},
		{"", "same", "same", true},
		{"", "a", "b", false},
	}

	for _, tt := range tests {
		if got := ClusterEqual(tt.optType, tt.a, tt.b); got != tt.want {
			t.Errorf("ClusterEqual(%q, %q, %q) = %v, want %v", tt.optType, tt.a, tt.b, got, tt.want)
		}
	}
}
