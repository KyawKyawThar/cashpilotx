// Package crypto provides symmetric encryption helpers for the CashPilotX service.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

var (
	ErrInvalidKey = errors.New("key must be 16, 24, or 32 bytes (AES-128/192/256)")
	ErrAuthFailed = errors.New("decryption failed: ciphertext is tampered or key is wrong")
)

// Seal encrypts plaintext using AES-GCM with a random nonce.
// Returns (ciphertext, nonce, tag) — all three must be persisted together.
func Seal(key, plaintext []byte) (ciphertext, nonce, tag []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, nil, ErrInvalidKey
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, nil, err
	}
	nonce = make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, nil, err
	}
	// Seal appends ciphertext followed by the 16-byte auth tag.
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	tagSize := gcm.Overhead()
	return sealed[:len(sealed)-tagSize], nonce, sealed[len(sealed)-tagSize:], nil
}

// Open decrypts ciphertext previously produced by Seal.
func Open(key, ciphertext, nonce, tag []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidKey
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, append(ciphertext, tag...), nil)
	if err != nil {
		return nil, ErrAuthFailed
	}
	return plaintext, nil
}
