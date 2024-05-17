package pkg

import (
	"context"
	"log/slog"
	"reflect"
	"slices"
	"sync/atomic"
	"time"

	"m7s.live/m7s/v5/pkg/util"
)

type (
	Track struct {
		*slog.Logger
		Ready       *util.Promise[struct{}]
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

	IDRingList struct {
		IDRList     []*AVRing
		IDRing      atomic.Pointer[AVRing]
		HistoryRing atomic.Pointer[AVRing]
	}

	AVTrack struct {
		Track
		*RingWriter
		ICodecCtx
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
		case reflect.Type:
			t.FrameType = v
		case *slog.Logger:
			t.Logger = v
		case *AVTrack:
			t.Logger = v.Logger.With("subtrack", t.FrameType.String())
			t.RingWriter = v.RingWriter
		case int:
			t.RingWriter = NewRingWriter(v)
		}
	}
	t.Ready = util.NewPromise(struct{}{})
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

func (t *Track) Trace(msg string, fields ...any) {
	t.Log(context.TODO(), TraceLevel, msg, fields...)
}

func (p *IDRingList) AddIDR(IDRing *AVRing) {
	p.IDRList = append(p.IDRList, IDRing)
	p.IDRing.Store(IDRing)
}

func (p *IDRingList) ShiftIDR() {
	p.IDRList = slices.Delete(p.IDRList, 0, 1)
	p.HistoryRing.Store(p.IDRList[0])
}
