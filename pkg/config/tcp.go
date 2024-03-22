package config

import (
	"crypto/tls"
	_ "embed"
	"net"
	"time"
)

//go:embed local.monibuca.com_bundle.pem
var LocalCert []byte

//go:embed local.monibuca.com.key
var LocalKey []byte

type TCP struct {
	ListenAddr    string `desc:"监听地址，格式为ip:port，ip 可省略默认监听所有网卡"`
	ListenAddrTLS string `desc:"监听地址，格式为ip:port，ip 可省略默认监听所有网卡"`
	CertFile      string `desc:"证书文件"`
	KeyFile       string `desc:"私钥文件"`
	ListenNum     int    `desc:"同时并行监听数量，0为CPU核心数量"` //同时并行监听数量，0为CPU核心数量
	NoDelay       bool   `desc:"是否禁用Nagle算法"`        //是否禁用Nagle算法
	KeepAlive     bool   `desc:"是否启用KeepAlive"`      //是否启用KeepAlive
}

func (tcp *TCP) Listen(l net.Listener, handler func(*net.TCPConn)) {
	var tempDelay time.Duration
	for {
		conn, err := l.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && !ne.Timeout() {
				if tempDelay == 0 {
					tempDelay = 5 * time.Millisecond
				} else {
					tempDelay *= 2
				}
				if max := 1 * time.Second; tempDelay > max {
					tempDelay = max
				}
				// slog.Warnf("%s: Accept error: %v; retrying in %v", tcp.ListenAddr, err, tempDelay)
				time.Sleep(tempDelay)
				continue
			}
			return
		}
		var tcpConn *net.TCPConn
		switch v := conn.(type) {
		case *net.TCPConn:
			tcpConn = v
		case *tls.Conn:
			tcpConn = v.NetConn().(*net.TCPConn)
		}
		if !tcp.NoDelay {
			tcpConn.SetNoDelay(false)
		}
		tempDelay = 0
		go handler(tcpConn)
	}
}
