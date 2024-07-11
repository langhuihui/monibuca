package pkg

import (
	"context"
	"log/slog"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/config"
	"reflect"
	"time"

	"m7s.live/m7s/v5/pkg/util"
)

type (
	Track struct {
		*slog.Logger
		ready       *util.Promise[struct{}]
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

	AVTrack struct {
		Track
		*RingWriter
		codec.ICodecCtx
		Allocator     *util.ScalableMemoryAllocator
		SequenceFrame IAVFrame
		WrapIndex     int
	}
)

func NewAVTrack(args ...any) (t *AVTrack) {
	t = &AVTrack{}
	for _, arg := range args {
		switch v := arg.(type) {
		case IAVFrame:
			t.FrameType = reflect.TypeOf(v)
			t.Allocator = v.GetAllocator()
		case reflect.Type:
			t.FrameType = v
		case *slog.Logger:
			t.Logger = v
		case *AVTrack:
			t.Logger = v.Logger.With("subtrack", t.FrameType.String())
			t.RingWriter = v.RingWriter
		case *config.Publish:
			t.RingWriter = NewRingWriter(v.RingSize)
			t.BufferRange[0] = v.BufferTime
			t.RingWriter.SLogger = t.Logger
		}
	}
	t.ready = util.NewPromise(struct{}{})
	t.Info("create")
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

func (t *AVTrack) Ready(err error) {
	if !t.IsReady() {
		if err != nil {
			t.Error("ready", "err", err)
		} else {
			switch ctx := t.ICodecCtx.(type) {
			case IVideoCodecCtx:
				t.Info("ready", "info", t.ICodecCtx.GetInfo(), "width", ctx.Width(), "height", ctx.Height())
			case IAudioCodecCtx:
				t.Info("ready", "info", t.ICodecCtx.GetInfo(), "channels", ctx.GetChannels(), "sample_rate", ctx.GetSampleRate())
			}
		}
		t.ready.Fulfill(err)
	}
}

func (t *Track) Ready(err error) {
	if !t.IsReady() {
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
	_, err := t.ready.Await()
	return err
}

func (t *Track) Trace(msg string, fields ...any) {
	t.Log(context.TODO(), TraceLevel, msg, fields...)
}
