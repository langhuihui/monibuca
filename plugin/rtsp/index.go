package plugin_rtsp

import (
	"net"

	"m7s.live/m7s/v5/pkg/task"

	"m7s.live/m7s/v5"
	. "m7s.live/m7s/v5/plugin/rtsp/pkg"
)

const defaultConfig = m7s.DefaultYaml(`tcp:
  listenaddr: :554`)

var _ = m7s.InstallPlugin[RTSPPlugin](defaultConfig, NewPuller, NewPusher)

type RTSPPlugin struct {
	m7s.Plugin
}

func (p *RTSPPlugin) OnTCPConnect(conn *net.TCPConn) task.ITask {
	ret := &RTSPServer{NetConnection: NewNetConnection(conn), conf: p}
	ret.Logger = p.With("remote", conn.RemoteAddr().String())
	return ret
}

func (p *RTSPPlugin) OnDeviceAdd(device *m7s.Device) task.ITask {
	ret := &RTSPDevice{}
	ret.Device = device
	ret.Plugin = &p.Plugin
	ret.Logger = p.With("device", device.Name)
	device.Handler = ret
	return ret
}
