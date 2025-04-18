package plugin_rtsp

import (
	"fmt"
	"net"
	"strings"

	"m7s.live/v5/pkg/task"

	"m7s.live/v5"
	. "m7s.live/v5/plugin/rtsp/pkg"
)

var _ = m7s.InstallPlugin[RTSPPlugin](m7s.PluginMeta{
	DefaultYaml: `tcp:
  listenaddr: :554
udp:
  portrange: 20000-30000     # UDP端口范围，用于UDP传输模式
  portpoolsize: 100          # 预分配的UDP端口池大小`,
	NewPuller:    NewPuller,
	NewPusher:    NewPusher,
	NewPullProxy: NewPullProxy,
	NewPushProxy: NewPushProxy,
})

type RTSPPlugin struct {
	m7s.Plugin
	UDP struct {
		PortRange    string `desc:"UDP端口范围，用于UDP传输模式"`
		PortPoolSize int    `default:"100" desc:"预分配的UDP端口池大小"`
	}
	udpPortRange []uint16    // 解析后的UDP端口范围
	udpPortPool  chan uint16 // UDP端口池，用于分配可用端口
}

func (p *RTSPPlugin) OnTCPConnect(conn *net.TCPConn) task.ITask {
	ret := &RTSPServer{NetConnection: NewNetConnection(conn), conf: p}
	ret.Logger = p.With("remote", conn.RemoteAddr().String())
	return ret
}

func (p *RTSPPlugin) OnInit() (err error) {
	if tcpAddr := p.GetCommonConf().TCP.ListenAddr; tcpAddr != "" {
		_, port, _ := strings.Cut(tcpAddr, ":")
		if port == "554" {
			p.PlayAddr = append(p.PlayAddr, "rtsp://{hostName}/{streamPath}")
			p.PushAddr = append(p.PushAddr, "rtsp://{hostName}/{streamPath}")
		} else {
			p.PlayAddr = append(p.PlayAddr, fmt.Sprintf("rtsp://{hostName}:%s/{streamPath}", port))
			p.PushAddr = append(p.PushAddr, fmt.Sprintf("rtsp://{hostName}:%s/{streamPath}", port))
		}
	}

	// 初始化UDP端口池
	p.initUDPPortPool()
	return
}

// 初始化UDP端口池
func (p *RTSPPlugin) initUDPPortPool() {
	// 解析端口范围
	var startPort, endPort uint16 = 20000, 30000 // 默认值
	if p.UDP.PortRange != "" {
		fmt.Sscanf(p.UDP.PortRange, "%d-%d", &startPort, &endPort)
	}

	// 初始化端口范围
	p.udpPortRange = []uint16{startPort, endPort}

	// 确定池大小
	poolSize := p.UDP.PortPoolSize
	if poolSize <= 0 {
		poolSize = 100
	}

	// 创建端口池
	p.udpPortPool = make(chan uint16, poolSize)

	// 预填充端口池
	for i := 0; i < poolSize; i++ {
		// 每个媒体需要2个端口(RTP+RTCP)，所以步进2
		port := startPort + uint16(i*2)
		if port+1 <= endPort {
			p.udpPortPool <- port
		} else {
			break
		}
	}

	p.Info("UDP port pool initialized", "size", len(p.udpPortPool), "range", fmt.Sprintf("%d-%d", startPort, endPort))
}

// 获取一个可用的UDP端口对(RTP端口和RTCP端口)
func (p *RTSPPlugin) GetUDPPort() (rtpPort uint16, rtcpPort uint16, err error) {
	select {
	case rtpPort = <-p.udpPortPool:
		rtcpPort = rtpPort + 1
		return
	default:
		err = fmt.Errorf("no available UDP port in pool")
		return
	}
}

// 释放一个UDP端口对回端口池
func (p *RTSPPlugin) ReleaseUDPPort(rtpPort uint16) {
	// 检查端口是否在有效范围内
	if rtpPort >= p.udpPortRange[0] && rtpPort+1 <= p.udpPortRange[1] {
		// 尝试非阻塞方式放回池中，如果池已满则丢弃
		select {
		case p.udpPortPool <- rtpPort:
			p.Debug("UDP port released back to pool", "port", rtpPort)
		default:
			p.Debug("UDP port pool is full, port discarded", "port", rtpPort)
		}
	}
}
