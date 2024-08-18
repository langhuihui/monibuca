package plugin_cascade

import (
	"bufio"
	"m7s.live/m7s/v5"
	"net/http"
	"strconv"
	"strings"

	"github.com/quic-go/quic-go"
	"m7s.live/m7s/v5/plugin/cascade/pkg"
)

type CascadeServerPlugin struct {
	m7s.Plugin
	AutoRegister bool                   `default:"true" desc:"下级自动注册"`
	RelayAPI     cascade.RelayAPIConfig `desc:"访问控制"`
}

var _ = m7s.InstallPlugin[CascadeServerPlugin]()

func (c *CascadeServerPlugin) OnQUICConnect(conn quic.Connection) {
	remoteAddr := conn.RemoteAddr().String()
	c.Info("client connected:", "remoteAddr", remoteAddr)
	stream, err := conn.AcceptStream(c)
	if err != nil {
		c.Error("AcceptStream", "err", err)
		return
	}
	var secret string
	r := bufio.NewReader(stream)
	if secret, err = r.ReadString(0); err != nil {
		c.Error("read secret", "err", err)
		return
	}
	secret = secret[:len(secret)-1] // 去掉msg末尾的0
	var instance cascade.Instance
	child := &instance
	err = c.DB.AutoMigrate(child)
	tx := c.DB.First(child, "secret = ?", secret)
	if tx.Error == nil {
		cascade.SubordinateMap.Set(child)
	} else if c.AutoRegister {
		child.Secret = secret
		child.IP = remoteAddr
		tx = c.DB.First(child, "ip = ?", remoteAddr)
		if tx.Error != nil {
			c.DB.Create(child)
		}
		cascade.SubordinateMap.Set(child)
	} else {
		c.Error("invalid secret:", "secret", secret)
		_, err = stream.Write([]byte{1, 0})
		return
	}
	child.IP = remoteAddr
	child.Online = true
	if child.Name == "" {
		child.Name = remoteAddr
	}
	c.DB.Updates(child)
	child.Connection = conn
	_, err = stream.Write([]byte{0, 0})
	err = stream.Close()
	c.Info("client register:", "remoteAddr", remoteAddr)
	for err == nil {
		var receiveRequestTask cascade.ReceiveRequestTask
		receiveRequestTask.Connection = conn
		receiveRequestTask.Plugin = &c.Plugin
		receiveRequestTask.Handler = c.GetGlobalCommonConf().GetHandler()
		if receiveRequestTask.Stream, err = conn.AcceptStream(c); err == nil {
			c.AddTask(&receiveRequestTask)
		}
	}
}

// API_relay_ 用于转发请求, api/relay/:instanceId/*
func (c *CascadeServerPlugin) API_relay_(w http.ResponseWriter, r *http.Request) {
	paths := strings.Split(r.URL.Path, "/")
	instanceId, err := strconv.ParseUint(paths[3], 10, 32)
	instance, ok := cascade.SubordinateMap.Get(uint(instanceId))
	if err != nil || !ok {
		//util.ReturnError(util.APIErrorNotFound, "instance not found", w, r)
		return
	}
	relayURL := "/" + strings.Join(paths[4:], "/")
	r.URL.Path = relayURL
	if r.URL.RawQuery != "" {
		relayURL += "?" + r.URL.RawQuery
	}
	c.Debug("relayQuic", "relayURL", relayURL)
	var relayer cascade.Http2Quic
	relayer.Connection = instance.Connection
	relayer.Stream, err = instance.OpenStream()
	if err != nil {
		//util.ReturnError(util.APIErrorInternal, err.Error(), w, r)
	}
	c.AddTask(&relayer)
	relayer.ServeHTTP(w, r)
}

// API_list 用于获取所有下级, api/list
func (c *CascadeServerPlugin) API_list(w http.ResponseWriter, r *http.Request) {
	//util.ReturnFetchList(SubordinateMap.ToList, w, r)
}
