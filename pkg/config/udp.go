package config

import (
	"crypto/tls"
	"net"
	"time"
)

type UDP struct {
	ListenAddr string `desc:"监听地址，格式为ip:port，ip 可省略默认监听所有网卡"`
	CertFile   string `desc:"证书文件"`
	KeyFile    string `desc:"私钥文件"`
	AutoListen bool   `default:"true" desc:"是否自动监听"`
	listener   net.Listener
}

func (udp *UDP) Listen(handler func(*net.UDPConn)) (err error) {
	udp.listener, err = net.Listen("udp", udp.ListenAddr)
	if err == nil {
		go udp.listen(udp.listener, handler)
	}
	return
}

func (udp *UDP) listen(l net.Listener, handler func(conn *net.UDPConn)) {
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
		var udpConn *net.UDPConn
		switch v := conn.(type) {
		case *net.UDPConn:
			udpConn = v
		case *tls.Conn:
			udpConn = v.NetConn().(*net.UDPConn)
		}
		tempDelay = 0
		go handler(udpConn)
	}
}
