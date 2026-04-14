package utils

import (
	"crypto/aes"
	"errors"
)

const padByte byte = 0x01

func UnpadPKCS7(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) == 0 || ciphertext[len(ciphertext)-1] != padByte {
		return nil, errors.New("invalid PKCS7 padding")
	}
	unpaddedText := make([]byte, len(ciphertext)-1)
	copy(unpaddedText, ciphertext[:len(ciphertext)-1])
	return unpaddedText, nil
}

// Pads the given plaintext data using PKCS7 padding,
// and returns the padded text as a byte slice.
func PadPKCS7(plaintext []byte) []byte {
	if len(plaintext)%aes.BlockSize == 0 {
		return plaintext
	}
	padding := aes.BlockSize - (len(plaintext) % aes.BlockSize)
	paddedText := make([]byte, len(plaintext)+padding)
	copy(paddedText, plaintext)
	for i := len(plaintext); i < len(paddedText); i++ {
		paddedText[i] = padByte
	}
	return paddedText
}
