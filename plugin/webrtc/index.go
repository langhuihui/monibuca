package plugin_webrtc

import (
	"embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/pion/logging"
	"github.com/pion/sdp/v3"

	. "github.com/pion/webrtc/v4"
	"m7s.live/v5"
	. "m7s.live/v5/pkg"
	. "m7s.live/v5/plugin/webrtc/pkg"
)

var (
	//go:embed web
	web embed.FS
	//go:embed default.yaml
	defaultYaml m7s.DefaultYaml
	reg_level   = regexp.MustCompile("profile-level-id=(4.+f)")
	_           = m7s.InstallPlugin[WebRTCPlugin](m7s.PluginMeta{
		DefaultYaml: defaultYaml,
		NewPuller:   NewPuller,
		NewPusher:   NewPusher,
	})
)

type WebRTCPlugin struct {
	m7s.Plugin
	ICEServers []ICEServer   `desc:"ice服务器配置"`
	Port       string        `default:"tcp:9000" desc:"监听端口"`
	PLI        time.Duration `default:"2s" desc:"发送PLI请求间隔"`         // 视频流丢包后，发送PLI请求
	EnableDC   bool          `default:"true" desc:"是否启用DataChannel"` // 在不支持编码格式的情况下是否启用DataChannel传输
	MimeType   []string      `desc:"MimeType过滤列表，为空则不过滤"`            // MimeType过滤列表，支持的格式如：video/H264, audio/opus
	s          SettingEngine
}

func (p *WebRTCPlugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/test/{name}":          p.testPage,
		"/push/{streamPath...}": p.servePush,
		"/play/{streamPath...}": p.servePlay,
	}
}

func (p *WebRTCPlugin) NewLogger(scope string) logging.LeveledLogger {
	return &LoggerTransform{Logger: p.Logger.With("scope", scope)}
}

// initSettingEngine 初始化SettingEngine
func (p *WebRTCPlugin) initSettingEngine() error {
	// 设置LoggerFactory
	p.s.LoggerFactory = p

	// 配置NAT 1:1 IP映射
	if p.GetCommonConf().PublicIP != "" {
		ips := []string{p.GetCommonConf().PublicIP}
		if p.GetCommonConf().PublicIPv6 != "" {
			ips = append(ips, p.GetCommonConf().PublicIPv6)
		}
		p.s.SetNAT1To1IPs(ips, ICECandidateTypeHost)
	}

	// 配置端口
	if err := p.configurePort(); err != nil {
		return err
	}

	return nil
}

// configurePort 配置端口设置
func (p *WebRTCPlugin) configurePort() error {
	ports, err := ParsePort2(p.Port)
	if err != nil {
		p.Error("webrtc port config error", "error", err, "port", p.Port)
		return err
	}

	switch v := ports.(type) {
	case TCPPort:
		tcpport := int(v)
		tcpl, err := net.ListenTCP("tcp", &net.TCPAddr{
			IP:   net.IP{0, 0, 0, 0},
			Port: tcpport,
		})
		if err != nil {
			p.Error("webrtc listener tcp", "error", err)
		}
		p.Using(tcpl)
		p.SetDescription("tcp", fmt.Sprintf("%d", tcpport))
		p.Info("webrtc start listen", "port", tcpport)
		p.s.SetICETCPMux(NewICETCPMux(nil, tcpl, 4096))
		p.s.SetNetworkTypes([]NetworkType{NetworkTypeTCP4, NetworkTypeTCP6})
		p.s.DisableSRTPReplayProtection(true)
	case UDPRangePort:
		p.s.SetEphemeralUDPPortRange(uint16(v[0]), uint16(v[1]))
		p.SetDescription("udp", fmt.Sprintf("%d-%d", v[0], v[1]))
	case UDPPort:
		// 创建共享WEBRTC端口 默认9000
		udpListener, err := net.ListenUDP("udp", &net.UDPAddr{
			IP:   net.IP{0, 0, 0, 0},
			Port: int(v),
		})
		if err != nil {
			p.Error("webrtc listener udp", "error", err)
			return err
		}
		p.Using(udpListener)
		p.SetDescription("udp", fmt.Sprintf("%d", v))
		p.Info("webrtc start listen", "port", v)
		p.s.SetICEUDPMux(NewICEUDPMux(nil, udpListener))
		p.s.SetNetworkTypes([]NetworkType{NetworkTypeUDP4, NetworkTypeUDP6})
	}

	return nil
}

func (p *WebRTCPlugin) CreatePC(sd SessionDescription, conf Configuration) (pc *PeerConnection, err error) {
	var ssd *sdp.SessionDescription
	ssd, err = sd.Unmarshal()
	if err != nil {
		return
	}
	var api *API
	api, err = CreateAPI(ssd, p.s)
	if err != nil {
		return
	}
	pc, err = api.NewPeerConnection(conf)
	if err == nil {
		err = pc.SetRemoteDescription(sd)
	}
	return
}

func (p *WebRTCPlugin) Start() (err error) {
	if len(p.ICEServers) > 0 {
		for i := range p.ICEServers {
			b, _ := p.ICEServers[i].MarshalJSON()
			p.ICEServers[i].UnmarshalJSON(b)
		}
	}
	MimeTypes = p.MimeType
	if err = p.initSettingEngine(); err != nil {
		return err
	}

	_, port, _ := strings.Cut(p.GetCommonConf().HTTP.ListenAddr, ":")
	if port == "80" {
		p.PushAddr = append(p.PushAddr, "http://{hostName}/webrtc/push/{streamPath}")
		p.PlayAddr = append(p.PlayAddr, "http://{hostName}/webrtc/play/{streamPath}")
	} else if port != "" {
		p.PushAddr = append(p.PushAddr, fmt.Sprintf("http://{hostName}:%s/webrtc/push/{streamPath}", port))
		p.PlayAddr = append(p.PlayAddr, fmt.Sprintf("http://{hostName}:%s/webrtc/play/{streamPath}", port))
	}
	_, port, _ = strings.Cut(p.GetCommonConf().HTTP.ListenAddrTLS, ":")
	if port == "443" {
		p.PushAddr = append(p.PushAddr, "https://{hostName}/webrtc/push/{streamPath}")
		p.PlayAddr = append(p.PlayAddr, "https://{hostName}/webrtc/play/{streamPath}")
	} else if port != "" {
		p.PushAddr = append(p.PushAddr, fmt.Sprintf("https://{hostName}:%s/webrtc/push/{streamPath}", port))
		p.PlayAddr = append(p.PlayAddr, fmt.Sprintf("https://{hostName}:%s/webrtc/play/{streamPath}", port))
	}

	return
}

func (p *WebRTCPlugin) testPage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	switch name {
	case "publish", "screenshare":
		name = "web/publish.html"
	case "subscribe":
		name = "web/subscribe.html"
	case "push":
		name = "web/push.html"
	case "pull":
		name = "web/pull.html"
	case "batchv2":
		name = "web/batchv2.html"
	default:
		name = "web/" + name
	}
	// Set appropriate MIME type based on file extension
	if strings.HasSuffix(name, ".html") {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	} else if strings.HasSuffix(name, ".js") {
		w.Header().Set("Content-Type", "application/javascript")
		// } else if strings.HasSuffix(name, ".css") {
		// 	w.Header().Set("Content-Type", "text/css")
		// } else if strings.HasSuffix(name, ".json") {
		// 	w.Header().Set("Content-Type", "application/json")
		// } else if strings.HasSuffix(name, ".png") {
		// 	w.Header().Set("Content-Type", "image/png")
		// } else if strings.HasSuffix(name, ".jpg") || strings.HasSuffix(name, ".jpeg") {
		// 	w.Header().Set("Content-Type", "image/jpeg")
		// } else if strings.HasSuffix(name, ".svg") {
		// 	w.Header().Set("Content-Type", "image/svg+xml")
	}
	f, err := web.Open(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	io.Copy(w, f)
}
