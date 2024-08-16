package config

import (
	"context"
	"log/slog"

	"github.com/quic-go/quic-go"
)

type QuicConfig interface {
	ListenQuic(context.Context, func(connection quic.Connection)) error
}

type Quic struct {
	ListenAddr string `desc:"监听地址，格式为ip:port，ip 可省略默认监听所有网卡"`
	CertFile   string `desc:"证书文件"`
	KeyFile    string `desc:"私钥文件"`
	AutoListen bool   `default:"true" desc:"是否自动监听"`
}

func (q *Quic) ListenQuic(ctx context.Context, handler func(connection quic.Connection)) error {
	ltsc, err := GetTLSConfig(q.CertFile, q.KeyFile)
	if err != nil {
		return err
	}
	listener, err := quic.ListenAddr(q.ListenAddr, ltsc, &quic.Config{
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
		go handler(conn)
	}
}
