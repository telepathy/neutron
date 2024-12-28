package service

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
)

func Encrypt(password string, key string) (string, error) {
	salt := padKey(key)
	block, err := aes.NewCipher([]byte(salt))
	if err != nil {
		return "", err
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}
	stream := cipher.NewCFBEncrypter(block, iv)
	cipherText := make([]byte, len(password))
	stream.XORKeyStream(cipherText, []byte(password))
	finalCiphertext := append(iv, cipherText...)
	return base64.StdEncoding.EncodeToString(finalCiphertext), nil
}

func Decrypt(password string, key string) (string, error) {
	salt := padKey(key)
	data, err := base64.StdEncoding.DecodeString(password)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher([]byte(salt))
	if err != nil {
		return "", err
	}
	if len(data) < aes.BlockSize {
		return "", err
	}
	iv := data[:aes.BlockSize]
	cipherText := data[aes.BlockSize:]
	stream := cipher.NewCFBDecrypter(block, iv)
	plainText := make([]byte, len(cipherText))
	stream.XORKeyStream(plainText, cipherText)
	return string(plainText), nil
}

func padKey(key string) string {
	const blockSize = aes.BlockSize
	if len(key) < blockSize {
		return key + string(bytes.Repeat([]byte("0"), blockSize-len(key)))
	}
	return key[:blockSize]
}
