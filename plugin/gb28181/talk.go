package plugin_gb28181pro

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	task "github.com/langhuihui/gotask"
)

// TalkWebsocketTask 负责管理一次 WebSocket 对讲/广播会话
type TalkWebsocketTask struct {
	task.Task
	conn      net.Conn
	plugin    *GB28181Plugin
	mutex     sync.Mutex
	broadcast *BroadcastSession
}

// NewTalkWebsocketTask 创建新的 WebSocket 对讲任务
func NewTalkWebsocketTask(plugin *GB28181Plugin, conn net.Conn) *TalkWebsocketTask {
	return &TalkWebsocketTask{
		conn:   conn,
		plugin: plugin,
	}
}

// GetKey 返回任务唯一键
func (t *TalkWebsocketTask) GetKey() uint32 {
	return t.Task.ID
}

// Go 运行 WebSocket 循环
func (t *TalkWebsocketTask) Go() error {
	for {
		msg, op, err := wsutil.ReadClientData(t.conn)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Error("WebSocket read error", "error", err)
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		switch op {
		case ws.OpText:
			if err := t.handleCommand(msg); err != nil {
				t.Error("Failed to handle command", "error", err)
			}
		case ws.OpBinary:
			t.sendAudio(msg)
		}
	}
}

func (t *TalkWebsocketTask) handleCommand(msg []byte) error {
	var command map[string]interface{}
	if err := json.Unmarshal(msg, &command); err != nil {
		return fmt.Errorf("parse JSON command: %w", err)
	}

	cmdType, ok := command["type"].(string)
	if !ok {
		return fmt.Errorf("invalid command format: missing type field")
	}

	switch cmdType {
	case "startBroadcast":
		deviceID, _ := command["deviceId"].(string)
		channelID, _ := command["channelId"].(string)

		if deviceID == "" || channelID == "" {
			return fmt.Errorf("missing deviceId or channelId for startBroadcast command")
		}

		// 确保设备存在
		if _, ok := t.plugin.devices.Get(deviceID); !ok {
			return fmt.Errorf("device not found: %s", deviceID)
		}

		// 绑定到对应的 BroadcastSession
		bs, ok := BroadcastSessions.Get(channelID)
		if !ok {
			return fmt.Errorf("broadcast session not found for channel %s", channelID)
		}

		t.mutex.Lock()
		t.broadcast = bs
		t.mutex.Unlock()

		bindTime := time.Now()
		t.plugin.Info("WebSocket bound to broadcast session",
			"channelId", channelID,
			"ready", bs.ready,
			"queuedPackets", len(bs.audioChan),
			"time", bindTime.Format("15:04:05.000"))

		// 等待设备准备好 RTP 接收端口
		// GB28181 设备通常需要 100-500ms 来打开 RTP 端口
		time.Sleep(200 * time.Millisecond)

	case "stopBroadcast":
		// 仅解绑当前 WebSocket 与广播会话，真正的 BYE 由其它控制接口触发
		t.mutex.Lock()
		t.broadcast = nil
		t.mutex.Unlock()

		t.Info("WebSocket unbound from broadcast session")
	}

	return nil
}

func (t *TalkWebsocketTask) sendAudio(payload []byte) {
	t.mutex.Lock()
	bs := t.broadcast
	t.mutex.Unlock()

	if bs != nil {
		if err := bs.SendAudioData(payload); err != nil {
			t.Error("Failed to send broadcast audio data", "error", err)
		}
	}
}

// Dispose 清理资源
func (t *TalkWebsocketTask) Dispose() {
	t.mutex.Lock()
	bs := t.broadcast
	t.broadcast = nil
	t.mutex.Unlock()

	// 如果绑定了广播会话，停止它
	// 这确保 WebSocket 断开时（如用户关闭浏览器）资源被正确清理
	if bs != nil {
		t.Info("WebSocket disconnected, stopping broadcast session",
			"channelId", bs.Channel.ChannelId)
		bs.StopBroadcast()
	}

	if t.conn != nil {
		_ = t.conn.Close()
	}
}
