package config

import (
	"crypto/tls"
	"log/slog"
	"net"
	"time"

	"m7s.live/v5/pkg/task"
)

type UDP struct {
	ListenAddr string `desc:"监听地址，格式为ip:port，ip 可省略默认监听所有网卡"`
	CertFile   string `desc:"证书文件"`
	KeyFile    string `desc:"私钥文件"`
	AutoListen bool   `default:"true" desc:"是否自动监听"`
}

func (config *UDP) CreateUDPWork(logger *slog.Logger, handler func(conn *net.UDPConn) task.ITask) *ListenUDPWork {
	ret := &ListenUDPWork{UDP: config, handler: handler}
	ret.Logger = logger.With("addr", config.ListenAddr)
	return ret
}

type ListenUDPWork struct {
	task.Work
	*UDP
	net.Listener
	handler func(conn *net.UDPConn) task.ITask
}

func (task *ListenUDPWork) Dispose() {
	task.Close()
}

func (task *ListenUDPWork) Start() (err error) {
	task.Listener, err = net.Listen("udp", task.ListenAddr)
	if err == nil {
		task.Info("listen udp")
	} else {
		task.Error("failed to listen udp", "error", err)
	}
	return
}

func (task *ListenUDPWork) Go() error {
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
			return err
		}
		var udpConn *net.UDPConn
		switch v := conn.(type) {
		case *net.UDPConn:
			udpConn = v
		case *tls.Conn:
			udpConn = v.NetConn().(*net.UDPConn)
		}
		tempDelay = 0
		subTask := task.handler(udpConn)
		task.AddTask(subTask)
	}
}
