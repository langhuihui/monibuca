package webrtc

import (
	"strconv"
	"strings"

	"github.com/pion/interceptor"
	"github.com/pion/sdp/v3"
	. "github.com/pion/webrtc/v4"
)

var MimeTypes []string
var ICEServers []ICEServer

// registerLegacyCodecs 注册传统方式的编解码器（向后兼容）
func registerLegacyCodecs(m *MediaEngine) {
	// 注册基础编解码器
	if defaultCodecs, err := GetDefaultCodecs(); err != nil {
		// p.Error("Failed to get default codecs", "error", err)
	} else {
		for _, codecWithType := range defaultCodecs {
			// 检查MimeType过滤
			if isMimeTypeAllowed(codecWithType.Codec.MimeType) {
				if err := m.RegisterCodec(codecWithType.Codec, codecWithType.Type); err != nil {
					// p.Warn("Failed to register default codec", "error", err, "mimeType", codecWithType.Codec.MimeType)
				} else {
					// p.Debug("Registered default codec", "mimeType", codecWithType.Codec.MimeType, "payloadType", codecWithType.Codec.PayloadType)
				}
			} else {
				// p.Debug("Default codec filtered", "mimeType", codecWithType.Codec.MimeType)
			}
		}
	}
}

// createMediaEngine 创建新的MediaEngine实例
func createMediaEngine(ssd *sdp.SessionDescription) *MediaEngine {
	m := &MediaEngine{}

	// 如果没有提供SDP，则使用传统方式注册编解码器
	if ssd == nil {
		registerLegacyCodecs(m)
		return m
	}

	// 从SDP中解析MediaDescription并注册编解码器
	registerCodecsFromSDP(m, ssd)

	return m
}

// createAPI 创建新的API实例
func CreateAPI(ssd *sdp.SessionDescription, s SettingEngine) (api *API, err error) {
	m := createMediaEngine(ssd)
	i := &interceptor.Registry{}

	// 注册默认拦截器
	if err := RegisterDefaultInterceptors(m, i); err != nil {
		// p.Error("register default interceptors error", "error", err)
		return nil, err
	}

	// 创建API
	api = NewAPI(WithMediaEngine(m), WithInterceptorRegistry(i), WithSettingEngine(s))
	return
}

// isMimeTypeAllowed 检查MimeType是否在允许列表中
func isMimeTypeAllowed(mimeType string) bool {
	// 如果过滤列表为空，则允许所有类型
	if len(MimeTypes) == 0 {
		return true
	}

	// 检查精确匹配
	for _, filter := range MimeTypes {
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

// registerCodecsFromSDP 从SDP的MediaDescription中注册编解码器
func registerCodecsFromSDP(m *MediaEngine, ssd *sdp.SessionDescription) {
	for _, md := range ssd.MediaDescriptions {
		// 跳过非音视频媒体类型
		if md.MediaName.Media != "audio" && md.MediaName.Media != "video" {
			continue
		}

		// 解析每个format（编解码器）
		for _, format := range md.MediaName.Formats {
			codec := parseCodecFromSDP(md, format)
			if codec == nil {
				continue
			}

			// 检查MimeType过滤
			if !isMimeTypeAllowed(codec.MimeType) {
				// p.Debug("MimeType filtered", "mimeType", codec.MimeType)
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
				// p.Warn("Failed to register codec from SDP", "error", err, "mimeType", codec.MimeType)
			} else {
				// p.Debug("Registered codec from SDP", "mimeType", codec.MimeType, "payloadType", codec.PayloadType)
			}
		}
	}
}

// parseCodecFromSDP 从SDP的MediaDescription中解析单个编解码器
func parseCodecFromSDP(md *sdp.MediaDescription, format string) *RTPCodecParameters {
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
		codecName, clockRate, channels = getStaticPayloadInfo(format)
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
			RTCPFeedback: getDefaultRTCPFeedback(mimeType),
		},
		PayloadType: PayloadType(payloadType),
	}
}

// getStaticPayloadInfo 获取静态payload类型的编解码器信息
func getStaticPayloadInfo(format string) (string, uint32, uint16) {
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
func getDefaultRTCPFeedback(mimeType string) []RTCPFeedback {
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
