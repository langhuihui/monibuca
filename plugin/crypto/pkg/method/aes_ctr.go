package method

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"fmt"
)

type AesCtrCryptor struct {
	key []byte
	iv  []byte
}

func newAesCtr(cfg Key) (ICryptor, error) {
	var cryptor *AesCtrCryptor
	if cfg.Key == "" || cfg.Iv == "" {
		return nil, errors.New("aes ctr cryptor config no key")
	}
	cryptor = &AesCtrCryptor{key: []byte(cfg.Key), iv: []byte(cfg.Iv)}

	return cryptor, nil
}

func init() {
	RegisterCryptor("aes_ctr", newAesCtr)
}

func (c *AesCtrCryptor) Encrypt(origin []byte) ([]byte, error) {

	block, err := aes.NewCipher(c.key)
	if err != nil {
		panic(err)
	}

	aesCtr := cipher.NewCTR(block, c.iv)

	// EncryptRaw the plaintext
	ciphertext := make([]byte, len(origin))
	aesCtr.XORKeyStream(ciphertext, origin)
	return ciphertext, nil
}

func (c *AesCtrCryptor) Decrypt(encrypted []byte) ([]byte, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		panic(err)
	}

	aesCtr := cipher.NewCTR(block, c.iv)

	// Decrypt the ciphertext
	plaintext := make([]byte, len(encrypted))
	aesCtr.XORKeyStream(plaintext, encrypted)
	return plaintext, nil
}

func (c *AesCtrCryptor) GetKey() string {
	return fmt.Sprintf("%s.%s", base64.RawStdEncoding.EncodeToString(c.key), base64.RawStdEncoding.EncodeToString(c.iv))
}
