package typeless

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/pbkdf2"
)

const appName = "Typeless"

type User struct {
	Email        string `json:"email"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	LoginTime    int64  `json:"login_time"`
	UserID       string `json:"user_id"`
	ClientUserID string `json:"client_user_id"`
}

type electronUserStore struct {
	UserData string `json:"userData"`
}

func DefaultUserDataPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "Typeless", "user-data.json"), nil
}

func DefaultHistoryDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "Typeless", "typeless.db"), nil
}

func LoadCurrentUser(_ context.Context, userDataPath string) (User, error) {
	if userDataPath == "" {
		defaultPath, err := DefaultUserDataPath()
		if err != nil {
			return User{}, err
		}
		userDataPath = defaultPath
	}

	encrypted, err := os.ReadFile(userDataPath)
	if err != nil {
		return User{}, fmt.Errorf("读取 Typeless 用户数据失败: %w", err)
	}

	plain, err := decryptElectronStore(encrypted, typelessUserStoreKey())
	if err != nil {
		return User{}, fmt.Errorf("解密 Typeless 用户数据失败: %w", err)
	}

	var store electronUserStore
	if err := json.Unmarshal(plain, &store); err != nil {
		return User{}, fmt.Errorf("解析 Typeless 用户数据失败: %w", err)
	}
	if strings.TrimSpace(store.UserData) == "" {
		return User{}, fmt.Errorf("Typeless 用户数据为空，请先登录 Typeless")
	}

	var user User
	if err := json.Unmarshal([]byte(store.UserData), &user); err != nil {
		return User{}, fmt.Errorf("解析 Typeless 登录态失败: %w", err)
	}
	if user.RefreshToken == "" {
		return User{}, fmt.Errorf("Typeless refresh_token 为空，请重新登录 Typeless")
	}
	return user, nil
}

func typelessUserStoreKey() []byte {
	platformArch := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	digest := sha256.Sum256([]byte(platformArch))
	password := fmt.Sprintf("%x%s", digest, appName)
	return pbkdf2.Key([]byte(password), []byte("typeless-user-service"), 10000, 32, sha256.New)
}

func decryptElectronStore(data []byte, encryptionKey []byte) ([]byte, error) {
	if len(data) < aes.BlockSize+1 {
		return nil, fmt.Errorf("加密数据长度不合法")
	}

	iv := data[:aes.BlockSize]
	cipherText := data[aes.BlockSize+1:]
	// electron-store/conf derives the AES key with initializationVector.toString().
	// Node replaces invalid UTF-8 bytes during Buffer.toString(), while Go keeps
	// them verbatim when converting []byte to string, so we mirror Node here.
	ivSalt := nodeBufferToUTF8String(iv)
	password := pbkdf2.Key(encryptionKey, []byte(ivSalt), 10000, 32, sha512.New)

	block, err := aes.NewCipher(password)
	if err != nil {
		return nil, err
	}
	if len(cipherText)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("密文长度不是 AES block size 的整数倍")
	}

	plain := make([]byte, len(cipherText))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, cipherText)
	return pkcs7Unpad(plain, aes.BlockSize)
}

func nodeBufferToUTF8String(data []byte) string {
	var builder strings.Builder
	builder.Grow(len(data))
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 && data[0] >= utf8.RuneSelf {
			builder.WriteRune(utf8.RuneError)
			data = data[1:]
			continue
		}
		builder.WriteRune(r)
		data = data[size:]
	}
	return builder.String()
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("PKCS7 数据长度不合法")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, fmt.Errorf("PKCS7 padding 不合法")
	}
	if !bytes.Equal(data[len(data)-padding:], bytes.Repeat([]byte{byte(padding)}, padding)) {
		return nil, fmt.Errorf("PKCS7 padding 内容不合法")
	}
	return data[:len(data)-padding], nil
}
