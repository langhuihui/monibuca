package method

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
)

//加密过程：
//  1、处理数据，对数据进行填充，采用PKCS7（当密钥长度不够时，缺几位补几个几）的方式。
//  2、对数据进行加密，采用AES加密方法中CBC加密模式
//  3、对得到的加密数据，进行base64加密，得到字符串
// 解密过程相反

// AesEncrypt 加密  cbc 模式
type AesCryptor struct {
	key []byte
}

func newAesCbc(cfg Key) (ICryptor, error) {
	var cryptor *AesCryptor
	if cfg.Key == "" {
		return nil, errors.New("aes cryptor config no key")
	} else {
		cryptor = &AesCryptor{key: []byte(cfg.Key)}
	}
	return cryptor, nil
}

func init() {
	RegisterCryptor("aes_cbc", newAesCbc)
}

func (c *AesCryptor) Encrypt(origin []byte) ([]byte, error) {
	//创建加密实例
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}
	//判断加密快的大小
	blockSize := block.BlockSize()
	//填充
	encryptBytes := pkcs7Padding(origin, blockSize)
	//初始化加密数据接收切片
	crypted := make([]byte, len(encryptBytes))
	//使用cbc加密模式
	blockMode := cipher.NewCBCEncrypter(block, c.key[:blockSize])
	//执行加密
	blockMode.CryptBlocks(crypted, encryptBytes)
	return crypted, nil
}

func (c *AesCryptor) Decrypt(encrypted []byte) ([]byte, error) {
	//创建实例
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}
	//获取块的大小
	blockSize := block.BlockSize()
	//使用cbc
	blockMode := cipher.NewCBCDecrypter(block, c.key[:blockSize])
	//初始化解密数据接收切片
	crypted := make([]byte, len(encrypted))
	//执行解密
	blockMode.CryptBlocks(crypted, encrypted)
	//去除填充
	crypted, err = pkcs7UnPadding(crypted)
	if err != nil {
		return nil, err
	}
	return crypted, nil
}

func (c *AesCryptor) GetKey() string {
	return base64.RawStdEncoding.EncodeToString(c.key)
}

// pkcs7Padding 填充
func pkcs7Padding(data []byte, blockSize int) []byte {
	//判断缺少几位长度。最少1，最多 blockSize
	padding := blockSize - len(data)%blockSize
	//补足位数。把切片[]byte{byte(padding)}复制padding个
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

// pkcs7UnPadding 填充的反向操作
func pkcs7UnPadding(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, errors.New("加密字符串错误！")
	}
	//获取填充的个数
	unPadding := int(data[length-1])
	return data[:(length - unPadding)], nil
}
