package config

import (
	"crypto/tls"
	_ "embed"
	"log/slog"
	"net"
	"runtime"
	"time"

	"m7s.live/v5/pkg/task"
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
}

func (config *TCP) CreateTCPWork(logger *slog.Logger, handler TCPHandler) *ListenTCPWork {
	ret := &ListenTCPWork{TCP: config, handler: handler}
	ret.SetDescription("listenAddr", config.ListenAddr)
	ret.Logger = logger.With("addr", config.ListenAddr)
	return ret
}

func (config *TCP) CreateTCPTLSWork(logger *slog.Logger, handler TCPHandler) *ListenTCPTLSWork {
	ret := &ListenTCPTLSWork{ListenTCPWork{TCP: config, handler: handler}}
	ret.SetDescription("listenAddr", config.ListenAddrTLS)
	ret.Logger = logger.With("addr", config.ListenAddrTLS)
	return ret
}

type TCPHandler = func(conn *net.TCPConn) task.ITask

type ListenTCPWork struct {
	task.Work
	*TCP
	net.Listener
	handler TCPHandler
}

func (task *ListenTCPWork) Start() (err error) {
	task.Listener, err = net.Listen("tcp", task.ListenAddr)
	if err == nil {
		task.Info("listen tcp")
	} else {
		task.Error("failed to listen tcp", "error", err)
	}
	if task.handler == nil {
		return nil
	}
	count := task.ListenNum
	if count == 0 {
		count = runtime.NumCPU()
	}
	for range count {
		go task.listen(task.handler)
	}
	return
}

func (task *ListenTCPWork) Dispose() {
	task.Info("tcp server stop")
	task.Listener.Close()
}

type ListenTCPTLSWork struct {
	ListenTCPWork
}

func (task *ListenTCPTLSWork) Start() (err error) {
	var tlsConfig *tls.Config
	if tlsConfig, err = GetTLSConfig(task.CertFile, task.KeyFile); err != nil {
		return
	}
	task.Listener, err = tls.Listen("tcp", task.ListenAddrTLS, tlsConfig)
	if err == nil {
		task.Info("listen tcp tls")
	} else {
		task.Error("failed to listen tcp tls", "error", err)
	}
	return
}

func (task *ListenTCPWork) listen(handler TCPHandler) {
	var tempDelay time.Duration
	for {
		conn, err := task.Accept()
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
		if !task.NoDelay {
			tcpConn.SetNoDelay(false)
		}
		tempDelay = 0
		subTask := handler(tcpConn)
		task.AddTask(subTask)
	}
}
