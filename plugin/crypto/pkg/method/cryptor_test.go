package method

import (
	"encoding/base64"
	"testing"
)

func TestStream(t *testing.T) {
	encKey, _ := CreateKey(32)
	macKey, _ := CreateKey(32)

	plaintext := "0123456789012345"
	pt := []byte(plaintext)
	var cfg Key
	cfg.EncKey = string(encKey)
	cfg.MacKey = string(macKey)
	c, _ := GetCryptor("stream", cfg)
	t.Log("key", c.GetKey())
	encryptData, err := c.Encrypt(pt)
	t.Log("stream encrypt base64", base64.RawStdEncoding.EncodeToString(encryptData), err)
	decryptData, err := c.Decrypt(encryptData)
	t.Log("stream decrypt", string(decryptData), err)
	if string(decryptData) != plaintext {
		t.Error("decrypt error")
	}

}

func TestAesCbc(t *testing.T) {

	encKey, _ := CreateKey(16)

	plaintext := "0123456789012345"
	pt := []byte(plaintext)

	var cfg Key
	cfg.Key = string(encKey)
	c, _ := GetCryptor("aes_cbc", cfg)
	t.Log(c.GetKey())
	encryptData, err := c.Encrypt(pt)
	t.Log("aes_cbc encrypt base64", base64.RawStdEncoding.EncodeToString(encryptData), err)
	decryptData, err := c.Decrypt(encryptData)
	t.Log("aes_cbc decrypt", string(decryptData), err)

	if string(decryptData) != plaintext {
		t.Error("decrypt error")
	}
}

func TestAesCtr(t *testing.T) {

	encKey, _ := CreateKey(32)
	iv, _ := CreateKey(16)
	plaintext := "0123456789012345"
	pt := []byte(plaintext)
	var cfg Key
	cfg.Key = string(encKey)
	cfg.Iv = string(iv)

	c, _ := GetCryptor("aes_ctr", cfg)
	t.Log(c.GetKey())
	encryptData, err := c.Encrypt(pt)
	t.Log("aes_ctr encrypt ", string(encryptData), err)
	decryptData, err := c.Decrypt(encryptData)
	t.Log("aes_ctr decrypt", string(decryptData), err)

	if string(decryptData) != plaintext {
		t.Error("decrypt error")
	}
}

func TestXor(t *testing.T) {

	encKey, _ := CreateKey(32)
	iv, _ := CreateKey(16)
	plaintext := "0123456789012345"
	pt := []byte(plaintext)
	var cfg Key
	cfg.Key = string(encKey)
	cfg.Iv = string(iv)

	c, _ := GetCryptor("xor", cfg)
	t.Log(c.GetKey())
	encryptData, err := c.Encrypt(pt)
	t.Log("xor encrypt ", string(encryptData), "len", len(string(encryptData)), err)
	decryptData, err := c.Decrypt(encryptData)
	t.Log("xor decrypt", string(decryptData), err)

	if string(decryptData) != plaintext {
		t.Error("decrypt error")
	}
}
