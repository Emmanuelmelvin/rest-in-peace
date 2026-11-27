package rt

import (
	"encoding/hex"
	"regexp"
	"strings"
	"testing"
)

func TestUUID(t *testing.T) {
	re := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := make(map[string]bool, 64)
	for i := 0; i < 64; i++ {
		u := UUID()
		if !re.MatchString(u) {
			t.Fatalf("UUID() = %q, does not match RFC 4122 v4 pattern", u)
		}
		if seen[u] {
			t.Fatalf("UUID() produced duplicate %q", u)
		}
		seen[u] = true
	}
}

func TestCryptoRandomHex(t *testing.T) {
	tests := []struct {
		name    string
		n       Value
		wantLen int
	}{
		{"zero", int64(0), 0},
		{"16 bytes", int64(16), 32},
		{"whole float", float64(8), 16},
		{"32 bytes", int64(32), 64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RandomHex(tt.n)
			if len(got) != tt.wantLen {
				t.Fatalf("RandomHex(%v) len = %d, want %d", tt.n, len(got), tt.wantLen)
			}
			if _, err := hex.DecodeString(got); err != nil {
				t.Fatalf("RandomHex(%v) = %q is not valid hex: %v", tt.n, got, err)
			}
			if got != strings.ToLower(got) {
				t.Fatalf("RandomHex(%v) = %q is not lowercase", tt.n, got)
			}
		})
	}

	if a, b := RandomHex(int64(16)), RandomHex(int64(16)); a == b {
		t.Fatalf("two RandomHex(16) calls returned identical %q", a)
	}

	errCases := map[string]Value{
		"negative":   int64(-1),
		"too large":  int64((1 << 20) + 1),
		"non-int":    1.5,
		"string arg": "16",
		"nil":        nil,
	}
	for name, arg := range errCases {
		if k := panicKind(func() { RandomHex(arg) }); k != KindTypeMismatch {
			t.Errorf("RandomHex(%s): got panic %q, want %q", name, k, KindTypeMismatch)
		}
	}
}

func TestHash(t *testing.T) {
	tests := []struct {
		algo string
		data string
		want string
	}{
		{"sha256", "hello", "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"},
		{"SHA256", "hello", "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"},
		{"sha512", "hello", "9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043"},
		{"SHA512", "", "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e"},
		{"sha1", "hello", "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"},
		{"md5", "hello", "5d41402abc4b2a76b9719d911017c592"},
		{"MD5", "", "d41d8cd98f00b204e9800998ecf8427e"},
	}
	for _, tt := range tests {
		got := Hash(tt.algo, tt.data)
		if got != tt.want {
			t.Errorf("Hash(%q, %q) = %q, want %q", tt.algo, tt.data, got, tt.want)
		}
	}

	if k := panicKind(func() { Hash("blake2b", "hello") }); k != KindTypeMismatch {
		t.Errorf("Hash unknown algo: got %q, want %q", k, KindTypeMismatch)
	}
}

func TestCryptoHMAC(t *testing.T) {
	tests := []struct {
		algo string
		key  string
		data string
		want string
	}{
		{
			"sha256",
			"key",
			"The quick brown fox jumps over the lazy dog",
			"f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8",
		},
		{
			"SHA256",
			"key",
			"The quick brown fox jumps over the lazy dog",
			"f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8",
		},
		{
			"sha512",
			"key",
			"The quick brown fox jumps over the lazy dog",
			"b42af09057bac1e2d41708e48a902e09b5ff7f12ab428a4fe86653c73dd248fb82f948a549f7b791a5b41915ee4d1ec3935357e4e2317250d0372afa2ebeeb3a",
		},
	}
	for _, tt := range tests {
		got := HMAC(tt.algo, tt.key, tt.data)
		if got != tt.want {
			t.Errorf("HMAC(%q, %q, %q) = %q, want %q", tt.algo, tt.key, tt.data, got, tt.want)
		}
	}

	if k := panicKind(func() { HMAC("md5", "key", "data") }); k != KindTypeMismatch {
		t.Errorf("HMAC unsupported algo: got %q, want %q", k, KindTypeMismatch)
	}
}

func TestPasswordHashAndVerify(t *testing.T) {
	encoded := HashPassword("s3cr3t!pass")
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "" || parts[1] != "pbkdf2-sha256" || parts[2] != "100000" {
		t.Fatalf("HashPassword format unexpected: %q", encoded)
	}
	if len(parts[3]) != 32 || len(parts[4]) != 64 {
		t.Fatalf("HashPassword salt/hash hex lengths = %d/%d, want 32/64", len(parts[3]), len(parts[4]))
	}

	// Same password produces different hashes due to random salt.
	encoded2 := HashPassword("s3cr3t!pass")
	if encoded == encoded2 {
		t.Fatalf("HashPassword produced identical hashes for two calls")
	}

	if !VerifyPassword("s3cr3t!pass", encoded) {
		t.Errorf("VerifyPassword returned false for correct password")
	}
	if VerifyPassword("wrong", encoded) {
		t.Errorf("VerifyPassword returned true for wrong password")
	}

	// Malformed inputs must return false and never panic.
	malformed := []string{
		"",
		"not-a-hash",
		"$pbkdf2-sha256$100000$abcd",
		"$pbkdf2-sha512$100000$0011$2233",
		"$pbkdf2-sha256$abc$0011$2233",
		"$pbkdf2-sha256$0$0011$2233",
		"$pbkdf2-sha256$-5$0011$2233",
		"$pbkdf2-sha256$99999999$0011$2233",
		"$pbkdf2-sha256$100$zzzz$2233",
		"$pbkdf2-sha256$100$0011$zzzz",
		"$pbkdf2-sha256$100$$2233",
		"$pbkdf2-sha256$100$0011$",
		"x$pbkdf2-sha256$100$0011$2233",
	}
	for _, bad := range malformed {
		if VerifyPassword("s3cr3t!pass", bad) {
			t.Errorf("VerifyPassword(_, %q) = true, want false", bad)
		}
	}
}

func TestBase64(t *testing.T) {
	tests := []struct {
		plain   string
		encoded string
	}{
		{"", ""},
		{"hello", "aGVsbG8="},
		{"hello, world!", "aGVsbG8sIHdvcmxkIQ=="},
		{"héllo 🌍", "aMOpbGxvIPCfjI0="},
	}
	for _, tt := range tests {
		if got := Base64Encode(tt.plain); got != tt.encoded {
			t.Errorf("Base64Encode(%q) = %q, want %q", tt.plain, got, tt.encoded)
		}
		if got := Base64Decode(tt.encoded); got != tt.plain {
			t.Errorf("Base64Decode(%q) = %q, want %q", tt.encoded, got, tt.plain)
		}
	}

	if k := panicKind(func() { Base64Decode("!!!not-base64!!!") }); k != KindTypeMismatch {
		t.Errorf("Base64Decode invalid: got %q, want %q", k, KindTypeMismatch)
	}
}
