package pkg

import (
	"context"
	"log/slog"
	"math"
	"reflect"
	"time"

	"github.com/langhuihui/gomem"
	task "github.com/langhuihui/gotask"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/config"

	"m7s.live/v5/pkg/util"
)

const threshold = 10 * time.Millisecond
const DROP_FRAME_LEVEL_NODROP = 0
const DROP_FRAME_LEVEL_DROP_P = 1
const DROP_FRAME_LEVEL_DROP_ALL = 2

type (
	Track struct {
		*slog.Logger
		ready       *util.Promise
		FrameType   reflect.Type
		bytesIn     int
		frameCount  int
		lastBPSTime time.Time
		BPS         int
		FPS         int
	}

	DataTrack struct {
		Track
	}
	TsTamer struct {
		BaseTs, LastTs, BeforeScaleChangedTs time.Duration
		LastScale                            float64
	}
	SpeedController struct {
		speed           float64
		pausedTime      time.Duration
		beginTime       time.Time
		beginTimestamp  time.Duration // 记录开始播放时的第一个时间戳
		Delta           time.Duration
		speedFrameCount int64         // 倍速控制帧计数，用于性能监控
		lastAdjustTime  time.Time     // 上次调整时间
		lastTimestamp   time.Duration // 上一个时间戳，用于检测重置
	}
	DropController struct {
		acceptFrameCount    int
		accpetFPS           int
		LastDropLevelChange time.Time
		DropFrameLevel      int // 0: no drop, 1: drop P-frame, 2: drop all
	}
	AVTrack struct {
		Track
		*RingWriter
		codec.ICodecCtx
		Allocator *gomem.ScalableMemoryAllocator
		WrapIndex int
		TsTamer
		SpeedController
		DropController
	}
)

func NewAVTrack(args ...any) (t *AVTrack) {
	t = &AVTrack{}
	for _, arg := range args {
		switch v := arg.(type) {
		case IAVFrame:
			t.FrameType = reflect.TypeOf(v)
			sample := v.GetSample()
			t.Allocator = sample.GetAllocator()
			t.ICodecCtx = sample.ICodecCtx
		case reflect.Type:
			t.FrameType = v
		case *slog.Logger:
			t.Logger = v.With("frameType", t.FrameType.String())
		case *AVTrack:
			t.Logger = v.Logger.With("subtrack", t.FrameType.String())
			t.RingWriter = v.RingWriter
			t.ready = util.NewPromiseWithTimeout(context.TODO(), time.Second*5)
		case *config.Publish:
			t.RingWriter = NewRingWriter(v.RingSize)
			t.BufferRange[0] = v.BufferTime
			t.RingWriter.SLogger = t.Logger
		case *util.Promise:
			t.ready = v
		}
	}
	//t.ready = util.NewPromise(struct{}{})
	t.Info("create", "dropFrameLevel", t.DropFrameLevel)
	return
}

func (t *Track) GetKey() reflect.Type {
	return t.FrameType
}

func (t *Track) AddBytesIn(n int) {
	t.bytesIn += n
	t.frameCount++
	if dur := time.Since(t.lastBPSTime); dur > time.Second {
		t.BPS = int(float64(t.bytesIn) / dur.Seconds())
		t.bytesIn = 0
		t.FPS = int(float64(t.frameCount) / dur.Seconds())
		t.frameCount = 0
		t.lastBPSTime = time.Now()
	}
}

func (t *AVTrack) AddBytesIn(n int) {
	dur := time.Since(t.lastBPSTime)
	t.Track.AddBytesIn(n)
	if t.frameCount == 0 {
		t.accpetFPS = int(float64(t.acceptFrameCount) / dur.Seconds())
		t.acceptFrameCount = 0
	}
}

func (t *AVTrack) FixTimestamp(data *Sample, scale float64) {
	t.AddBytesIn(data.Size)
	data.Timestamp = t.Tame(data.Timestamp, t.FPS, scale)
}

func (t *AVTrack) NewFrame(avFrame *AVFrame) (frame IAVFrame) {
	frame = reflect.New(t.FrameType.Elem()).Interface().(IAVFrame)
	if avFrame.Sample == nil {
		avFrame.Sample = frame.GetSample()
	}
	if avFrame.BaseSample == nil {
		avFrame.BaseSample = &BaseSample{}
	}
	frame.GetSample().BaseSample = avFrame.BaseSample
	return
}

func (t *AVTrack) AcceptFrame() {
	t.acceptFrameCount++
}

func (t *AVTrack) changeDropFrameLevel(newLevel int) {
	t.Warn("change drop frame level", "from", t.DropFrameLevel, "to", newLevel)
	t.DropFrameLevel = newLevel
	t.LastDropLevelChange = time.Now()
}

func (t *AVTrack) CheckIfNeedDropFrame(maxFPS int, speed float64) (drop bool) {
	drop = maxFPS > 0 && (t.accpetFPS > maxFPS)

	// 根据倍速调整丢帧策略，避免过度丢帧导致播放不均匀
	if speed > 2 && speed <= 4 {
		// 4倍速：非常保守，只在极端情况下才丢帧
		drop = drop && (t.accpetFPS > maxFPS*3)
	} else if speed > 4 && speed <= 8 {
		// 5-8倍速：保守策略
		drop = drop && (t.accpetFPS > maxFPS*2)
	} else if speed > 16 {
		// 极高倍速：激进策略
		drop = drop || (t.accpetFPS > maxFPS/2)
	}
	// 正常倍速(<=2倍)和慢放(speed<1)保持原有逻辑

	if drop {
		defer func() {
			if time.Since(t.LastDropLevelChange) > time.Second && t.DropFrameLevel < DROP_FRAME_LEVEL_DROP_ALL {
				t.changeDropFrameLevel(t.DropFrameLevel + 1)
			}
		}()
	}

	switch t.DropFrameLevel {
	case DROP_FRAME_LEVEL_NODROP:
		if drop {
			t.changeDropFrameLevel(DROP_FRAME_LEVEL_DROP_P)
		}
	case DROP_FRAME_LEVEL_DROP_P: // Drop P-frame
		if !t.Value.IDR {
			return true
		} else if !drop {
			t.changeDropFrameLevel(DROP_FRAME_LEVEL_NODROP)
		}
		return false
	default:
		if !drop {
			t.changeDropFrameLevel(DROP_FRAME_LEVEL_DROP_P)
		} else {
			return true
		}
	}
	return
}

func (t *AVTrack) Ready(err error) {
	if t.ready.IsPending() {
		if err != nil {
			t.Error("ready", "err", err)
		} else {
			switch ctx := t.ICodecCtx.(type) {
			case IVideoCodecCtx:
				t.Info("ready", "codec", t.ICodecCtx.FourCC(), "info", t.ICodecCtx.GetInfo(), "width", ctx.Width(), "height", ctx.Height())
			case IAudioCodecCtx:
				t.Info("ready", "codec", t.ICodecCtx.FourCC(), "info", t.ICodecCtx.GetInfo(), "channels", ctx.GetChannels(), "sample_rate", ctx.GetSampleRate())
			}
		}
		t.ready.Fulfill(err)
	}
}

func (t *Track) Ready(err error) {
	if t.ready.IsPending() {
		if err != nil {
			t.Error("ready", "err", err)
		} else {
			t.Info("ready")
		}
		t.ready.Fulfill(err)
	}
}

func (t *Track) IsReady() bool {
	return !t.ready.IsPending()
}

func (t *Track) WaitReady() error {
	return t.ready.Await()
}

func (t *Track) Trace(msg string, fields ...any) {
	t.Log(context.TODO(), task.TraceLevel, msg, fields...)
}

func (t *TsTamer) Tame(ts time.Duration, fps int, scale float64) (result time.Duration) {
	if t.LastTs == 0 {
		t.BaseTs -= ts
	}
	result = max(1*time.Millisecond, t.BaseTs+ts)

	// 突变检测：仅在 fps 合理的情况下启用
	// 如果 fps > 100，说明是快速下载场景，接收速度远大于真实帧率，不应该做突变检测
	if fps > 0 && fps <= 100 {
		frameDur := float64(time.Second) / float64(fps)
		diff := math.Abs(float64(result - t.LastTs))
		threshold := 10 * frameDur * scale
		if diff > threshold { //时间戳突变
			result = t.LastTs + time.Duration(frameDur)
			t.BaseTs = result - ts
		}
	}

	t.LastTs = result
	if t.LastScale != scale {
		t.BeforeScaleChangedTs = result
		t.LastScale = scale
	}
	result = t.BeforeScaleChangedTs + time.Duration(float64(result-t.BeforeScaleChangedTs)/scale)
	return
}

func (t *AVTrack) SpeedControl(speed float64) {
	t.speedControl(speed, t.LastTs)
}

func (t *AVTrack) AddPausedTime(d time.Duration) {
	t.pausedTime += d
}

// GetSpeed 返回当前的播放倍速
func (t *AVTrack) GetSpeed() float64 {
	return t.speed
}

// ResetSpeedController 重置倍速控制器的状态，用于避免状态冲突
func (t *AVTrack) ResetSpeedController() {
	t.speed = 1.0 // 重置为正常速度
	t.beginTime = time.Time{}
	t.beginTimestamp = 0
	t.Delta = 0
	t.speedFrameCount = 0
	t.lastAdjustTime = time.Time{}
	t.lastTimestamp = 0
	t.Info("speed controller reset")
}

func (t *AVTrack) speedControl(speed float64, ts time.Duration) {
	now := time.Now()

	if speed != t.speed || t.beginTime.IsZero() {
		// 倍速改变或首次调用，重新初始化
		t.speed = speed
		t.beginTime = now
		t.beginTimestamp = ts // 记录开始播放时的第一个时间戳
		t.pausedTime = 0
		t.Delta = 0
		t.speedFrameCount = 0
		return
	}

	if speed == 0 {
		// 暂停模式
		return
	}

	// 检测时间戳重置（视频循环播放时）
	if t.lastTimestamp != 0 && ts < t.lastTimestamp-100*time.Millisecond {
		// 时间戳大幅倒退，重置倍速控制状态
		t.Info("timestamp reset detected, reinitializing speed control", "last_ts", t.lastTimestamp.Milliseconds(), "current_ts", ts.Milliseconds())
		t.beginTime = now
		t.beginTimestamp = ts
		t.pausedTime = 0
		t.Delta = 0
		t.speedFrameCount = 0
		return
	}
	t.lastTimestamp = ts

	elapsed := now.Sub(t.beginTime) - t.pausedTime
	t.speedFrameCount++

	// 正确的倍速控制算法：
	// 视频时间戳是按正常速度编码的，要倍速播放就需要按比例压缩播放时间
	// 理论播放时间 = (当前时间戳 - 开始时间戳) / 倍速
	theoreticalElapsed := time.Duration(float64(ts-t.beginTimestamp) / speed)

	// 计算需要休眠的时间：理论时间 - 实际时间
	t.Delta = theoreticalElapsed - elapsed

	// 限制Delta在合理范围内
	if t.Delta > 500*time.Millisecond {
		t.Delta = 500 * time.Millisecond
	} else if t.Delta < -500*time.Millisecond {
		t.Delta = -500 * time.Millisecond
	}

	// 动态负载保护：较短时间后激活，防止超时
	if elapsed > 2*time.Minute && t.speedFrameCount%200 == 0 {
		// 如果Delta太小（休眠时间太短），增加休眠时间保护系统
		if t.Delta < 5*time.Millisecond {
			t.Delta = 5 * time.Millisecond
			if t.Logger.Enabled(t.ready, task.TraceLevel) {
				t.Trace("load protection activated", "elapsed_min", elapsed.Minutes(), "forced_delta_ms", t.Delta.Milliseconds())
			}
		}
	}

	// 限制Delta在合理范围内，避免极端情况
	if t.Delta > 500*time.Millisecond {
		t.Delta = 500 * time.Millisecond
	} else if t.Delta < -500*time.Millisecond {
		t.Delta = -500 * time.Millisecond
	}

	// 根据倍速调整控制参数
	var controlThreshold, maxSleep time.Duration
	if speed <= 2 {
		controlThreshold = 1 * time.Millisecond // 低倍速更精确控制
		maxSleep = 200 * time.Millisecond
	} else if speed <= 8 {
		controlThreshold = 1 * time.Millisecond // 中等倍速也要精确控制
		maxSleep = 100 * time.Millisecond
	} else {
		controlThreshold = 1 * time.Millisecond // 高倍速也需要控制
		maxSleep = 50 * time.Millisecond
	}

	// 移除定期重新同步，避免播放速度突然改变
	// 系统会自然适应，不需要强制重新同步

	// 只有当需要休眠的时间超过阈值时才进行休眠
	if t.Delta > controlThreshold {
		sleepTime := min(t.Delta, maxSleep)
		// 计算实际倍速：(当前时间戳 - 开始时间戳) / 播放经过时间
		actualSpeedRatio := float64(ts.Milliseconds()-t.beginTimestamp.Milliseconds()) / float64(elapsed.Milliseconds())
		t.Trace("SPEED_CONTROL", "speed", speed, "elapsed_ms", elapsed.Milliseconds(),
			"current_ts_ms", ts.Milliseconds(), "begin_ts_ms", t.beginTimestamp.Milliseconds(),
			"delta_ms", t.Delta.Milliseconds(), "sleep_ms", sleepTime.Milliseconds(),
			"actual_speed_ratio", actualSpeedRatio)
		time.Sleep(sleepTime)
	} else {
		// 记录所有SpeedControl调用，即使不休眠
		t.Trace("SPEED_CONTROL_NO_SLEEP", "speed", speed, "elapsed_ms", elapsed.Milliseconds(),
			"current_ts_ms", ts.Milliseconds(), "delta_ms", t.Delta.Milliseconds(), "threshold_ms", controlThreshold.Milliseconds())
	}
}
