package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	encPrefix  = "enc:"
	saltLen    = 16
	nonceLen   = 12
	keyLen     = 32
	a2Time     = 3
	a2Memory   = 64 * 1024 // 64 MB
	a2Threads  = 4
)

// IsEncrypted reports whether s was produced by Encrypt.
func IsEncrypted(s string) bool {
	return strings.HasPrefix(s, encPrefix)
}

// Encrypt encrypts plaintext using masterPwd.
// The result has the form "enc:<base64(salt|nonce|ciphertext)>".
func Encrypt(masterPwd, plaintext string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", err
	}

	key := deriveKey(masterPwd, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ct := gcm.Seal(nil, nonce, []byte(plaintext), nil)

	packed := make([]byte, 0, saltLen+nonceLen+len(ct))
	packed = append(packed, salt...)
	packed = append(packed, nonce...)
	packed = append(packed, ct...)

	return encPrefix + base64.StdEncoding.EncodeToString(packed), nil
}

// Decrypt decrypts a value produced by Encrypt.
// If the value is not encrypted (no "enc:" prefix) it is returned unchanged.
// Returns an error if masterPwd is wrong.
func Decrypt(masterPwd, value string) (string, error) {
	if !IsEncrypted(value) {
		return value, nil
	}

	packed, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, encPrefix))
	if err != nil {
		return "", fmt.Errorf("invalid encrypted value: %w", err)
	}
	if len(packed) < saltLen+nonceLen+1 {
		return "", errors.New("encrypted data too short")
	}

	salt := packed[:saltLen]
	nonce := packed[saltLen : saltLen+nonceLen]
	ct := packed[saltLen+nonceLen:]

	key := deriveKey(masterPwd, salt)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", errors.New("decryption failed — wrong master password?")
	}
	return string(plain), nil
}

func deriveKey(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, a2Time, a2Memory, a2Threads, keyLen)
}
