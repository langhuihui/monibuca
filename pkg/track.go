package pkg

import (
	"context"
	"log/slog"
	"math"
	"reflect"
	"time"

	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"

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
		speed          float64
		pausedTime     time.Duration
		beginTime      time.Time
		beginTimestamp time.Duration
		Delta          time.Duration
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
		Allocator *util.ScalableMemoryAllocator
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

func (t *AVTrack) CheckIfNeedDropFrame(maxFPS int) (drop bool) {
	drop = maxFPS > 0 && (t.accpetFPS > maxFPS)
	if drop {
		defer func() {
			if time.Since(t.LastDropLevelChange) > time.Second && t.DropFrameLevel > 0 {
				t.changeDropFrameLevel(t.DropFrameLevel + 1)
			}
		}()
	}
	// Enhanced frame dropping strategy based on DropFrameLevel
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
	if fps > 0 {
		frameDur := float64(time.Second) / float64(fps)
		if math.Abs(float64(result-t.LastTs)) > 10*frameDur*scale { //时间戳突变
			// t.Warn("timestamp mutation", "fps", t.FPS, "lastTs", uint32(t.LastTs/time.Millisecond), "ts", uint32(frame.Timestamp/time.Millisecond), "frameDur", time.Duration(frameDur))
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

func (t *AVTrack) speedControl(speed float64, ts time.Duration) {
	if speed != t.speed || t.beginTime.IsZero() {
		t.speed = speed
		t.beginTime = time.Now()
		t.beginTimestamp = ts
		t.pausedTime = 0
	} else {
		elapsed := time.Since(t.beginTime) - t.pausedTime
		if speed == 0 {
			t.Delta = ts - elapsed
			if t.Logger.Enabled(t.ready, task.TraceLevel) {
				t.Trace("speed 0", "ts", ts, "elapsed", elapsed, "delta", t.Delta)
			}
			return
		}
		should := time.Duration(float64(ts-t.beginTimestamp) / speed)
		t.Delta = should - elapsed
		if t.Delta > threshold {
			if t.Logger.Enabled(t.ready, task.TraceLevel) {
				t.Trace("speed control", "speed", speed, "elapsed", elapsed, "should", should, "delta", t.Delta)
			}
			time.Sleep(min(t.Delta, time.Millisecond*500))
		}
	}
}
