package config

import (
	"crypto/tls"
	_ "embed"
	"net"
	"runtime"
	"time"
)

//go:embed local.monibuca.com_bundle.pem
var LocalCert []byte

//go:embed local.monibuca.com.key
var LocalKey []byte

func GetTLSConfig(certFile, keyFile string) (tslConfig *tls.Config, err error) {
	var keyPair tls.Certificate
	if certFile != "" || keyFile != "" {
		keyPair, err = tls.LoadX509KeyPair(certFile, keyFile)
	} else {
		keyPair, err = tls.X509KeyPair(LocalCert, LocalKey)
	}
	if err == nil {
		tslConfig = &tls.Config{
			Certificates: []tls.Certificate{keyPair},
			NextProtos:   []string{"monibuca"},
		}
	}
	return
}

type TCP struct {
	ListenAddr    string `desc:"监听地址，格式为ip:port，ip 可省略默认监听所有网卡"`
	ListenAddrTLS string `desc:"监听地址，格式为ip:port，ip 可省略默认监听所有网卡"`
	CertFile      string `desc:"证书文件"`
	KeyFile       string `desc:"私钥文件"`
	ListenNum     int    `desc:"同时并行监听数量，0为CPU核心数量"` //同时并行监听数量，0为CPU核心数量
	NoDelay       bool   `desc:"是否禁用Nagle算法"`        //是否禁用Nagle算法
	KeepAlive     bool   `desc:"是否启用KeepAlive"`      //是否启用KeepAlive
	AutoListen    bool   `default:"true" desc:"是否自动监听"`
	listener      net.Listener
	listenerTls   net.Listener
}

func (tcp *TCP) StopListen() {
	if tcp.listener != nil {
		tcp.listener.Close()
	}
	if tcp.listenerTls != nil {
		tcp.listenerTls.Close()
	}
}

func (tcp *TCP) Listen(handler func(*net.TCPConn)) (err error) {
	tcp.listener, err = net.Listen("tcp", tcp.ListenAddr)
	if err == nil {
		count := tcp.ListenNum
		if count == 0 {
			count = runtime.NumCPU()
		}
		for range count {
			go tcp.listen(tcp.listener, handler)
		}
	}
	return
}

func (tcp *TCP) ListenTLS(handler func(*net.TCPConn)) (err error) {
	if tlsConfig, err := GetTLSConfig(tcp.CertFile, tcp.KeyFile); err == nil {
		tcp.listenerTls, err = tls.Listen("tcp", tcp.ListenAddrTLS, tlsConfig)
		if err == nil {
			count := tcp.ListenNum
			if count == 0 {
				count = runtime.NumCPU()
			}
			for range count {
				go tcp.listen(tcp.listenerTls, handler)
			}
		}
	} else {
		return err
	}
	return
}

func (tcp *TCP) listen(l net.Listener, handler func(*net.TCPConn)) {
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
