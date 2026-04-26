package typeless

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha512"
	"testing"

	"golang.org/x/crypto/pbkdf2"
)

func TestDecryptElectronStore(t *testing.T) {
	t.Parallel()

	encryptionKey := []byte{
		0x00, 0x7f, 0x80, 0x81, 0xfe, 0xff, 0x41, 0x42,
		0x43, 0x10, 0x11, 0x12, 0x90, 0x91, 0x92, 0x93,
		0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04,
		0x55, 0xaa, 0xf0, 0x0f, 0x20, 0x21, 0x22, 0x23,
	}
	plain := []byte(`{"userData":"{\"refresh_token\":\"token\"}"}`)
	iv := []byte{
		0xdd, 0x73, 0xd3, 0x23, 0x4d, 0x6c, 0xa8, 0x2b,
		0x0f, 0x98, 0x27, 0xd6, 0x28, 0x88, 0xfc, 0x59,
	}

	encrypted := encryptElectronStoreFixture(t, plain, encryptionKey, iv)
	decrypted, err := decryptElectronStore(encrypted, encryptionKey)
	if err != nil {
		t.Fatalf("decryptElectronStore() error = %v", err)
	}
	if !bytes.Equal(decrypted, plain) {
		t.Fatalf("decryptElectronStore() mismatch\nwant: %q\ngot:  %q", plain, decrypted)
	}
}

func encryptElectronStoreFixture(t *testing.T, plain, encryptionKey, iv []byte) []byte {
	t.Helper()

	ivSalt := nodeBufferToUTF8String(iv)
	password := pbkdf2.Key(encryptionKey, []byte(ivSalt), 10000, 32, sha512.New)

	block, err := aes.NewCipher(password)
	if err != nil {
		t.Fatalf("aes.NewCipher() error = %v", err)
	}

	padded := pkcs7PadFixture(plain, aes.BlockSize)
	cipherText := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(cipherText, padded)

	encrypted := make([]byte, 0, len(iv)+1+len(cipherText))
	encrypted = append(encrypted, iv...)
	encrypted = append(encrypted, ':')
	encrypted = append(encrypted, cipherText...)
	return encrypted
}

func TestNodeBufferToUTF8String(t *testing.T) {
	t.Parallel()

	data := []byte{
		0xdd, 0x73, 0xd3, 0x23, 0x4d, 0x6c, 0xa8, 0x2b,
		0x0f, 0x98, 0x27, 0xd6, 0x28, 0x88, 0xfc, 0x59,
	}
	got := []byte(nodeBufferToUTF8String(data))
	want := []byte{
		0xef, 0xbf, 0xbd, 0x73, 0xef, 0xbf, 0xbd, 0x23,
		0x4d, 0x6c, 0xef, 0xbf, 0xbd, 0x2b, 0x0f, 0xef,
		0xbf, 0xbd, 0x27, 0xef, 0xbf, 0xbd, 0x28, 0xef,
		0xbf, 0xbd, 0xef, 0xbf, 0xbd, 0x59,
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("nodeBufferToUTF8String() mismatch\nwant: %x\ngot:  %x", want, got)
	}
}

func pkcs7PadFixture(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	return append(bytes.Clone(data), bytes.Repeat([]byte{byte(padding)}, padding)...)
}
