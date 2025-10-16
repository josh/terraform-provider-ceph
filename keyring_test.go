package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestEmptyKeyring(t *testing.T) {
	_, err := parseCephKeyring("")
	if err == nil {
		t.Errorf("parseCephKeyring() error = nil, wantErr non-nil")
	}
}

func TestParseClientAdminKeyring(t *testing.T) {
	text := `[client.admin]
key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
caps mds = "allow *"
caps mgr = "allow *"
caps mon = "allow *"
caps osd = "allow *"
`

	expected := []CephUser{
		{
			Entity: "client.admin",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCephCapsFromMap(map[string]string{"mds": "allow *", "mgr": "allow *", "mon": "allow *", "osd": "allow *"}),
		},
	}

	actual, err := parseCephKeyring(text)
	if err != nil {
		t.Errorf("parseCephKeyring() error = %v, wantErr nil", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("parseCephKeyring() = %v, want %v", actual, expected)
	}
}

func TestParseMultipleOSDsKeyring(t *testing.T) {
	text := `[osd.0]
key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
caps mgr = "allow profile osd"
caps mon = "allow profile osd"
caps osd = "allow *"

[osd.1]
key = BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB==
caps mgr = "allow profile osd"
caps mon = "allow profile osd"
caps osd = "allow *"`

	expected := []CephUser{
		{
			Entity: "osd.0",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCephCapsFromMap(map[string]string{"mgr": "allow profile osd", "mon": "allow profile osd", "osd": "allow *"}),
		},
		{
			Entity: "osd.1",
			Key:    "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB==",
			Caps:   MustCephCapsFromMap(map[string]string{"mgr": "allow profile osd", "mon": "allow profile osd", "osd": "allow *"}),
		},
	}

	actual, err := parseCephKeyring(text)
	if err != nil {
		t.Errorf("parseCephKeyring() error = %v, wantErr nil", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("parseCephKeyring() = %v, want %v", actual, expected)
	}
}

func TestParseClientFooKeyring(t *testing.T) {
	text := `[client.foo]
	key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
	caps mds = "allow rw path=/"
	caps mon = "allow rw"
	caps osd = "allow rwx"
`

	expected := []CephUser{
		{
			Entity: "client.foo",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCephCapsFromMap(map[string]string{"mds": "allow rw path=/", "mon": "allow rw", "osd": "allow rwx"}),
		},
	}

	actual, err := parseCephKeyring(text)
	if err != nil {
		t.Errorf("parseCephKeyring() error = %v, wantErr nil", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("parseCephKeyring() = %v, want %v", actual, expected)
	}
}

func TestParseNoCapsKeyring(t *testing.T) {
	text := `[client.foo]
	key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==`

	expected := []CephUser{
		{
			Entity: "client.foo",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   CephCaps{},
		},
	}

	actual, err := parseCephKeyring(text)
	if err != nil {
		t.Errorf("parseCephKeyring() error = %v, wantErr nil", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("parseCephKeyring() = %v, want %v", actual, expected)
	}
}

func TestInvalidKeyring(t *testing.T) {
	text := `hello`

	_, err := parseCephKeyring(text)
	if err == nil {
		t.Errorf("parseCephKeyring() error = nil, wantErr non-nil")
		return
	}

	expectedError := "parse error:1:hello"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("parseCephKeyring() error = %q, want error containing %q", err.Error(), expectedError)
	}
}

func TestIgnoreUnknownProperties(t *testing.T) {
	text := `[client.foo]
	key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
	foo = bar
`

	expected := []CephUser{
		{
			Entity: "client.foo",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   CephCaps{},
		},
	}

	actual, err := parseCephKeyring(text)
	if err != nil {
		t.Errorf("parseCephKeyring() error = %v, wantErr nil", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("parseCephKeyring() = %v, want %v", actual, expected)
	}
}

func TestFormatClientAdminKeyring(t *testing.T) {
	users := []CephUser{
		{
			Entity: "client.admin",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCephCapsFromMap(map[string]string{"mds": "allow *", "mgr": "allow *", "mon": "allow *", "osd": "allow *"}),
		},
	}

	expected := `[client.admin]
	key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
	caps mds = "allow *"
	caps mgr = "allow *"
	caps mon = "allow *"
	caps osd = "allow *"
`

	actual := formatCephKeyring(users)

	if actual != expected {
		t.Errorf("formatCephKeyring() = %q, want %q", actual, expected)
	}
}

func TestFormatClientFooKeyring(t *testing.T) {
	users := []CephUser{
		{
			Entity: "client.foo",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCephCapsFromMap(map[string]string{"mds": "allow rw path=/", "mon": "allow rw", "osd": "allow rwx"}),
		},
	}

	expected := `[client.foo]
	key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
	caps mds = "allow rw path=/"
	caps mon = "allow rw"
	caps osd = "allow rwx"
`

	actual := formatCephKeyring(users)

	if actual != expected {
		t.Errorf("formatCephKeyring() = %q, want %q", actual, expected)
	}
}

func TestFormatNoCapsKeyring(t *testing.T) {
	users := []CephUser{
		{
			Entity: "client.foo",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   CephCaps{},
		},
	}

	expected := `[client.foo]
	key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
`

	actual := formatCephKeyring(users)

	if actual != expected {
		t.Errorf("formatCephKeyring() = %q, want %q", actual, expected)
	}
}

func TestFormatMultipleUsers(t *testing.T) {
	users := []CephUser{
		{
			Entity: "osd.0",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCephCapsFromMap(map[string]string{"mgr": "allow profile osd", "mon": "allow profile osd", "osd": "allow *"}),
		},
		{
			Entity: "osd.1",
			Key:    "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB==",
			Caps:   MustCephCapsFromMap(map[string]string{"mgr": "allow profile osd", "mon": "allow profile osd", "osd": "allow *"}),
		},
	}

	expected := `[osd.0]
	key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
	caps mgr = "allow profile osd"
	caps mon = "allow profile osd"
	caps osd = "allow *"

[osd.1]
	key = BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB==
	caps mgr = "allow profile osd"
	caps mon = "allow profile osd"
	caps osd = "allow *"
`

	actual := formatCephKeyring(users)

	if actual != expected {
		t.Errorf("formatCephKeyring() = %q, want %q", actual, expected)
	}
}

func TestFormatParseRoundTrip(t *testing.T) {
	original := []CephUser{
		{
			Entity: "client.admin",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCephCapsFromMap(map[string]string{"mds": "allow *", "mgr": "allow *", "mon": "allow *", "osd": "allow *"}),
		},
	}

	serialized := formatCephKeyring(original)

	parsed, err := parseCephKeyring(serialized)
	if err != nil {
		t.Errorf("parseCephKeyring() error = %v, wantErr nil", err)
	}

	if !reflect.DeepEqual(parsed, original) {
		t.Errorf("Round-trip failed: got %v, want %v", parsed, original)
	}

	reserialized := formatCephKeyring(parsed)
	if reserialized != serialized {
		t.Errorf("Re-serialization changed output:\nFirst:  %q\nSecond: %q", serialized, reserialized)
	}
}
