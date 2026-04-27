package typeless

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/emmansun/gmsm/sm3"
)

const (
	typelessAppVersion       = "mac_1.2.1"
	typelessSecuritySecret   = "5f69d2e7b648a41e027807ad5dd1d679f5df194ea43c2d47aea317b9"
	typelessAuthorizationKey = "a8ceffb90069eac13d3ecb057da340054e5936bae788cd56bd1a4e72"
	typelessHubFileURL       = "file:///Applications/Typeless.app/Contents/Resources/app.asar/dist/renderer/hub.html"
)

type dictionaryAuthorizationPayload struct {
	Env          string                       `json:"X-Env"`
	ClientDomain string                       `json:"X-Client-Domain"`
	ClientPath   string                       `json:"X-Client-Path"`
	Random       string                       `json:"X-Random"`
	Timestamp    int64                        `json:"t"`
	Proof        string                       `json:"p"`
	Device       string                       `json:"d"`
	Extra        dictionaryAuthorizationExtra `json:"3c86e26ccbb7274f752e7d868a1541ebfb7f37e7"`
}

type dictionaryAuthorizationExtra struct {
	A string `json:"a"`
}

func (c *DictionaryClient) runDictionaryRequest(
	ctx context.Context,
	method string,
	pathname string,
	query url.Values,
	body any,
	out any,
) error {
	user, err := LoadCurrentUser(ctx, c.userDataPath)
	if err != nil {
		return err
	}

	endpoint, err := url.Parse(c.apiHost + pathname)
	if err != nil {
		return err
	}
	if len(query) > 0 {
		endpoint.RawQuery = query.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(payload)
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bodyReader)
	if err != nil {
		return err
	}
	headers, err := buildDictionaryHeaders(user, pathname, time.Now())
	if err != nil {
		return err
	}
	request.Header = headers

	client := &http.Client{Timeout: c.timeout}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("调用 Typeless API 失败: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("读取 Typeless API 返回失败: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("调用 Typeless API 失败: HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("解析 Typeless API 返回失败: %w; body=%s", err, strings.TrimSpace(string(responseBody)))
	}
	return nil
}

func buildDictionaryHeaders(user User, pathname string, now time.Time) (http.Header, error) {
	timestamp := now.UnixMilli()
	timestampText := strconv.FormatInt(timestamp, 10)
	sha1SecretKey := timestampText + ":" + typelessSecuritySecret
	signText := timestampText + ":" + typelessAppVersion + ":" + pathname + ":" + user.UserID

	mac := hmac.New(sha1.New, []byte(sha1SecretKey))
	_, _ = mac.Write([]byte(signText))
	sha1Hash := hex.EncodeToString(mac.Sum(nil))

	sm3Hash := sm3.New()
	_, _ = sm3Hash.Write([]byte(timestampText + ":" + sha1Hash + ":" + typelessSecuritySecret))
	proof := hex.EncodeToString(sm3Hash.Sum(nil))

	randomText, err := randomDigits(6)
	if err != nil {
		return nil, err
	}
	authorizationPayload, err := json.Marshal(dictionaryAuthorizationPayload{
		Env:          "prod",
		ClientDomain: typelessHubFileURL,
		ClientPath:   typelessHubFileURL,
		Random:       randomText,
		Timestamp:    timestamp,
		Proof:        proof,
		Device:       "UNKNOWN",
		Extra:        dictionaryAuthorizationExtra{},
	})
	if err != nil {
		return nil, err
	}
	xAuthorization, err := encryptOpenSSL(authorizationPayload, []byte(typelessAuthorizationKey))
	if err != nil {
		return nil, err
	}

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+user.RefreshToken)
	headers.Set("Accept", "application/json")
	headers.Set("Content-Type", "application/json")
	headers.Set("User-Agent", "node")
	headers.Set("X-Browser-Name", "UNKNOWN")
	headers.Set("X-Browser-Version", "UNKNOWN")
	headers.Set("X-Browser-Major", "UNKNOWN")
	headers.Set("X-App-Version", typelessAppVersion)
	headers.Set("X-Authorization", xAuthorization)
	return headers, nil
}

func encryptOpenSSL(plaintext []byte, passphrase []byte) (string, error) {
	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	encrypted, err := encryptOpenSSLWithSalt(plaintext, passphrase, salt)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

func encryptOpenSSLWithSalt(plaintext []byte, passphrase []byte, salt []byte) ([]byte, error) {
	if len(salt) != 8 {
		return nil, fmt.Errorf("OpenSSL salt 长度必须为 8 字节")
	}
	key, iv := evpBytesToKey(passphrase, salt, 32, aes.BlockSize)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	cipherText := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(cipherText, padded)

	output := make([]byte, 0, len("Salted__")+len(salt)+len(cipherText))
	output = append(output, "Salted__"...)
	output = append(output, salt...)
	output = append(output, cipherText...)
	return output, nil
}

func evpBytesToKey(passphrase []byte, salt []byte, keyLen int, ivLen int) ([]byte, []byte) {
	output := make([]byte, 0, keyLen+ivLen)
	previous := []byte{}
	for len(output) < keyLen+ivLen {
		hash := md5.New()
		_, _ = hash.Write(previous)
		_, _ = hash.Write(passphrase)
		_, _ = hash.Write(salt)
		previous = hash.Sum(nil)
		output = append(output, previous...)
	}
	return output[:keyLen], output[keyLen : keyLen+ivLen]
}

func randomDigits(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("随机数字长度必须大于 0")
	}
	min := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(length-1)), nil)
	max := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(length)), nil)
	diff := new(big.Int).Sub(max, min)
	value, err := rand.Int(rand.Reader, diff)
	if err != nil {
		return "", err
	}
	value.Add(value, min)
	return value.String(), nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	return append(bytes.Clone(data), bytes.Repeat([]byte{byte(padding)}, padding)...)
}
