package method

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

type ICryptor interface {
	Encrypt(origin []byte) ([]byte, error)
	Decrypt(encrypted []byte) ([]byte, error)
	GetKey() string // 获取密钥 格式：base64(key).base64(iv)
}

const (
	CryptoEncrypt = iota + 1
	CryptoDecrypt
)

type CryptoBuilder func(cfg Key) (ICryptor, error)

var (
	builders = make(map[string]CryptoBuilder)
)

func RegisterCryptor(name string, builder CryptoBuilder) {
	builders[name] = builder
}

func GetCryptor(cryptor string, cfg Key) (ICryptor, error) {
	builder, exists := builders[cryptor]
	if !exists {
		return nil, fmt.Errorf("Unknown ICryptor %q", cryptor)
	}
	return builder(cfg)
}

func CreateKey(keySize int) ([]byte, error) {
	key := make([]byte, keySize)
	_, err := rand.Read(key)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func Md5Sum(s string) string {
	ret := md5.Sum([]byte(s))
	return hex.EncodeToString(ret[:])
}
