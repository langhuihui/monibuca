package rtsp

import (
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/langhuihui/gomem"
	"m7s.live/v5"
)

func NewPullProxy() m7s.IPullProxy {
	return &RTSPPullProxy{}
}

type RTSPPullProxy struct {
	m7s.TCPPullProxy
	conn Stream
}

func (d *RTSPPullProxy) Start() (err error) {
	d.URL, err = url.Parse(d.PullProxyConfig.URL)
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
		MemoryAllocator: gomem.NewScalableMemoryAllocator(1 << 12),
		UserAgent:       "monibuca" + m7s.Version,
	}
	d.conn.Logger = d.Logger
	return d.TCPPullProxy.Start()
}

func (d *RTSPPullProxy) Dispose() {
	if d.conn.NetConnection != nil && d.conn.NetConnection.Conn != nil {
		_ = d.conn.Teardown()
		d.conn.NetConnection.Dispose()
		d.conn.NetConnection = nil
	}
	d.TCPPullProxy.Dispose()
	d.Info("RTSP pull proxy disposed and all resources cleaned up")
}

func (d *RTSPPullProxy) GetTickInterval() time.Duration {
	return time.Second * 5
}

func (d *RTSPPullProxy) Tick(any) {
	var err error
	switch d.Status {
	case m7s.PullProxyStatusOffline:
		err = d.conn.Connect(d.Context, d.PullProxyConfig.URL)
		if err != nil {
			return
		}
		d.ChangeStatus(m7s.PullProxyStatusOnline)
	case m7s.PullProxyStatusOnline, m7s.PullProxyStatusPulling:
		if d.conn.Conn == nil {
			err = d.conn.Connect(d.Context, d.PullProxyConfig.URL)
		} else {
			t := time.Now()
			err = d.conn.Options()
			d.RTT = time.Since(t)
		}
		if err != nil {
			d.ChangeStatus(m7s.PullProxyStatusOffline)
		}
	}
}
