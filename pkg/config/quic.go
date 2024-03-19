package config

import (
	"context"
	"crypto/tls"
	"log/slog"

	"github.com/quic-go/quic-go"
)

type QuicConfig interface {
	ListenQuic(context.Context, QuicPlugin) error
}

type Quic struct {
	ListenAddr string `desc:"监听地址，格式为ip:port，ip 可省略默认监听所有网卡"`
	CertFile   string `desc:"证书文件"`
	KeyFile    string `desc:"私钥文件"`
}

func (q *Quic) ListenQuic(ctx context.Context, plugin QuicPlugin) error {
	listener, err := quic.ListenAddr(q.ListenAddr, q.generateTLSConfig(), &quic.Config{
		EnableDatagrams: true,
	})
	if err != nil {
		return err
	}
	slog.Info("quic listen", "addr", q.ListenAddr)
	for {
		conn, err := listener.Accept(ctx)
		if err != nil {
			return err
		}
		go plugin.ServeQuic(conn)
	}
}

func (q *Quic) generateTLSConfig() *tls.Config {
	// key, err := rsa.GenerateKey(rand.Reader, 1024)
	// if err != nil {
	// 	panic(err)
	// }
	// template := x509.Certificate{SerialNumber: big.NewInt(1)}
	// certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	// if err != nil {
	// 	panic(err)
	// }
	// keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	// certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	// tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)

	keyPair, err := tls.X509KeyPair(LocalCert, LocalKey)
	if q.CertFile != "" || q.KeyFile != "" {
		keyPair, err = tls.LoadX509KeyPair(q.CertFile, q.KeyFile)
	}
	if err != nil {
		slog.Error("LoadX509KeyPair", "error", err)
		panic(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{keyPair},
		NextProtos:   []string{"monibuca"},
	}
}
