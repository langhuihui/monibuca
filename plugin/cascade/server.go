package plugin_cascade

import (
	"bufio"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/protobuf/types/known/timestamppb"
	"m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"

	"context"

	"github.com/quic-go/quic-go"
	"m7s.live/v5/plugin/cascade/pb"
	cascade "m7s.live/v5/plugin/cascade/pkg"
)

type CascadeServerPlugin struct {
	m7s.Plugin
	pb.UnimplementedServerServer
	AutoRegister bool                   `default:"true" desc:"下级自动注册"`
	RelayAPI     cascade.RelayAPIConfig `desc:"访问控制"`
	clients      util.Collection[uint, *cascade.Instance]
}

func (c *CascadeServerPlugin) Start() (err error) {
	if c.GetCommonConf().Quic.ListenAddr == "" {
		return pkg.ErrNotListen
	}
	c.clients.L = &sync.RWMutex{}
	if c.DB == nil {
		return pkg.ErrNoDB
	}
	c.DB.AutoMigrate(&cascade.Instance{})
	var instance []*cascade.Instance
	c.DB.Find(&instance)
	for _, instance := range instance {
		c.clients.Add(instance)
		if instance.Online {
			instance.Online = false
			c.DB.Save(instance)
		}
	}
	return
}

var _ = m7s.InstallPlugin[CascadeServerPlugin](m7s.PluginMeta{
	DefaultYaml: `quic:
  listenaddr: :44944`,
	ServiceDesc:         &pb.Server_ServiceDesc,
	RegisterGRPCHandler: pb.RegisterServerHandler,
})

type CascadeServer struct {
	task.Work
	quic.Connection
	conf   *CascadeServerPlugin
	client *cascade.Instance
}

func (c *CascadeServerPlugin) OnQUICConnect(conn quic.Connection) task.ITask {
	ret := &CascadeServer{
		Connection: conn,
		conf:       c,
	}
	ret.Logger = c.Logger.With("remoteAddr", conn.RemoteAddr().String())
	return ret
}

func (task *CascadeServer) Go() (err error) {
	remoteAddr := task.Connection.RemoteAddr().String()
	var stream quic.Stream
	if stream, err = task.AcceptStream(task); err != nil {
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
	child := &cascade.Instance{}
	if secret != "" {
		tx := task.conf.DB.First(child, "secret = ?", secret)
		err = tx.Error
	} else {
		tx := task.conf.DB.First(child, "ip = ?", remoteAddr)
		err = tx.Error
	}
	if err == nil {
		task.conf.clients.Set(child)
	} else if task.conf.AutoRegister {
		child.Secret = sql.NullString{String: secret, Valid: secret != ""}
		child.IP = remoteAddr
		err = task.conf.DB.First(child, "ip = ?", remoteAddr).Error
		if err != nil {
			err = task.conf.DB.Create(child).Error
		}
		task.conf.clients.Set(child)
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
	err = task.conf.DB.Updates(child).Error
	child.Connection = task.Connection
	task.client = child
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
	return
}

func (task *CascadeServer) Dispose() {
	if task.Connection != nil {
		task.Connection.CloseWithError(quic.ApplicationErrorCode(0), task.StopReason().Error())
	}
	if task.client != nil {
		task.client.Online = false
		task.conf.DB.Save(task.client)
	}
}

// API_relay_ 用于转发请求, api/relay/:instanceId/*
func (c *CascadeServerPlugin) API_relay_(w http.ResponseWriter, r *http.Request) {
	paths := strings.Split(r.URL.Path, "/")
	instanceId, err := strconv.ParseUint(paths[3], 10, 32)
	instance, ok := c.clients.Get(uint(instanceId))
	if err != nil || !ok {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	if !instance.Online {
		http.Error(w, "instance offline", http.StatusServiceUnavailable)
		return
	}
	relayURL := "/" + strings.Join(paths[4:], "/")
	r.URL.Path = relayURL
	if r.URL.RawQuery != "" {
		relayURL += "?" + r.URL.RawQuery
	}
	c.Debug("relayQuic", "relayURL", relayURL)
	var relayer cascade.Http2Quic
	relayer.Stream, err = instance.OpenStreamSync(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	relayer.ServeHTTP(w, r)
}

func (c *CascadeServerPlugin) GetClientList(ctx context.Context, req *pb.GetClientListRequest) (clientList *pb.GetClientListResponse, err error) {
	clientList = &pb.GetClientListResponse{}

	for instance := range c.clients.Range {
		client := &pb.CascadeClient{
			Id:          uint32(instance.ID),
			Name:        instance.Name,
			Ip:          instance.IP,
			Online:      instance.Online,
			CreatedTime: timestamppb.New(instance.CreatedAt),
			UpdatedTime: timestamppb.New(instance.UpdatedAt),
		}
		clientList.Data = append(clientList.Data, client)
	}

	return
}

func (c *CascadeServerPlugin) CreateClient(ctx context.Context, req *pb.CreateClientRequest) (result *pb.CreateClientResponse, err error) {
	result = &pb.CreateClientResponse{}

	instance := &cascade.Instance{
		Name:   req.Name,
		Secret: sql.NullString{String: req.Secret, Valid: req.Secret != ""},
	}

	if err = c.DB.Create(instance).Error; err != nil {
		return
	}

	c.clients.Set(instance)

	result.Data = &pb.CascadeClient{
		Id:          uint32(instance.ID),
		Name:        instance.Name,
		CreatedTime: timestamppb.New(instance.CreatedAt),
		UpdatedTime: timestamppb.New(instance.UpdatedAt),
	}

	return
}

func (c *CascadeServerPlugin) UpdateClient(ctx context.Context, req *pb.UpdateClientRequest) (result *pb.UpdateClientResponse, err error) {
	result = &pb.UpdateClientResponse{}

	instance := &cascade.Instance{}
	if err = c.DB.First(instance, req.Id).Error; err != nil {
		return
	}

	instance.Name = req.Name
	instance.Secret = sql.NullString{String: req.Secret, Valid: req.Secret != ""}

	if err = c.DB.Save(instance).Error; err != nil {
		return
	}

	c.clients.Set(instance)

	result.Data = &pb.CascadeClient{
		Id:          uint32(instance.ID),
		Name:        instance.Name,
		UpdatedTime: timestamppb.New(instance.UpdatedAt),
	}

	return
}

func (c *CascadeServerPlugin) DeleteClient(ctx context.Context, req *pb.DeleteClientRequest) (result *pb.DeleteClientResponse, err error) {
	result = &pb.DeleteClientResponse{}

	instance := &cascade.Instance{}
	if err = c.DB.First(instance, req.Id).Error; err != nil {
		return
	}

	if err = c.DB.Delete(instance).Error; err != nil {
		return
	}

	c.clients.RemoveByKey(instance.ID)
	return
}
