package plugin_crypto

import (
	m7s "m7s.live/v5"
	crypto "m7s.live/v5/plugin/crypto/pkg"
)

var _ = m7s.InstallPlugin[CryptoPlugin](crypto.NewTransform)

type CryptoPlugin struct {
	m7s.Plugin
	IsStatic   bool   `desc:"是否静态密钥" default:"false"`
	Algo       string `desc:"加密算法" default:"aes_ctr"` //加密算法
	EncryptLen int    `desc:"加密字节长度" default:"1024"`  //加密字节长度
	Secret     struct {
		Key string `desc:"加密密钥" default:"your key"` //加密密钥
		Iv  string `desc:"加密向量" default:"your iv"`  //加密向量
	} `desc:"密钥配置"`
}

// OnInit 初始化插件时的回调函数
func (p *CryptoPlugin) OnInit() (err error) {
	// 初始化全局配置
	crypto.GlobalConfig = crypto.Config{
		IsStatic:   p.IsStatic,
		Algo:       p.Algo,
		EncryptLen: p.EncryptLen,
		Secret: struct {
			Key string `desc:"加密密钥" default:"your key"`
			Iv  string `desc:"加密向量" default:"your iv"`
		}{
			Key: p.Secret.Key,
			Iv:  p.Secret.Iv,
		},
	}

	p.Info("crypto config initialized",
		"algo", p.Algo,
		"isStatic", p.IsStatic,
		"encryptLen", p.EncryptLen,
	)

	return nil
}
