package plugin_gb28181pro

import (
	"context"
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
	deviceId  string
	channelId string
	mutex     sync.Mutex
	broadcast *BroadcastSession
}

// NewTalkWebsocketTask 创建新的 WebSocket 对讲任务
func NewTalkWebsocketTask(plugin *GB28181Plugin, conn net.Conn, deviceId, channelId string) *TalkWebsocketTask {
	return &TalkWebsocketTask{
		conn:      conn,
		plugin:    plugin,
		deviceId:  deviceId,
		channelId: channelId,
	}
}

// GetKey 返回任务唯一键
func (t *TalkWebsocketTask) GetKey() uint32 {
	return t.Task.ID
}

// Go 运行 WebSocket 循环
func (t *TalkWebsocketTask) Go() error {
	// 按 "deviceId_channelId" 粒度加锁，保证同一通道 check+create 原子，
	// 不同通道之间互不阻塞，两台客户端对同一通道可同时发送音频。
	bsKey := t.deviceId + "_" + t.channelId
	mu := getBroadcastLock(bsKey)
	mu.Lock()
	if bs, ok := BroadcastSessions.Get(bsKey); ok {
		// 已有会话（或空闲计时器仍在跑），直接接管；两台客户端均可同时发送音频
		bs.Attach()
		t.mutex.Lock()
		t.broadcast = bs
		t.mutex.Unlock()
		mu.Unlock()
		t.Info("reusing existing broadcast session", "channelId", t.channelId)
	} else {
		// 需要新建会话：查找设备 → SIP MESSAGE → 等待 INVITE
		device, ok := t.plugin.devices.Get(t.deviceId)
		if !ok {
			mu.Unlock()
			return fmt.Errorf("device not found: %s", t.deviceId)
		}
		if !device.Online {
			mu.Unlock()
			return fmt.Errorf("device offline: %s", t.deviceId)
		}

		bs, err := device.StartBroadcast(t.channelId)
		if err != nil {
			mu.Unlock()
			return fmt.Errorf("StartBroadcast failed: %w", err)
		}
		// StartBroadcast 内部已调用 BroadcastSessions.Set(bs)，可以释放锁；
		// 后续 WaitInvite 耗时较长，不应持锁。
		mu.Unlock()

		// 等待摄像机发回 INVITE（30s 超时）
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := bs.WaitInvite(ctx); err != nil {
			_ = bs.StopBroadcast()
			return fmt.Errorf("WaitInvite failed: %w", err)
		}

		t.mutex.Lock()
		t.broadcast = bs
		t.mutex.Unlock()
		t.Info("new broadcast session started", "channelId", t.channelId)
	}

	// 纯二进制音频循环：所有帧均为 G.711 PCM
	for {
		msg, op, err := wsutil.ReadClientData(t.conn)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if op == ws.OpBinary {
			t.sendAudio(msg)
		}
		// 忽略文本帧（不再使用 JSON 命令）
	}
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

	// WS 断开 → 启动会话空闲计时器（30s 后真正停止）
	if bs != nil {
		t.Info("WebSocket disconnected, starting idle timer", "channelId", bs.Channel.ChannelId)
		bs.Detach()
	}

	if t.conn != nil {
		_ = t.conn.Close()
	}
}
