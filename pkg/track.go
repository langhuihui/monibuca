package pkg

import (
	"log/slog"
	"reflect"
	"slices"
	"sync/atomic"

	"m7s.live/m7s/v5/pkg/codec"
)

type (
	Track struct {
		*slog.Logger `json:"-" yaml:"-"`
		FrameType    reflect.Type
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
		Codec codec.FourCC
		Track
		RingWriter
		IDRingList `json:"-" yaml:"-"` //最近的关键帧位置，首屏渲染
		ICodecCtx
	}
)

func (t *Track) GetKey() reflect.Type {
	return t.FrameType
}

func (p *IDRingList) AddIDR(IDRing *AVRing) {
	p.IDRList = append(p.IDRList, IDRing)
	p.IDRing.Store(IDRing)
}

func (p *IDRingList) ShiftIDR() {
	p.IDRList = slices.Delete(p.IDRList, 0, 1)
	p.HistoryRing.Store(p.IDRList[0])
}
