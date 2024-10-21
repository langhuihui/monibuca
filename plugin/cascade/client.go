package plugin_cascade

import (
	"crypto/tls"
	"fmt"
	"m7s.live/v5"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/plugin/cascade/pkg"
	"time"

	"github.com/quic-go/quic-go"
)

type CascadeClientPlugin struct {
	m7s.Plugin
	RelayAPI cascade.RelayAPIConfig `desc:"访问控制"`
	AutoPush bool                   `desc:"自动推流到上级"` //自动推流到上级
	Server   string                 `desc:"上级服务器"`   // TODO: support multiple servers
	Secret   string                 `desc:"连接秘钥"`
	conn     quic.Connection
}

var _ = m7s.InstallPlugin[CascadeClientPlugin](cascade.NewCascadePuller)

type CascadeClient struct {
	task.Task
	cfg *CascadeClientPlugin
	quic.Connection
}

func (task *CascadeClient) Start() (err error) {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"monibuca"},
	}
	cfg := task.cfg
	task.Connection, err = quic.DialAddr(cfg.Context, cfg.Server, tlsConf, &quic.Config{
		KeepAlivePeriod: time.Second * 10,
		EnableDatagrams: true,
	})
	if err != nil {
		return
	}
	var stream quic.Stream
	if stream, err = task.OpenStreamSync(task.cfg); err == nil {
		res := []byte{0}
		fmt.Fprintf(stream, "%s", task.cfg.Secret)
		stream.Write([]byte{0})
		_, err = stream.Read(res)
		if err == nil && res[0] == 0 {
			task.Info("connected to cascade server", "server", task.cfg.Server)
			stream.Close()
		} else {
			var zapErr any = err
			if err == nil {
				zapErr = res[0]
			}
			task.Error("connect to cascade server", "server", task.cfg.Server, "err", zapErr)
			return nil
		}
	}
	return
}

func (task *CascadeClient) Run() (err error) {
	for err == nil {
		var s quic.Stream
		if s, err = task.AcceptStream(task.Task.Context); err == nil {
			task.cfg.AddTask(&cascade.ReceiveRequestTask{
				Stream:     s,
				Handler:    task.cfg.GetGlobalCommonConf().GetHandler(),
				Connection: task.Connection,
				Plugin:     &task.cfg.Plugin,
			})
		}
	}
	return
}

func (c *CascadeClientPlugin) OnInit() (err error) {
	if c.Secret == "" && c.Server == "" {
		return nil
	}
	connectTask := CascadeClient{
		cfg: c,
	}
	connectTask.SetRetry(-1, time.Second)
	c.AddTask(&connectTask)
	return
}

func (c *CascadeClientPlugin) Pull(streamPath string, conf config.Pull) {
	puller := &cascade.Puller{
		Connection: c.conn,
	}
	puller.GetPullJob().Init(puller, &c.Plugin, streamPath, conf)
}

//func (c *CascadeClientPlugin) Start() {
//	retryDelay := [...]int{2, 3, 5, 8, 13}
//	for i := 0; c.Err() == nil; i++ {
//		connected, err := c.Remote()
//		if err == nil {
//			//不需要重试了，服务器返回了错误
//			return
//		}
//		c.Error("connect to cascade server ", "server", c.Server, "err", err)
//		if connected {
//			i = 0
//		} else if i >= 5 {
//			i = 4
//		}
//		time.Sleep(time.Second * time.Duration(retryDelay[i]))
//	}
//}

//func (c *CascadeClientPlugin) Remote() (wasConnected bool, err error) {
//	tlsConf := &tls.Config{
//		InsecureSkipVerify: true,
//		NextProtos:         []string{"monibuca"},
//	}
//	c.conn, err = quic.DialAddr(c, c.Server, tlsConf, &quic.Config{
//		KeepAlivePeriod: time.Second * 10,
//		EnableDatagrams: true,
//	})
//	wasConnected = err == nil
//	if stream := quic.Stream(nil); err == nil {
//		if stream, err = c.conn.OpenStreamSync(c); err == nil {
//			res := []byte{0}
//			fmt.Fprintf(stream, "%s", c.Secret)
//			stream.Write([]byte{0})
//			_, err = stream.Read(res)
//			if err == nil && res[0] == 0 {
//				c.Info("connected to cascade server", "server", c.Server)
//				stream.Close()
//			} else {
//				var zapErr any = err
//				if err == nil {
//					zapErr = res[0]
//				}
//				c.Error("connect to cascade server", "server", c.Server, "err", zapErr)
//				return false, nil
//			}
//		}
//	}
//
//	for err == nil {
//		var quicHttp cascade.QuicHTTP
//		//quicHttp.RelayAPIConfig = &c.RelayAPI
//		err = quicHttp.Accept(c.conn, &c.Plugin)
//	}
//	return wasConnected, err
//}
