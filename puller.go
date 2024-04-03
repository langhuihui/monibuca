package m7s

import "m7s.live/m7s/v5/pkg/config"

type PullHandler interface {
	Connect() error
	OnConnected()
	Disconnect()
	Pull() error
	Reconnect() bool
}

type Puller struct {
	Publisher
	PullHandler
	config.Pull
	RemoteURL      string // 远程服务器地址（用于推拉）
	ReConnectCount int    //重连次数
}

func (p *Puller) Start() error {
	return nil
}
