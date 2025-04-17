package plugin_webrtc

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	. "github.com/pion/webrtc/v4"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/task"
	. "m7s.live/v5/plugin/webrtc/pkg"
)

// BatchV2 通过WebSocket方式实现单PeerConnection传输多个流的功能
func (conf *WebRTCPlugin) BatchV2(w http.ResponseWriter, r *http.Request) {
	// 检查是否是WebSocket请求
	if r.Header.Get("Upgrade") != "websocket" {
		http.Error(w, "WebSocket protocol required", http.StatusBadRequest)
		return
	}

	// 升级HTTP连接为WebSocket连接
	wsConn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		conf.Error("failed to upgrade to WebSocket", "error", err)
		http.Error(w, "Failed to upgrade to WebSocket: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 创建一个WebSocket处理器
	wsHandler := &WebSocketHandler{
		conn:   wsConn,
		config: conf,
	}
	// 创建PeerConnection并设置高级配置
	if wsHandler.PeerConnection, err = conf.api.NewPeerConnection(Configuration{
		// 本地测试不需要配置 ICE 服务器
		ICETransportPolicy:   ICETransportPolicyAll,
		BundlePolicy:         BundlePolicyMaxBundle,
		RTCPMuxPolicy:        RTCPMuxPolicyRequire,
		ICECandidatePoolSize: 1,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// 添加任务
	conf.AddTask(wsHandler).WaitStopped()
}

// WebSocketHandler 处理WebSocket连接和信令
type WebSocketHandler struct {
	SingleConnection
	conn   net.Conn
	config *WebRTCPlugin
}

// Go 处理WebSocket消息
func (wsh *WebSocketHandler) Go() (err error) {
	var msg []byte
	// 等待初始SDP offer
	msg, err = wsutil.ReadClientText(wsh.conn)
	if err != nil {
		return err
	}

	// 解析初始SDP offer
	var initialSignal struct {
		Type string `json:"type"`
		SDP  string `json:"sdp"`
	}
	if err = json.Unmarshal(msg, &initialSignal); err != nil {
		return err
	}

	if initialSignal.Type != "offer" {
		return wsh.sendError("Initial message must be an SDP offer")
	}

	// 设置远程描述
	wsh.SDP = initialSignal.SDP

	// 验证SDP是否包含ICE ufrag
	if !wsh.validateSDP(initialSignal.SDP) {
		return wsh.sendError("Invalid SDP: missing ICE credentials")
	}

	// 设置远程描述
	if err = wsh.SetRemoteDescription(SessionDescription{
		Type: SDPTypeOffer,
		SDP:  initialSignal.SDP,
	}); err != nil {
		wsh.Error("Failed to set remote description", "error", err)
		return err
	}

	// 创建并发送应答
	if answer, err := wsh.GetAnswer(); err == nil {
		wsh.sendAnswer(answer.SDP)
	} else {
		return err
	}
	wsh.Info("WebSocket connection established")
	for {
		msg, err := wsutil.ReadClientText(wsh.conn)
		if err != nil {
			wsh.Error("WebSocket read error", "error", err)
			return err
		}

		var signal Signal
		if err := json.Unmarshal(msg, &signal); err != nil {
			wsh.Error("Failed to unmarshal signal", "error", err)
			wsh.sendError("Invalid signal format: " + err.Error())
			continue
		}

		wsh.Debug("Signal received", "type", signal.Type, "stream_path", signal.StreamPath)

		switch signal.Type {
		case SignalTypePublish:
			wsh.handlePublish(signal)
		case SignalTypeSubscribe:
			wsh.handleSubscribe(signal)
		case SignalTypeUnsubscribe:
			wsh.handleUnsubscribe(signal)
		case SignalTypeUnpublish:
			wsh.handleUnpublish(signal)
		case SignalTypeAnswer:
			wsh.handleAnswer(signal)
		case SignalTypeGetStreamList:
			wsh.handleGetStreamList()
		default:
			wsh.sendError("Unknown signal type: " + string(signal.Type))
		}
	}
}

// Dispose 清理资源
func (wsh *WebSocketHandler) Dispose() {
	wsh.PeerConnection.Close()
	wsh.conn.Close()
}

// sendJSON 发送JSON消息
func (wsh *WebSocketHandler) sendJSON(data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return wsutil.WriteServerText(wsh.conn, jsonData)
}

// sendAnswer 发送SDP应答
func (wsh *WebSocketHandler) sendAnswer(sdp string) error {
	return wsh.sendJSON(SignalSDP{
		Type: "answer",
		SDP:  sdp,
	})
}

// sendError 发送错误消息
func (wsh *WebSocketHandler) sendError(message string) error {
	return wsh.sendJSON(SignalError{
		Type:    "error",
		Message: message,
	})
}

// handlePublish 处理发布信号
func (wsh *WebSocketHandler) handlePublish(signal Signal) {
	if publisher, err := wsh.config.Publish(wsh.config.Context, signal.StreamPath); err == nil {
		wsh.Publisher = publisher
		wsh.Receive()

		// 重新协商SDP
		if answer, err := wsh.GetAnswer(); err == nil {
			wsh.sendAnswer(answer.SDP)
		} else {
			wsh.sendError(err.Error())
		}
	} else {
		wsh.sendError(err.Error())
	}
}

// handleSubscribe 处理订阅信号
func (wsh *WebSocketHandler) handleSubscribe(signal Signal) {
	// 验证SDP是否包含ICE ufrag
	if !wsh.validateSDP(signal.Offer) {
		wsh.sendError("Invalid SDP: missing ICE credentials")
		return
	}
	wsh.Debug("Received subscribe request", "streams", signal.StreamList)

	// 设置远程描述
	if err := wsh.SetRemoteDescription(SessionDescription{
		Type: SDPTypeOffer,
		SDP:  signal.Offer,
	}); err != nil {
		wsh.sendError("Failed to set remote description: " + err.Error())
		return
	}

	// 只添加新的订阅，不处理移除操作（移除操作由unsubscribe信号处理）
	for _, streamPath := range signal.StreamList {
		// 跳过已订阅的流
		if wsh.HasSubscriber(streamPath) {
			continue
		}
		conf := wsh.config.GetCommonConf().Subscribe
		// Disable audio as it's not needed in batchv2
		conf.SubAudio = false
		if subscriber, err := wsh.config.SubscribeWithConfig(wsh.config.Context, streamPath, conf); err == nil {
			subscriber.RemoteAddr = wsh.RemoteAddr()
			wsh.AddSubscriber(subscriber).WaitStarted()
			wsh.Info("Subscribed to new stream", "stream", streamPath)
		} else {
			wsh.sendError(err.Error())
		}
	}

	// 发送应答
	if answer, err := wsh.GetAnswer(); err == nil {
		wsh.Info("Created answer for subscribe request", "streams", signal.StreamList)

		// 记录应答SDP中的编解码器信息
		if strings.Contains(answer.SDP, "H264") {
			wsh.Debug("Answer contains H264 codec")

			// 提取profile-level-id和sprop-parameter-sets
			if strings.Contains(answer.SDP, "profile-level-id=") {
				wsh.Debug("Answer contains profile-level-id")
			}
			if strings.Contains(answer.SDP, "sprop-parameter-sets=") {
				wsh.Debug("Answer contains sprop-parameter-sets")
			}
		}

		wsh.sendAnswer(answer.SDP)
	} else {
		wsh.Error("Failed to create answer", "error", err)
		wsh.sendError("Failed to create answer: " + err.Error())
	}
}

// handleUnsubscribe 处理取消订阅信号
func (wsh *WebSocketHandler) handleUnsubscribe(signal Signal) {
	// 验证SDP是否包含ICE ufrag
	if !wsh.validateSDP(signal.Offer) {
		wsh.sendError("Invalid SDP: missing ICE credentials")
		return
	}
	wsh.Debug("Received unsubscribe request", "streams", signal.StreamList)

	// 设置远程描述
	if err := wsh.SetRemoteDescription(SessionDescription{
		Type: SDPTypeOffer,
		SDP:  signal.Offer,
	}); err != nil {
		wsh.sendError("Failed to set remote description: " + err.Error())
		return
	}

	// 移除指定的订阅
	for _, streamPath := range signal.StreamList {
		if wsh.HasSubscriber(streamPath) {
			// 获取RemoteStream对象
			if remoteStream, ok := wsh.Get(streamPath); ok {
				wsh.RemoveSubscriber(remoteStream)
				wsh.Info("Unsubscribed from stream", "stream", streamPath)
			}
		}
	}

	// 发送应答
	if answer, err := wsh.GetAnswer(); err == nil {
		wsh.Info("Created answer for unsubscribe request", "streams", signal.StreamList)
		wsh.sendAnswer(answer.SDP)
	} else {
		wsh.Error("Failed to create answer", "error", err)
		wsh.sendError("Failed to create answer: " + err.Error())
	}
}

// handleUnpublish 处理取消发布信号
func (wsh *WebSocketHandler) handleUnpublish(signal Signal) {
	if wsh.Publisher != nil && wsh.Publisher.StreamPath == signal.StreamPath {
		wsh.Publisher.Stop(task.ErrStopByUser)
		wsh.Publisher = nil

		// 重新协商SDP
		if answer, err := wsh.GetAnswer(); err == nil {
			wsh.sendAnswer(answer.SDP)
		} else {
			wsh.sendError(err.Error())
		}
	} else {
		wsh.sendError("Not publishing this stream")
	}
}

// handleAnswer 处理应答信号
func (wsh *WebSocketHandler) handleAnswer(signal Signal) {
	// 验证SDP是否包含ICE ufrag
	if !wsh.validateSDP(signal.Answer) {
		wsh.sendError("Invalid SDP: missing ICE credentials")
		return
	}

	if err := wsh.SetRemoteDescription(SessionDescription{
		Type: SDPTypeAnswer,
		SDP:  signal.Answer,
	}); err != nil {
		wsh.sendError("Failed to set remote description: " + err.Error())
	}
}

// RemoteAddr 获取远程地址
func (wsh *WebSocketHandler) RemoteAddr() string {
	if wsh.conn != nil {
		return wsh.conn.RemoteAddr().String()
	}
	return ""
}

// validateSDP 验证SDP是否包含必要的ICE凭证
func (wsh *WebSocketHandler) validateSDP(sdp string) bool {
	// 检查SDP是否为空
	if sdp == "" {
		return false
	}

	// 检查SDP是否包含ICE ufrag或pwd
	hasUfrag := strings.Contains(sdp, "a=ice-ufrag:")
	hasPwd := strings.Contains(sdp, "a=ice-pwd:")

	// 在开发环境中，我们可以放宽要求，只要有一个就可以
	return hasUfrag || hasPwd
}

// handleGetStreamList 处理获取流列表信号
func (wsh *WebSocketHandler) handleGetStreamList() {
	// 获取所有可用的流列表
	var streams []StreamInfo

	// 遍历所有流，检查是否有H.264视频编码
	for publisher := range wsh.config.Server.Streams.SafeRange {
		// 检查是否有视频轨道
		if publisher.HasVideoTrack() {
			// 获取视频编解码器上下文
			ctx := publisher.GetVideoCodecCtx()
			if ctx != nil {
				switch ctx := ctx.GetBase().(type) {
				case *codec.H264Ctx:
					// 获取视频信息
					streams = append(streams, StreamInfo{
						Path:   publisher.StreamPath,
						Codec:  "H264",
						Width:  uint32(ctx.Width()),
						Height: uint32(ctx.Height()),
						Fps:    uint32(publisher.VideoTrack.FPS),
					})
				}
			}
		}
	}

	// 发送流列表响应
	wsh.sendJSON(StreamListResponse{
		Type:    "streamList",
		Streams: streams,
	})
}
