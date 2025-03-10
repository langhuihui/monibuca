package plugin_gb28181pro

import (
	"fmt"

	gb28181 "m7s.live/v5/plugin/gb28181pro/pkg"
)

// SendRtpInfo 表示发送RTP流的信息
type SendRtpInfo struct {
	// 推流IP
	IP string

	// 推流端口
	Port int

	// 推流标识
	SSRC string

	// 目标平台或设备的编号
	TargetID string

	// 目标平台或设备的名称
	TargetName string

	// 是否是发送给上级平台
	SendToPlatform bool

	// 直播流的应用名
	App string

	// 通道ID
	ChannelID string

	// 推流状态，0-等待设备推流，1-等待平台回复ACK，2-推流中
	Status int

	// 设备推流的StreamID
	Stream string

	// 是否为TCP传输
	TCP bool

	// 是否为TCP主动模式
	TCPActive bool

	// 自己推流使用的IP
	LocalIP string

	// 自己推流使用的端口
	LocalPort int

	// 使用的流媒体服务ID
	MediaServerID string

	// 使用的服务的ID
	ServerID string

	// INVITE的CallID
	CallID string

	// INVITE的FromTag
	FromTag string

	// INVITE的ToTag
	ToTag string

	// 发送时，RTP的PT，默认为96
	PT int

	// 当流为回放类型时的开始时间
	StartTime int64

	// 当流为回放类型时的结束时间
	StopTime int64

	// 平台模型信息（如果是平台）
	PlatformModel *gb28181.PlatformModel

	// 通道信息（如果适用）
	Channel *gb28181.CommonGBChannel
}

// String 返回SendRtpInfo的字符串表示
func (s *SendRtpInfo) String() string {
	return fmt.Sprintf("SendRtpInfo{IP=%s, Port=%d, SSRC=%s, TargetID=%s, App=%s, Stream=%s, ChannelID=%s, Status=%d, TCP=%t, TCPActive=%t}",
		s.IP, s.Port, s.SSRC, s.TargetID, s.App, s.Stream, s.ChannelID, s.Status, s.TCP, s.TCPActive)
}

// GetKey 实现Collection接口，返回唯一标识符
func (s *SendRtpInfo) GetKey() string {
	return s.CallID
}

// GetProtocolString 获取协议字符串
func (s *SendRtpInfo) GetProtocolString() string {
	if s.TCP {
		if s.TCPActive {
			return "TCP主动"
		}
		return "TCP被动"
	}
	return "UDP"
}
