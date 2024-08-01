package plugin_gb28181

import (
	"fmt"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/icholy/digest"
	"github.com/pion/rtp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
	"m7s.live/m7s/v5/plugin/gb28181/pb"
	gb28181 "m7s.live/m7s/v5/plugin/gb28181/pkg"
	rtp2 "m7s.live/m7s/v5/plugin/rtp/pkg"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SipConfig struct {
	ListenAddr    []string
	ListenTLSAddr []string
	CertFile      string `desc:"证书文件"`
	KeyFile       string `desc:"私钥文件"`
}

type PositionConfig struct {
	Expires  time.Duration `default:"3600s" desc:"订阅周期"` //订阅周期
	Interval time.Duration `default:"6s" desc:"订阅间隔"`    //订阅间隔
}

type GB28181Plugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	Serial    string `default:"34020000002000000001" desc:"sip 服务 id"` //sip 服务器 id, 默认 34020000002000000001
	Realm     string `default:"3402000000" desc:"sip 服务域"`             //sip 服务器域，默认 3402000000
	Username  string
	Password  string
	Sip       SipConfig
	MediaPort util.Range[uint16] `default:"10000-20000" desc:"媒体端口范围"` //媒体端口范围
	Position  PositionConfig
	ua        *sipgo.UserAgent
	server    *sipgo.Server
	client    *sipgo.Client
	devices   util.Collection[string, *Device]
	dialogs   util.Collection[uint32, *Dialog]
	tcpPorts  chan uint16
}

var _ = m7s.InstallPlugin[GB28181Plugin](pb.RegisterApiHandler, &pb.Api_ServiceDesc)

func init() {
	sip.SIPDebug = true
}
func (gb *GB28181Plugin) OnInit() (err error) {
	logger := zerolog.New(os.Stdout)
	gb.ua, err = sipgo.NewUA(sipgo.WithUserAgent("M7S/" + m7s.Version))   // Build user agent
	gb.client, _ = sipgo.NewClient(gb.ua, sipgo.WithClientLogger(logger)) // Creating client handle for ua
	gb.server, _ = sipgo.NewServer(gb.ua, sipgo.WithServerLogger(logger)) // Creating server handle for ua
	gb.server.OnRegister(gb.OnRegister)
	gb.server.OnMessage(gb.OnMessage)
	gb.server.OnBye(gb.OnBye)
	gb.devices.L = new(sync.RWMutex)

	if gb.MediaPort.Valid() {
		gb.tcpPorts = make(chan uint16, gb.MediaPort.Size())
		for i := range gb.MediaPort.Size() {
			gb.tcpPorts <- gb.MediaPort[0] + i
		}
	} else {
		tcpConfig := &gb.GetCommonConf().TCP
		tcpConfig.ListenAddr = fmt.Sprintf(":%d", gb.MediaPort[0])
	}
	for _, addr := range gb.Sip.ListenAddr {
		netWork, addr, _ := strings.Cut(addr, ":")
		go gb.server.ListenAndServe(gb, netWork, addr)
	}
	if len(gb.Sip.ListenTLSAddr) > 0 {
		if tslConfig, err := config.GetTLSConfig(gb.Sip.CertFile, gb.Sip.KeyFile); err == nil {
			for _, addr := range gb.Sip.ListenTLSAddr {
				netWork, addr, _ := strings.Cut(addr, ":")
				go gb.server.ListenAndServeTLS(gb, netWork, addr, tslConfig)
			}
		} else {
			return err
		}
	}
	return
}

func (gb *GB28181Plugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/api/ps/replay/{streamPath...}": gb.api_ps_replay,
	}
}

func (gb *GB28181Plugin) OnRegister(req *sip.Request, tx sip.ServerTransaction) {
	from := req.From()
	if from == nil || from.Address.User == "" {
		gb.Error("OnRegister", "error", "no user")
		return
	}
	isUnregister := false
	id := from.Address.User
	exp := req.GetHeader("Expires")
	if exp == nil {
		gb.Error("OnRegister", "error", "no expires")
		return
	}
	expSec, err := strconv.ParseInt(exp.Value(), 10, 32)
	if err != nil {
		gb.Error("OnRegister", "error", err.Error())
		return
	}
	if expSec == 0 {
		isUnregister = true
	}
	// 不需要密码情况
	if gb.Username != "" && gb.Password != "" {
		h := req.GetHeader("Authorization")
		var chal digest.Challenge
		var cred *digest.Credentials
		var digCred *digest.Credentials
		if h == nil {
			chal = digest.Challenge{
				Realm:     gb.Realm,
				Nonce:     fmt.Sprintf("%d", time.Now().UnixMicro()),
				Opaque:    "monibuca",
				Algorithm: "MD5",
			}

			res := sip.NewResponseFromRequest(req, sip.StatusUnauthorized, "Unathorized", nil)
			res.AppendHeader(sip.NewHeader("WWW-Authenticate", chal.String()))

			err = tx.Respond(res)
			return
		}

		cred, err = digest.ParseCredentials(h.Value())
		if err != nil {
			log.Error().Err(err).Msg("parsing creds failed")
			err = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusUnauthorized, "Bad credentials", nil))
			return
		}

		// Check registry
		if cred.Username != gb.Username {
			err = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Bad authorization header", nil))
			return
		}

		// Make digest and compare response
		digCred, err = digest.Digest(&chal, digest.Options{
			Method:   "REGISTER",
			URI:      cred.URI,
			Username: gb.Username,
			Password: gb.Password,
		})

		if err != nil {
			gb.Error("Calc digest failed")
			err = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusUnauthorized, "Bad credentials", nil))
			return
		}

		if cred.Response != digCred.Response {
			err = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusUnauthorized, "Unathorized", nil))
			return
		}
	}
	err = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusOK, "OK", nil))
	if isUnregister {
		if d, ok := gb.devices.Get(id); ok {
			close(d.eventChan)
		}
	} else {
		if d, ok := gb.devices.Get(id); ok {
			gb.RecoverDevice(d, req)
		} else {
			d = gb.StoreDevice(id, req)
		}
	}
}

func (gb *GB28181Plugin) OnMessage(req *sip.Request, tx sip.ServerTransaction) {
	from := req.From()
	if from == nil || from.Address.User == "" {
		gb.Error("OnMessage", "error", "no user")
		return
	}
	id := from.Address.User
	if d, ok := gb.devices.Get(id); ok {
		d.UpdateTime = time.Now()
		temp := &gb28181.Message{}
		err := gb28181.DecodeXML(temp, req.Body())
		if err != nil {
			gb.Error("OnMessage", "error", err.Error())
			return
		}
		err = d.onMessage(req, tx, temp)
	} else {
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusNotFound, "Not Found", nil))
	}
}

func (gb *GB28181Plugin) RecoverDevice(d *Device, req *sip.Request) {
	from := req.From()
	source := req.Source()
	hostname := strings.Split(source, ":")
	port, _ := strconv.Atoi(hostname[1])
	d.Recipient = sip.Uri{
		Host: hostname[0],
		Port: port,
		User: from.Address.User,
	}
	d.RegisterTime = time.Now()
	d.Status = DeviceRecoverStatus
	d.UpdateTime = time.Now()
}

func (gb *GB28181Plugin) StoreDevice(id string, req *sip.Request) (d *Device) {
	from := req.From()
	source := req.Source()
	desc := req.Destination()
	serverHostName := strings.Split(desc, ":")
	servIp := serverHostName[0]
	publicIP := gb.GetPublicIP(servIp)
	//如果相等，则服务器是内网通道.海康摄像头不支持...自动获取
	//if strings.LastIndex(deviceIp, ".") != -1 && strings.LastIndex(servIp, ".") != -1 {
	//	if servIp[0:strings.LastIndex(servIp, ".")] == deviceIp[0:strings.LastIndex(deviceIp, ".")] || mediaIP == "" {
	//		mediaIP = servIp
	//	}
	//}
	hostname := strings.Split(source, ":")
	port, _ := strconv.Atoi(hostname[1])
	serverPort, _ := strconv.Atoi(serverHostName[1])
	d = &Device{
		ID:           id,
		RegisterTime: time.Now(),
		UpdateTime:   time.Now(),
		Status:       DeviceRegisterStatus,
		Recipient: sip.Uri{
			Host: hostname[0],
			Port: port,
			User: from.Address.User,
		},
		Transport: req.Transport(),
		Logger:    gb.Logger.With("id", id),
		mediaIp:   publicIP,
		eventChan: make(chan any, 10),
		contactHDR: sip.ContactHeader{
			Address: sip.Uri{
				User: gb.Serial,
				Host: publicIP,
				Port: serverPort,
			},
		},
		fromHDR: sip.FromHeader{
			Address: sip.Uri{
				User: gb.Serial,
				Host: gb.Realm,
			},
			Params: sip.NewParams(),
		},
	}
	d.fromHDR.Params.Add("tag", sip.GenerateTagN(16))
	d.dialogClient = sipgo.NewDialogClient(gb.client, d.contactHDR)
	d.channels.L = new(sync.RWMutex)
	d.Info("StoreDevice", "source", source, "desc", desc, "servIp", servIp, "publicIP", publicIP, "recipient", req.Recipient)
	gb.devices.Add(d)
	if gb.DB != nil {
		//TODO
	}
	go d.eventLoop(gb)
	return
}

func (gb *GB28181Plugin) NewPullHandler() m7s.PullHandler {
	return &Dialog{
		gb: gb,
	}
}

func (gb *GB28181Plugin) GetPullableList() []string {
	return slices.Collect(func(yield func(string) bool) {
		for d := range gb.devices.Range {
			for c := range d.channels.Range {
				yield(fmt.Sprintf("%s/%s", d.ID, c.DeviceID))
			}
		}
	})
}

func (gb *GB28181Plugin) OnTCPConnect(conn *net.TCPConn) {
	var reader = (*rtp2.TCP)(conn)
	var theDialog *Dialog
	_ = reader.Read(func(data util.Buffer) (err error) {
		if theDialog != nil {
			return theDialog.ReadRTP(data)
		}
		var rtpPacket rtp.Packet
		if err = rtpPacket.Unmarshal(data); err != nil {
			gb.Error("decode rtp", "err", err)
		}
		ssrc := rtpPacket.SSRC
		if dialog, ok := gb.dialogs.Get(ssrc); ok {
			theDialog = dialog
			return dialog.ReadRTP(data)
		}
		gb.Warn("dialog not found", "ssrc", ssrc)
		return
	})
	if theDialog != nil {
		close(theDialog.FeedChan)
	}
}

func (gb *GB28181Plugin) OnBye(req *sip.Request, tx sip.ServerTransaction) {
	if dialog, ok := gb.dialogs.Find(func(d *Dialog) bool {
		return d.GetCallID() == req.CallID().Value()
	}); ok {
		gb.Warn("OnBye", "dialog", dialog.GetCallID())
		dialog.Bye()
	}
}
