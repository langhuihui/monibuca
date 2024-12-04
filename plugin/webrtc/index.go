package plugin_webrtc

import (
	"embed"
	_ "embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/pion/logging"
	"m7s.live/v5/pkg/config"

	"github.com/pion/interceptor"
	. "github.com/pion/webrtc/v3"
	"m7s.live/v5"
	. "m7s.live/v5/pkg"
	. "m7s.live/v5/plugin/webrtc/pkg"
)

var (
	//go:embed web
	web       embed.FS
	reg_level = regexp.MustCompile("profile-level-id=(4.+f)")
	_         = m7s.InstallPlugin[WebRTCPlugin](NewPuller, NewPusher)
)

type WebRTCPlugin struct {
	m7s.Plugin
	ICEServers []ICEServer   `desc:"ice服务器配置"`
	Port       string        `default:"tcp:9000" desc:"监听端口"`
	PLI        time.Duration `default:"2s" desc:"发送PLI请求间隔"`         // 视频流丢包后，发送PLI请求
	EnableOpus bool          `default:"true" desc:"是否启用opus编码"`      // 是否启用opus编码
	EnableVP9  bool          `default:"false" desc:"是否启用vp9编码"`      // 是否启用vp9编码
	EnableAv1  bool          `default:"false" desc:"是否启用av1编码"`      // 是否启用av1编码
	EnableDC   bool          `default:"true" desc:"是否启用DataChannel"` // 在不支持编码格式的情况下是否启用DataChannel传输
	m          MediaEngine
	s          SettingEngine
	api        *API
}

func (p *WebRTCPlugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/test/{name}": p.testPage,
	}
}

func (p *WebRTCPlugin) NewLogger(scope string) logging.LeveledLogger {
	return &LoggerTransform{Logger: p.Logger.With("scope", scope)}
}

func (p *WebRTCPlugin) OnInit() (err error) {
	if len(p.ICEServers) > 0 {
		for i := range p.ICEServers {
			b, _ := p.ICEServers[i].MarshalJSON()
			p.ICEServers[i].UnmarshalJSON(b)
		}
	}
	p.s.LoggerFactory = p
	RegisterCodecs(&p.m)
	if p.EnableOpus {
		p.m.RegisterCodec(RTPCodecParameters{
			RTPCodecCapability: RTPCodecCapability{MimeTypeOpus, 48000, 2, "minptime=10;useinbandfec=1", nil},
			PayloadType:        111,
		}, RTPCodecTypeAudio)
	}
	if p.EnableVP9 {
		p.m.RegisterCodec(RTPCodecParameters{
			RTPCodecCapability: RTPCodecCapability{MimeTypeVP9, 90000, 0, "", nil},
			PayloadType:        100,
		}, RTPCodecTypeVideo)
	}
	if p.EnableAv1 {
		p.m.RegisterCodec(RTPCodecParameters{
			RTPCodecCapability: RTPCodecCapability{MimeTypeAV1, 90000, 0, "profile=2;level-idx=8;tier=1", nil},
			PayloadType:        45,
		}, RTPCodecTypeVideo)
	}
	i := &interceptor.Registry{}
	if p.GetCommonConf().PublicIP != "" {
		ips := []string{p.GetCommonConf().PublicIP}
		if p.GetCommonConf().PublicIPv6 != "" {
			ips = append(ips, p.GetCommonConf().PublicIPv6)
		}
		p.s.SetNAT1To1IPs(ips, ICECandidateTypeHost)
	}
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
		p.OnDispose(func() {
			_ = tcpl.Close()
		})
		if err != nil {
			p.Error("webrtc listener tcp", "error", err)
		}
		p.SetDescription("tcp", fmt.Sprintf("%d", tcpport))
		p.Info("webrtc start listen", "port", tcpport)
		p.s.SetICETCPMux(NewICETCPMux(nil, tcpl, 4096))
		p.s.SetNetworkTypes([]NetworkType{NetworkTypeTCP4, NetworkTypeTCP6})
	case UDPRangePort:
		p.s.SetEphemeralUDPPortRange(uint16(v[0]), uint16(v[1]))
		p.SetDescription("udp", fmt.Sprintf("%d-%d", v[0], v[1]))
	case UDPPort:
		// 创建共享WEBRTC端口 默认9000
		udpListener, err := net.ListenUDP("udp", &net.UDPAddr{
			IP:   net.IP{0, 0, 0, 0},
			Port: int(v),
		})
		p.OnDispose(func() {
			_ = udpListener.Close()
		})
		if err != nil {
			p.Error("webrtc listener udp", "error", err)
			return err
		}
		p.SetDescription("udp", fmt.Sprintf("%d", v))
		p.Info("webrtc start listen", "port", v)
		p.s.SetICEUDPMux(NewICEUDPMux(nil, udpListener))
		p.s.SetNetworkTypes([]NetworkType{NetworkTypeUDP4, NetworkTypeUDP6})
	}
	if err = RegisterDefaultInterceptors(&p.m, i); err != nil {
		return err
	}
	p.api = NewAPI(WithMediaEngine(&p.m),
		WithInterceptorRegistry(i), WithSettingEngine(p.s))
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
	}
	f, err := web.Open(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	io.Copy(w, f)
}

func (p *WebRTCPlugin) Pull(streamPath string, conf config.Pull, pubConf *config.Publish) {
	if strings.HasPrefix(conf.URL, "https://rtc.live.cloudflare.com") {
		cfClient := NewCFClient(DIRECTION_PULL)
		var err error
		cfClient.PeerConnection, err = p.api.NewPeerConnection(Configuration{
			ICEServers:   p.ICEServers,
			BundlePolicy: BundlePolicyMaxBundle,
		})
		if err != nil {
			p.Error("pull", "error", err)
			return
		}
		cfClient.GetPullJob().Init(cfClient, &p.Plugin, streamPath, conf, pubConf)
	}
}
