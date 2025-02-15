package gb28181

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/emiago/sipgo/sip"
)

// DecodeSDP 从 SIP 请求中解析 SDP 信息
func DecodeSDP(req *sip.Request) (*InviteInfo, error) {
	inviteInfo := NewInviteInfo()

	// 获取请求者ID
	from := req.From()
	if from == nil || from.Address.User == "" {
		return nil, fmt.Errorf("无法从请求中获取来源id")
	}
	inviteInfo.RequesterId = from.Address.User

	// 获取目标通道ID
	channelIDArray := getChannelIDFromRequest(req)
	if channelIDArray == nil {
		return nil, fmt.Errorf("无法从请求中获取通道id")
	}
	inviteInfo.TargetChannelId = channelIDArray[0]
	if len(channelIDArray) == 2 {
		inviteInfo.SourceChannelId = channelIDArray[1]
	}

	// 获取CallID
	callID := req.CallID()
	if callID != nil {
		inviteInfo.CallId = callID.Value()
	}

	// 解析SDP消息
	sdpStr := string(req.Body())
	if sdpStr == "" {
		return nil, fmt.Errorf("SDP内容为空")
	}

	// 解析SDP各个字段
	lines := strings.Split(sdpStr, "\r\n")
	var mediaDesc []string

	for _, line := range lines {
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "s="):
			inviteInfo.SessionName = strings.TrimPrefix(line, "s=")

		case strings.HasPrefix(line, "c="):
			// c=IN IP4 192.168.1.100
			parts := strings.Split(line, " ")
			if len(parts) >= 3 {
				inviteInfo.IP = parts[2]
			}

		case strings.HasPrefix(line, "t="):
			// t=开始时间 结束时间
			parts := strings.Split(strings.TrimPrefix(line, "t="), " ")
			if len(parts) >= 2 {
				startTime, err := strconv.ParseInt(parts[0], 10, 64)
				if err == nil {
					inviteInfo.StartTime = startTime
				}
				stopTime, err := strconv.ParseInt(parts[1], 10, 64)
				if err == nil {
					inviteInfo.StopTime = stopTime
				}
			}

		case strings.HasPrefix(line, "m="):
			mediaDesc = strings.Split(strings.TrimPrefix(line, "m="), " ")
			if len(mediaDesc) >= 3 {
				port, err := strconv.Atoi(mediaDesc[1])
				if err == nil {
					inviteInfo.Port = port
				}
				// 检查传输协议
				if strings.Contains(mediaDesc[2], "TCP") {
					inviteInfo.TCP = true
				}
			}

		case strings.HasPrefix(line, "a=setup:"):
			if strings.Contains(line, "active") {
				inviteInfo.TCPActive = true
			} else if strings.Contains(line, "passive") {
				inviteInfo.TCPActive = false
			}

		case strings.HasPrefix(line, "y="):
			inviteInfo.SSRC = strings.TrimPrefix(line, "y=")

		case strings.HasPrefix(line, "a=downloadspeed:"):
			inviteInfo.DownloadSpeed = strings.TrimPrefix(line, "a=downloadspeed:")
		}
	}

	return inviteInfo, nil
}

// getChannelIDFromRequest 从请求中获取通道ID
func getChannelIDFromRequest(req *sip.Request) []string {
	to := req.To()
	if to == nil {
		return nil
	}

	channelID := to.Address.User
	if channelID == "" {
		return nil
	}

	// 检查是否包含源通道ID
	if strings.Contains(channelID, "@") {
		return strings.Split(channelID, "@")
	}

	return []string{channelID}
}
