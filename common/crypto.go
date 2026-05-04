package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"

	"golang.org/x/crypto/bcrypt"
)

func Password2Hash(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

func ValidatePasswordAndHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func Encrypt(text string) (string, error) {
	if CryptoSecret == "" {
		return base64.StdEncoding.EncodeToString([]byte(text)), nil
	}
	block, err := aes.NewCipher([]byte(padKey(CryptoSecret)))
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := aesGCM.Seal(nonce, nonce, []byte(text), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func Decrypt(encrypted string) (string, error) {
	if CryptoSecret == "" {
		data, err := base64.StdEncoding.DecodeString(encrypted)
		return string(data), err
	}
	data, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher([]byte(padKey(CryptoSecret)))
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := aesGCM.NonceSize()
	if len(data) < nonceSize {
		return "", err
	}
	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func padKey(key string) string {
	if len(key) >= 32 {
		return key[:32]
	}
	for len(key) < 32 {
		key += key
	}
	return key[:32]
}
