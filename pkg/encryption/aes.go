package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
)

func Encrypt(data []byte, key []byte) ([]byte, error) {
	counter, err := getCounter(key)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, counter.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return counter.Seal(nonce, nonce, data, nil), nil
}

func Decrypt(data []byte, key []byte) ([]byte, error) {
	counter, err := getCounter(key)
	if err != nil {
		return nil, err
	}

	nonceSize := counter.NonceSize()

	nonce, cyphertext := data[:nonceSize], data[nonceSize:]

	return counter.Open(nil, nonce, cyphertext, nil)
}

func getCounter(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	counter, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return counter, err
}
