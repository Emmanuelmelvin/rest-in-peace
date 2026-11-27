package rt

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"strconv"
	"strings"
)

const (
	maxRandomHexBytes = 1 << 20
	pbkdf2Iterations  = 100000
	pbkdf2SaltBytes   = 16
	pbkdf2KeyBytes    = 32
	pbkdf2MaxIter     = 10_000_000
)

// UUID returns a random RFC 4122 version 4 UUID string.
func UUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(Errorf(KindInternal, "UUID failed to read random bytes: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// RandomHex returns n cryptographically random bytes encoded as lowercase hex.
// n must be a whole number in [0, 1<<20].
func RandomHex(n Value) string {
	k, ok := toInt(n)
	if !ok {
		panic(Errorf(KindTypeMismatch, "RANDOMHEX needs a whole NUMBER, got %s", TypeName(n)))
	}
	if k < 0 || k > maxRandomHexBytes {
		panic(Errorf(KindTypeMismatch, "RANDOMHEX length must be between 0 and %d, got %d", maxRandomHexBytes, k))
	}
	if k == 0 {
		return ""
	}
	b := make([]byte, k)
	if _, err := rand.Read(b); err != nil {
		panic(Errorf(KindInternal, "RANDOMHEX failed to read random bytes: %v", err))
	}
	return hex.EncodeToString(b)
}

// Hash computes a cryptographic digest of data using algo ("sha256", "sha512",
// "sha1", or "md5", case-insensitive) and returns lowercase hex.
func Hash(algo string, data string) string {
	switch strings.ToLower(algo) {
	case "sha256":
		sum := sha256.Sum256([]byte(data))
		return hex.EncodeToString(sum[:])
	case "sha512":
		sum := sha512.Sum512([]byte(data))
		return hex.EncodeToString(sum[:])
	case "sha1":
		sum := sha1.Sum([]byte(data))
		return hex.EncodeToString(sum[:])
	case "md5":
		sum := md5.Sum([]byte(data))
		return hex.EncodeToString(sum[:])
	default:
		panic(Errorf(KindTypeMismatch, "HASH unknown algorithm %q — use sha256, sha512, sha1, or md5", algo))
	}
}

// HMAC computes an HMAC digest of data with key using algo ("sha256" or
// "sha512", case-insensitive) and returns lowercase hex.
func HMAC(algo string, key string, data string) string {
	var h func() hash.Hash
	switch strings.ToLower(algo) {
	case "sha256":
		h = sha256.New
	case "sha512":
		h = sha512.New
	default:
		panic(Errorf(KindTypeMismatch, "HMAC unknown algorithm %q — use sha256 or sha512", algo))
	}
	mac := hmac.New(h, []byte(key))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// HashPassword hashes pw with PBKDF2-SHA256 (100,000 iterations, 16-byte random
// salt, 32-byte derived key) and returns "$pbkdf2-sha256$100000$<saltHex>$<hashHex>".
func HashPassword(pw string) string {
	salt := make([]byte, pbkdf2SaltBytes)
	if _, err := rand.Read(salt); err != nil {
		panic(Errorf(KindInternal, "HASHPASSWORD failed to read random bytes: %v", err))
	}
	dk, err := pbkdf2.Key(sha256.New, pw, salt, pbkdf2Iterations, pbkdf2KeyBytes)
	if err != nil {
		panic(Errorf(KindInternal, "HASHPASSWORD failed: %v", err))
	}
	return fmt.Sprintf("$pbkdf2-sha256$%d$%s$%s",
		pbkdf2Iterations, hex.EncodeToString(salt), hex.EncodeToString(dk))
}

// VerifyPassword checks pw against an encoded string produced by HashPassword
// using constant-time comparison. Malformed encoded strings return false.
func VerifyPassword(pw string, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "" || parts[1] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[2])
	if err != nil || iter <= 0 || iter > pbkdf2MaxIter {
		return false
	}
	salt, err := hex.DecodeString(parts[3])
	if err != nil || len(salt) == 0 {
		return false
	}
	expected, err := hex.DecodeString(parts[4])
	if err != nil || len(expected) == 0 {
		return false
	}
	dk, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(expected))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(dk, expected) == 1
}

// Base64Encode encodes s using standard base64 encoding.
func Base64Encode(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// Base64Decode decodes a standard base64 string, panicking with KindTypeMismatch
// if the input is not valid base64.
func Base64Decode(s string) string {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(Errorf(KindTypeMismatch, "BASE64DECODE invalid base64 input: %v", err))
	}
	return string(b)
}
