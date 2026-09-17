package auxcloud

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"strconv"
)

const (
	timestampTokenEncryptKey = "kdixkdqp54545^#*"
	passwordEncryptKey       = "4969fj#k23#"
	bodyEncryptKey           = "xgx3d*fe3478$ukx"

	// license, licenseID and companyID are vendor constants required by the
	// undocumented AC Freedom / AUX Cloud API. They are not user secrets.
	license   = "PAFbJJ3WbvDxH5vvWezXN5BujETtH/iuTtIIW5CE/SeHN7oNKqnEajgljTcL0fBQQWM0XAAAAAAnBhJyhMi7zIQMsUcwR/PEwGA3uB5HLOnr+xRrci+FwHMkUtK7v4yo0ZHa+jPvb6djelPP893k7SagmffZmOkLSOsbNs8CAqsu8HuIDs2mDQAAAAA="
	licenseID = "3c015b249dd66ef0f11f9bef59ecd737"
	companyID = "48eb1b36cf0202ab2ef07b880ecda60d"

	spoofAppVersion  = "2.2.10.456537160"
	spoofUserAgent   = "Dalvik/2.1.0 (Linux; U; Android 12; SM-G991B Build/SP1A.210812.016)"
	spoofSystem      = "android"
	spoofAppPlatform = "android"
)

var (
	apiServers = map[string]string{
		"eu":  "https://app-service-deu-f0e9ebbb.smarthomecs.de",
		"usa": "https://app-service-usa-fd7cc04c.smarthomecs.com",
		"cn":  "https://app-service-chn-31a93883.ibroadlink.com",
	}

	aesIV = []byte{234, 170, 170, 58, 187, 88, 98, 162, 25, 24, 181, 119, 29, 22, 21, 170}
)

func hashPassword(password string) string {
	sum := sha1.Sum([]byte(password + passwordEncryptKey)) //nolint:gosec // AUX Cloud login protocol requires SHA-1
	return hex.EncodeToString(sum[:])
}

func loginToken(jsonPayload string) string {
	sum := md5.Sum([]byte(jsonPayload + bodyEncryptKey)) //nolint:gosec // AUX Cloud login protocol requires MD5
	return hex.EncodeToString(sum[:])
}

func timestampKey(ts int64) []byte {
	sum := md5.Sum([]byte(strconv.FormatInt(ts, 10) + timestampTokenEncryptKey)) //nolint:gosec // AUX Cloud login protocol requires MD5
	return sum[:]
}

func compactJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func encryptLoginBody(jsonPayload string, ts int64) ([]byte, error) {
	return encryptAESCBCZeroPad(timestampKey(ts), aesIV, []byte(jsonPayload))
}

func encryptAESCBCZeroPad(key, iv, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	padLen := aes.BlockSize - (len(data) % aes.BlockSize)
	padded := make([]byte, len(data)+padLen)
	copy(padded, data)
	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(out, padded)
	return out, nil
}

func decryptAESCBCZeroPad(key, iv, data []byte) ([]byte, error) {
	if len(data)%aes.BlockSize != 0 {
		return nil, errInvalidCiphertext
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(out, data)
	return bytes.TrimRight(out, "\x00"), nil
}
