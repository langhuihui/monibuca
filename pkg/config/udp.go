package config

import (
	"context"
	"net"
)

type UDPConfig interface {
	ListenUDP(context.Context, func(conn *net.UDPConn)) error
}

type UDP struct {
	ListenAddr string `desc:"监听地址，格式为ip:port，ip 可省略默认监听所有网卡"`
	CertFile   string `desc:"证书文件"`
	KeyFile    string `desc:"私钥文件"`
	AutoListen bool   `default:"true" desc:"是否自动监听"`
}
