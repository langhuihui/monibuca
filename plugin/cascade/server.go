package plugin_cascade

import (
	"bufio"
	"net/http"
	"strconv"
	"strings"

	"m7s.live/v5"
	"m7s.live/v5/pkg/task"

	"github.com/quic-go/quic-go"
	cascade "m7s.live/v5/plugin/cascade/pkg"
)

type CascadeServerPlugin struct {
	m7s.Plugin
	AutoRegister bool                   `default:"true" desc:"下级自动注册"`
	RelayAPI     cascade.RelayAPIConfig `desc:"访问控制"`
}

var _ = m7s.InstallPlugin[CascadeServerPlugin]()

type CascadeServer struct {
	task.Work
	quic.Connection
	conf *CascadeServerPlugin
}

func (c *CascadeServerPlugin) OnQUICConnect(conn quic.Connection) task.ITask {
	ret := &CascadeServer{
		Connection: conn,
		conf:       c,
	}
	ret.Logger = c.Logger.With("remoteAddr", conn.RemoteAddr().String())
	return ret
}

func (task *CascadeServer) Go() {
	remoteAddr := task.Connection.RemoteAddr().String()
	stream, err := task.AcceptStream(task)
	if err != nil {
		task.Error("AcceptStream", "err", err)
		return
	}
	var secret string
	r := bufio.NewReader(stream)
	if secret, err = r.ReadString(0); err != nil {
		task.Error("read secret", "err", err)
		return
	}
	secret = secret[:len(secret)-1] // 去掉msg末尾的0
	var instance cascade.Instance
	child := &instance
	err = task.conf.DB.AutoMigrate(child)
	tx := task.conf.DB.First(child, "secret = ?", secret)
	if tx.Error == nil {
		cascade.SubordinateMap.Set(child)
	} else if task.conf.AutoRegister {
		child.Secret = secret
		child.IP = remoteAddr
		tx = task.conf.DB.First(child, "ip = ?", remoteAddr)
		if tx.Error != nil {
			task.conf.DB.Create(child)
		}
		cascade.SubordinateMap.Set(child)
	} else {
		task.Error("invalid secret:", "secret", secret)
		_, err = stream.Write([]byte{1, 0})
		return
	}
	child.IP = remoteAddr
	child.Online = true
	if child.Name == "" {
		child.Name = remoteAddr
	}
	task.conf.DB.Updates(child)
	child.Connection = task.Connection
	_, err = stream.Write([]byte{0, 0})
	err = stream.Close()
	task.Info("client register:", "remoteAddr", remoteAddr)
	for err == nil {
		var receiveRequestTask cascade.ReceiveRequestTask
		receiveRequestTask.Connection = task.Connection
		receiveRequestTask.Plugin = &task.conf.Plugin
		receiveRequestTask.Handler = task.conf.GetGlobalCommonConf().GetHandler()
		if receiveRequestTask.Stream, err = task.AcceptStream(task); err == nil {
			task.AddTask(&receiveRequestTask)
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
