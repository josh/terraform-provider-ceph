package keyring

import (
	"reflect"
	"strings"
	"testing"
)

func TestKeyTypeKnownAESKey(t *testing.T) {
	got, err := KeyType("AQB5m89objcKIxAAda2ULz/l3NH+mv9XzKePHQ==")
	if err != nil {
		t.Fatalf("KeyType() error = %v", err)
	}
	if got != KeyTypeAES {
		t.Errorf("KeyType() = %q, want %q", got, KeyTypeAES)
	}
}

func TestGenerateKeyRoundTrip(t *testing.T) {
	cases := []struct {
		keyType string
		length  int
		prefix  string
	}{
		{KeyTypeAES, 40, "AQ"},
		{KeyTypeAES256K, 60, "Ag"},
	}
	for _, tc := range cases {
		key, err := GenerateKey(tc.keyType)
		if err != nil {
			t.Fatalf("GenerateKey(%q) error = %v", tc.keyType, err)
		}
		if len(key) != tc.length || !strings.HasPrefix(key, tc.prefix) {
			t.Errorf("GenerateKey(%q) = %q, want %d chars starting %q", tc.keyType, key, tc.length, tc.prefix)
		}
		got, err := KeyType(key)
		if err != nil {
			t.Fatalf("KeyType(%q) error = %v", key, err)
		}
		if got != tc.keyType {
			t.Errorf("KeyType() = %q, want %q", got, tc.keyType)
		}
		other, _ := GenerateKey(tc.keyType)
		if other == key {
			t.Errorf("GenerateKey(%q) returned the same key twice", tc.keyType)
		}
	}
}

func TestGenerateKeyUnsupportedType(t *testing.T) {
	if _, err := GenerateKey("rc4"); err == nil {
		t.Error("GenerateKey(\"rc4\") error = nil, want error")
	}
}

func TestKeyTypeMalformed(t *testing.T) {
	for _, key := range []string{
		"",
		"not base64!",
		"AAAA",
		"AwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
	} {
		if _, err := KeyType(key); err == nil {
			t.Errorf("KeyType(%q) error = nil, want error", key)
		}
	}
}

func TestEmptyKeyring(t *testing.T) {
	_, err := Parse("")
	if err == nil {
		t.Errorf("Parse() error = nil, wantErr non-nil")
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

	expected := []User{
		{
			Entity: "client.admin",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCapsFromMap(map[string]string{"mds": "allow *", "mgr": "allow *", "mon": "allow *", "osd": "allow *"}),
		},
	}

	actual, err := Parse(text)
	if err != nil {
		t.Errorf("Parse() error = %v, wantErr nil", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Parse() = %v, want %v", actual, expected)
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

	expected := []User{
		{
			Entity: "osd.0",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCapsFromMap(map[string]string{"mgr": "allow profile osd", "mon": "allow profile osd", "osd": "allow *"}),
		},
		{
			Entity: "osd.1",
			Key:    "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB==",
			Caps:   MustCapsFromMap(map[string]string{"mgr": "allow profile osd", "mon": "allow profile osd", "osd": "allow *"}),
		},
	}

	actual, err := Parse(text)
	if err != nil {
		t.Errorf("Parse() error = %v, wantErr nil", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Parse() = %v, want %v", actual, expected)
	}
}

func TestParseClientFooKeyring(t *testing.T) {
	text := `[client.foo]
	key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
	caps mds = "allow rw path=/"
	caps mon = "allow rw"
	caps osd = "allow rwx"
`

	expected := []User{
		{
			Entity: "client.foo",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCapsFromMap(map[string]string{"mds": "allow rw path=/", "mon": "allow rw", "osd": "allow rwx"}),
		},
	}

	actual, err := Parse(text)
	if err != nil {
		t.Errorf("Parse() error = %v, wantErr nil", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Parse() = %v, want %v", actual, expected)
	}
}

func TestParseQuotedCommandCapsKeyring(t *testing.T) {
	// ceph auth export escapes embedded quotes (KeyRing::print replaces
	// `"` with `\"`).
	text := `[client.foo]
	key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
	caps mon = "allow command \"osd blacklist\""
`

	expected := []User{
		{
			Entity: "client.foo",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCapsFromMap(map[string]string{"mon": `allow command "osd blacklist"`}),
		},
	}

	actual, err := Parse(text)
	if err != nil {
		t.Errorf("Parse() error = %v, wantErr nil", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Parse() = %v, want %v", actual, expected)
	}
}

func TestParseNoCapsKeyring(t *testing.T) {
	text := `[client.foo]
	key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==`

	expected := []User{
		{
			Entity: "client.foo",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   Caps{},
		},
	}

	actual, err := Parse(text)
	if err != nil {
		t.Errorf("Parse() error = %v, wantErr nil", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Parse() = %v, want %v", actual, expected)
	}
}

func TestInvalidKeyring(t *testing.T) {
	text := `hello`

	_, err := Parse(text)
	if err == nil {
		t.Errorf("Parse() error = nil, wantErr non-nil")
		return
	}

	expectedError := "parse error:1:hello"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("Parse() error = %q, want error containing %q", err.Error(), expectedError)
	}
}

func TestIgnoreUnknownProperties(t *testing.T) {
	text := `[client.foo]
	key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
	foo = bar
`

	expected := []User{
		{
			Entity: "client.foo",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   Caps{},
		},
	}

	actual, err := Parse(text)
	if err != nil {
		t.Errorf("Parse() error = %v, wantErr nil", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Parse() = %v, want %v", actual, expected)
	}
}

func TestFormatClientAdminKeyring(t *testing.T) {
	users := []User{
		{
			Entity: "client.admin",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCapsFromMap(map[string]string{"mds": "allow *", "mgr": "allow *", "mon": "allow *", "osd": "allow *"}),
		},
	}

	expected := `[client.admin]
	key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
	caps mds = "allow *"
	caps mgr = "allow *"
	caps mon = "allow *"
	caps osd = "allow *"
`

	actual := Format(users)

	if actual != expected {
		t.Errorf("Format() = %q, want %q", actual, expected)
	}
}

func TestFormatClientFooKeyring(t *testing.T) {
	users := []User{
		{
			Entity: "client.foo",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCapsFromMap(map[string]string{"mds": "allow rw path=/", "mon": "allow rw", "osd": "allow rwx"}),
		},
	}

	expected := `[client.foo]
	key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
	caps mds = "allow rw path=/"
	caps mon = "allow rw"
	caps osd = "allow rwx"
`

	actual := Format(users)

	if actual != expected {
		t.Errorf("Format() = %q, want %q", actual, expected)
	}
}

func TestFormatNoCapsKeyring(t *testing.T) {
	users := []User{
		{
			Entity: "client.foo",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   Caps{},
		},
	}

	expected := `[client.foo]
	key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
`

	actual := Format(users)

	if actual != expected {
		t.Errorf("Format() = %q, want %q", actual, expected)
	}
}

func TestFormatMultipleUsers(t *testing.T) {
	users := []User{
		{
			Entity: "osd.0",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCapsFromMap(map[string]string{"mgr": "allow profile osd", "mon": "allow profile osd", "osd": "allow *"}),
		},
		{
			Entity: "osd.1",
			Key:    "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB==",
			Caps:   MustCapsFromMap(map[string]string{"mgr": "allow profile osd", "mon": "allow profile osd", "osd": "allow *"}),
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

	actual := Format(users)

	if actual != expected {
		t.Errorf("Format() = %q, want %q", actual, expected)
	}
}

func TestFormatParseRoundTrip(t *testing.T) {
	original := []User{
		{
			Entity: "client.admin",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCapsFromMap(map[string]string{"mds": "allow *", "mgr": "allow *", "mon": "allow *", "osd": "allow *"}),
		},
	}

	serialized := Format(original)

	parsed, err := Parse(serialized)
	if err != nil {
		t.Errorf("Parse() error = %v, wantErr nil", err)
	}

	if !reflect.DeepEqual(parsed, original) {
		t.Errorf("Round-trip failed: got %v, want %v", parsed, original)
	}

	reserialized := Format(parsed)
	if reserialized != serialized {
		t.Errorf("Re-serialization changed output:\nFirst:  %q\nSecond: %q", serialized, reserialized)
	}
}

func TestFormatQuotedCommandCaps(t *testing.T) {
	users := []User{
		{
			Entity: "client.foo",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCapsFromMap(map[string]string{"mon": `allow command "osd blacklist"`}),
		},
	}

	expected := `[client.foo]
	key = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==
	caps mon = "allow command \"osd blacklist\""
`

	if actual := Format(users); actual != expected {
		t.Errorf("Format() = %q, want %q", actual, expected)
	}
}

func TestFormatParseRoundTripQuotedCaps(t *testing.T) {
	original := []User{
		{
			Entity: "client.foo",
			Key:    "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			Caps:   MustCapsFromMap(map[string]string{"mon": `allow command "osd blacklist"`}),
		},
	}

	parsed, err := Parse(Format(original))
	if err != nil {
		t.Fatalf("Parse() error = %v, wantErr nil", err)
	}

	if !reflect.DeepEqual(parsed, original) {
		t.Errorf("Round-trip failed: got %v, want %v", parsed, original)
	}
}
