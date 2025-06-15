package plugin_webrtc

import (
	"embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pion/logging"
	"github.com/pion/sdp/v3"
	"m7s.live/v5/pkg/config"

	"github.com/pion/interceptor"
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

// createMediaEngine 创建新的MediaEngine实例
func (p *WebRTCPlugin) createMediaEngine(ssd *sdp.SessionDescription) *MediaEngine {
	m := &MediaEngine{}

	// 如果没有提供SDP，则使用传统方式注册编解码器
	if ssd == nil {
		p.registerLegacyCodecs(m)
		return m
	}

	// 从SDP中解析MediaDescription并注册编解码器
	p.registerCodecsFromSDP(m, ssd)

	return m
}

// registerLegacyCodecs 注册传统方式的编解码器（向后兼容）
func (p *WebRTCPlugin) registerLegacyCodecs(m *MediaEngine) {
	// 注册基础编解码器
	if defaultCodecs, err := GetDefaultCodecs(); err != nil {
		p.Error("Failed to get default codecs", "error", err)
	} else {
		for _, codecWithType := range defaultCodecs {
			// 检查MimeType过滤
			if p.isMimeTypeAllowed(codecWithType.Codec.MimeType) {
				if err := m.RegisterCodec(codecWithType.Codec, codecWithType.Type); err != nil {
					p.Warn("Failed to register default codec", "error", err, "mimeType", codecWithType.Codec.MimeType)
				} else {
					p.Debug("Registered default codec", "mimeType", codecWithType.Codec.MimeType, "payloadType", codecWithType.Codec.PayloadType)
				}
			} else {
				p.Debug("Default codec filtered", "mimeType", codecWithType.Codec.MimeType)
			}
		}
	}
}

// registerCodecsFromSDP 从SDP的MediaDescription中注册编解码器
func (p *WebRTCPlugin) registerCodecsFromSDP(m *MediaEngine, ssd *sdp.SessionDescription) {
	for _, md := range ssd.MediaDescriptions {
		// 跳过非音视频媒体类型
		if md.MediaName.Media != "audio" && md.MediaName.Media != "video" {
			continue
		}

		// 解析每个format（编解码器）
		for _, format := range md.MediaName.Formats {
			codec := p.parseCodecFromSDP(md, format)
			if codec == nil {
				continue
			}

			// 检查MimeType过滤
			if !p.isMimeTypeAllowed(codec.MimeType) {
				p.Debug("MimeType filtered", "mimeType", codec.MimeType)
				continue
			}

			// 确定编解码器类型
			var codecType RTPCodecType
			if md.MediaName.Media == "audio" {
				codecType = RTPCodecTypeAudio
			} else {
				codecType = RTPCodecTypeVideo
			}

			// 注册编解码器
			if err := m.RegisterCodec(*codec, codecType); err != nil {
				p.Warn("Failed to register codec from SDP", "error", err, "mimeType", codec.MimeType)
			} else {
				p.Debug("Registered codec from SDP", "mimeType", codec.MimeType, "payloadType", codec.PayloadType)
			}
		}
	}
}

// parseCodecFromSDP 从SDP的MediaDescription中解析单个编解码器
func (p *WebRTCPlugin) parseCodecFromSDP(md *sdp.MediaDescription, format string) *RTPCodecParameters {
	var codecName string
	var clockRate uint32
	var channels uint16
	var fmtpLine string

	// 从rtpmap属性中解析编解码器名称、时钟频率和通道数
	for _, attr := range md.Attributes {
		if attr.Key == "rtpmap" && strings.HasPrefix(attr.Value, format+" ") {
			// 格式：payloadType codecName/clockRate[/channels]
			parts := strings.Split(attr.Value, " ")
			if len(parts) >= 2 {
				codecParts := strings.Split(parts[1], "/")
				if len(codecParts) >= 2 {
					codecName = strings.ToUpper(codecParts[0])
					if rate, err := strconv.ParseUint(codecParts[1], 10, 32); err == nil {
						clockRate = uint32(rate)
					}
					if len(codecParts) >= 3 {
						if ch, err := strconv.ParseUint(codecParts[2], 10, 16); err == nil {
							channels = uint16(ch)
						}
					}
				}
			}
			break
		}
	}

	// 从fmtp属性中解析格式参数
	for _, attr := range md.Attributes {
		if attr.Key == "fmtp" && strings.HasPrefix(attr.Value, format+" ") {
			if spaceIdx := strings.Index(attr.Value, " "); spaceIdx >= 0 {
				fmtpLine = attr.Value[spaceIdx+1:]
			}
			break
		}
	}

	// 如果没有找到rtpmap，尝试静态payload类型
	if codecName == "" {
		codecName, clockRate, channels = p.getStaticPayloadInfo(format)
	}

	if codecName == "" {
		return nil
	}

	// 构造MimeType
	var mimeType string
	if md.MediaName.Media == "audio" {
		mimeType = "audio/" + codecName
	} else {
		mimeType = "video/" + codecName
	}

	// 解析PayloadType
	payloadType, err := strconv.ParseUint(format, 10, 8)
	if err != nil {
		return nil
	}

	return &RTPCodecParameters{
		RTPCodecCapability: RTPCodecCapability{
			MimeType:     mimeType,
			ClockRate:    clockRate,
			Channels:     channels,
			SDPFmtpLine:  fmtpLine,
			RTCPFeedback: p.getDefaultRTCPFeedback(mimeType),
		},
		PayloadType: PayloadType(payloadType),
	}
}

// getStaticPayloadInfo 获取静态payload类型的编解码器信息
func (p *WebRTCPlugin) getStaticPayloadInfo(format string) (string, uint32, uint16) {
	switch format {
	case "0":
		return "PCMU", 8000, 1
	case "8":
		return "PCMA", 8000, 1
	case "96":
		return "H264", 90000, 0
	case "97":
		return "H264", 90000, 0
	case "98":
		return "H264", 90000, 0
	case "111":
		return "OPUS", 48000, 2
	default:
		return "", 0, 0
	}
}

// getDefaultRTCPFeedback 获取默认的RTCP反馈
func (p *WebRTCPlugin) getDefaultRTCPFeedback(mimeType string) []RTCPFeedback {
	if strings.HasPrefix(mimeType, "video/") {
		return []RTCPFeedback{
			{Type: "goog-remb", Parameter: ""},
			{Type: "ccm", Parameter: "fir"},
			{Type: "nack", Parameter: ""},
			{Type: "nack", Parameter: "pli"},
			{Type: "transport-cc", Parameter: ""},
		}
	}
	return nil
}

// isMimeTypeAllowed 检查MimeType是否在允许列表中
func (p *WebRTCPlugin) isMimeTypeAllowed(mimeType string) bool {
	// 如果过滤列表为空，则允许所有类型
	if len(p.MimeType) == 0 {
		return true
	}

	// 检查精确匹配
	for _, filter := range p.MimeType {
		if strings.EqualFold(filter, mimeType) {
			return true
		}
		// 支持通配符匹配，如 "video/*" 或 "audio/*"
		if strings.HasSuffix(filter, "/*") {
			prefix := strings.TrimSuffix(filter, "/*")
			if strings.HasPrefix(strings.ToLower(mimeType), strings.ToLower(prefix)+"/") {
				return true
			}
		}
	}

	return false
}

// createAPI 创建新的API实例
func (p *WebRTCPlugin) createAPI(ssd *sdp.SessionDescription) (api *API, err error) {
	m := p.createMediaEngine(ssd)
	i := &interceptor.Registry{}

	// 注册默认拦截器
	if err := RegisterDefaultInterceptors(m, i); err != nil {
		p.Error("register default interceptors error", "error", err)
		return nil, err
	}

	// 创建API
	api = NewAPI(WithMediaEngine(m), WithInterceptorRegistry(i), WithSettingEngine(p.s))
	return
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

	return nil
}

func (p *WebRTCPlugin) CreatePC(sd SessionDescription, conf Configuration) (pc *PeerConnection, err error) {
	var ssd *sdp.SessionDescription
	ssd, err = sd.Unmarshal()
	if err != nil {
		return
	}
	var api *API
	// 创建PeerConnection并设置高级配置
	api, err = p.createAPI(ssd)
	if err != nil {
		return
	}
	pc, err = api.NewPeerConnection(conf)
	if err == nil {
		err = pc.SetRemoteDescription(sd)
	}
	return
}

func (p *WebRTCPlugin) OnInit() (err error) {
	if len(p.ICEServers) > 0 {
		for i := range p.ICEServers {
			b, _ := p.ICEServers[i].MarshalJSON()
			p.ICEServers[i].UnmarshalJSON(b)
		}
	}

	if err = p.initSettingEngine(); err != nil {
		return err
	}

	_, port, _ := strings.Cut(p.GetCommonConf().HTTP.ListenAddr, ":")
	if port == "80" {
		p.PushAddr = append(p.PushAddr, "http://{hostName}/webrtc/push")
		p.PlayAddr = append(p.PlayAddr, "http://{hostName}/webrtc/play")
	} else if port != "" {
		p.PushAddr = append(p.PushAddr, fmt.Sprintf("http://{hostName}:%s/webrtc/push", port))
		p.PlayAddr = append(p.PlayAddr, fmt.Sprintf("http://{hostName}:%s/webrtc/play", port))
	}
	_, port, _ = strings.Cut(p.GetCommonConf().HTTP.ListenAddrTLS, ":")
	if port == "443" {
		p.PushAddr = append(p.PushAddr, "https://{hostName}/webrtc/push")
		p.PlayAddr = append(p.PlayAddr, "https://{hostName}/webrtc/play")
	} else if port != "" {
		p.PushAddr = append(p.PushAddr, fmt.Sprintf("https://{hostName}:%s/webrtc/push", port))
		p.PlayAddr = append(p.PlayAddr, fmt.Sprintf("https://{hostName}:%s/webrtc/play", port))
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

func (p *WebRTCPlugin) Pull(streamPath string, conf config.Pull, pubConf *config.Publish) {
	if strings.HasPrefix(conf.URL, "https://rtc.live.cloudflare.com") {
		cfClient := NewCFClient(DIRECTION_PULL)
		api, err := p.createAPI(nil)
		if err != nil {
			p.Error("create API failed", "error", err)
			return
		}
		cfClient.PeerConnection, err = api.NewPeerConnection(Configuration{
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
