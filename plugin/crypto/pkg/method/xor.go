package method

import (
	"crypto/subtle"
	"encoding/base64"
	"errors"
)

// SimpleXorCryptor 加密一次
type SimpleXorCryptor struct {
	key []byte
}

func newSimpleXor(cfg Key) (ICryptor, error) {
	var cryptor *SimpleXorCryptor
	if cfg.Key == "" {
		return nil, errors.New("xor cryptor config no key")
	} else {
		cryptor = &SimpleXorCryptor{key: []byte(cfg.Key)}
	}
	return cryptor, nil
}

// simpleXorEncryptDecrypt 对给定的字节数组进行 XOR 加密和解密
// key 是用于加密和解密的密钥
func simpleXorEncryptDecrypt(data []byte, key []byte) []byte {
	dataLen := len(data)
	result := make([]byte, dataLen)
	keyLen := len(key)
	for i := 0; i < dataLen; i += keyLen {
		end := i + keyLen
		if end > dataLen {
			end = dataLen
		}
		subtle.XORBytes(result[i:end], data[i:end], key[:end-i])
	}
	return result
}

func (c *SimpleXorCryptor) Encrypt(origin []byte) ([]byte, error) {
	return simpleXorEncryptDecrypt(origin, c.key), nil
}

func (c *SimpleXorCryptor) Decrypt(encrypted []byte) ([]byte, error) {
	return simpleXorEncryptDecrypt(encrypted, c.key), nil
}

func (c *SimpleXorCryptor) GetKey() string {
	return base64.RawStdEncoding.EncodeToString(c.key)
}

// 复杂的XOR加密器 加密两次
type ComplexXorCryptor struct {
	key []byte
	iv  []byte
}

func newComplexXor(cfg Key) (ICryptor, error) {
	var cryptor *ComplexXorCryptor
	if cfg.Key == "" {
		return nil, errors.New("xor cryptor config no key")
	} else {
		cryptor = &ComplexXorCryptor{key: []byte(cfg.Key), iv: []byte(cfg.Iv)}
	}
	return cryptor, nil
}

// complexXorEncryptDecrypt 对给定的字节数组进行 XOR 加密和解密
func complexXorEncryptDecrypt(arrayBuffer, key, iv []byte) []byte {
	// Assuming the key and iv have been provided and are not nil
	if key == nil || iv == nil {
		panic("key and iv must not be nil")
	}

	result := make([]byte, len(arrayBuffer))
	keyLen := len(key)
	ivLen := len(iv)

	for i := 0; i < len(result); i++ {
		result[i] = arrayBuffer[i] ^ (key[i%keyLen] ^ iv[i%ivLen])
	}
	return result
}

func (c *ComplexXorCryptor) Encrypt(origin []byte) ([]byte, error) {
	return complexXorEncryptDecrypt(origin, c.key, c.iv), nil
}

func (c *ComplexXorCryptor) Decrypt(encrypted []byte) ([]byte, error) {
	return complexXorEncryptDecrypt(encrypted, c.key, c.iv), nil
}

func (c *ComplexXorCryptor) GetKey() string {
	return base64.RawStdEncoding.EncodeToString(c.key) + "." + base64.RawStdEncoding.EncodeToString(c.iv)
}

func init() {
	RegisterCryptor("xor_s", newSimpleXor)
	RegisterCryptor("xor_c", newComplexXor)
}
