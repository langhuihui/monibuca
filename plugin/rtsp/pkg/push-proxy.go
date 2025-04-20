package rtsp

import (
	"fmt"
	"net"
	"net/url"
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg/util"
)

func NewPushProxy() m7s.IPushProxy {
	return &RTSPPushProxy{}
}

type RTSPPushProxy struct {
	m7s.TCPPushProxy
	conn Stream
}

func (d *RTSPPushProxy) Start() (err error) {
	urlStr := d.PushProxyConfig.URL
	d.URL, err = url.Parse(urlStr)
	if err != nil {
		return
	}
	if ips, err := net.LookupIP(d.URL.Hostname()); err != nil {
		return err
	} else if len(ips) == 0 {
		return fmt.Errorf("no IP found for host: %s", d.URL.Hostname())
	} else {
		d.TCPAddr, err = net.ResolveTCPAddr("tcp", net.JoinHostPort(ips[0].String(), d.URL.Port()))
		if err != nil {
			return err
		}
		if d.TCPAddr.Port == 0 {
			d.TCPAddr.Port = 554 // Default RTSP port
		}
	}

	d.conn.NetConnection = &NetConnection{
		MemoryAllocator: util.NewScalableMemoryAllocator(1 << 12),
		UserAgent:       "monibuca" + m7s.Version,
	}
	d.conn.Logger = d.Logger
	return d.TCPPushProxy.Start()
}

func (d *RTSPPushProxy) Tick(any) {
	switch d.Status {
	case m7s.PushProxyStatusOffline:
		err := d.conn.Connect(d.URL.String())
		if err != nil {
			return
		}
		d.ChangeStatus(m7s.PushProxyStatusOnline)
	case m7s.PushProxyStatusOnline, m7s.PushProxyStatusPushing:
		t := time.Now()
		err := d.conn.Options()
		d.RTT = time.Since(t)
		if err != nil {
			d.ChangeStatus(m7s.PushProxyStatusOffline)
		}
	}
}

func (d *RTSPPushProxy) Dispose() {
	// 先停止任何正在进行的操作
	if d.conn.NetConnection != nil {
		// 尝试发送TEARDOWN信令
		_ = d.conn.Teardown()

		// 确保所有资源正确释放
		d.conn.NetConnection.Dispose()
		d.conn.NetConnection = nil
	}

	// 调用父类的Dispose方法
	d.TCPPushProxy.Dispose()
	d.Info("RTSP push proxy disposed and all resources cleaned up")
}
