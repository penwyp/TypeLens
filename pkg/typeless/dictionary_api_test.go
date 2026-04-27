package typeless

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"
)

func TestEncryptOpenSSLWithSaltMatchesCompatibilityVector(t *testing.T) {
	t.Parallel()

	salt, err := hex.DecodeString("0102030405060708")
	if err != nil {
		t.Fatal(err)
	}
	got, err := encryptOpenSSLWithSalt([]byte("hello"), []byte("secret"), salt)
	if err != nil {
		t.Fatalf("encryptOpenSSLWithSalt() error = %v", err)
	}

	want, err := hex.DecodeString("53616c7465645f5f0102030405060708de2f4d01f88c62e9f966a4d55accb78f")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("encryptOpenSSLWithSalt() = %x, want %x", got, want)
	}
}

func TestEVPBytesToKeyMatchesCompatibilityVector(t *testing.T) {
	t.Parallel()

	salt, err := hex.DecodeString("0102030405060708")
	if err != nil {
		t.Fatal(err)
	}
	key, iv := evpBytesToKey([]byte("secret"), salt, 32, 16)

	wantKey := "c9e5a1bd216dbe1317e230cef48f38ee7f0e17ad64022144bccec4a1aa2879ab"
	if got := hex.EncodeToString(key); got != wantKey {
		t.Fatalf("key = %s, want %s", got, wantKey)
	}
	wantIV := "e24b32bbbc4ef02ecbcb6576523ad893"
	if got := hex.EncodeToString(iv); got != wantIV {
		t.Fatalf("iv = %s, want %s", got, wantIV)
	}
}

func TestBuildDictionaryHeaders(t *testing.T) {
	t.Parallel()

	headers, err := buildDictionaryHeaders(User{
		RefreshToken: "refresh-token",
		UserID:       "user-1",
	}, "/user/dictionary/list", time.UnixMilli(1770000000123))
	if err != nil {
		t.Fatalf("buildDictionaryHeaders() error = %v", err)
	}

	if got, want := headers.Get("Authorization"), "Bearer refresh-token"; got != want {
		t.Fatalf("Authorization = %q, want %q", got, want)
	}
	if got := headers.Get("X-App-Version"); got != typelessAppVersion {
		t.Fatalf("X-App-Version = %q, want %q", got, typelessAppVersion)
	}
	if got, want := headers.Get("User-Agent"), "node"; got != want {
		t.Fatalf("User-Agent = %q, want %q", got, want)
	}
	decoded, err := base64.StdEncoding.DecodeString(headers.Get("X-Authorization"))
	if err != nil {
		t.Fatalf("X-Authorization is not base64: %v", err)
	}
	if !bytes.HasPrefix(decoded, []byte("Salted__")) {
		t.Fatalf("X-Authorization payload prefix = %q, want Salted__", decoded[:min(len(decoded), len("Salted__"))])
	}
}
