package pkg

import (
	"log/slog"
	"slices"

	"m7s.live/m7s/v5/pkg/util"
)

type Track struct {
	*slog.Logger `json:"-" yaml:"-"`
}

type DataTrack struct {
	Track
}

type IDRingList struct {
	IDRList     []*util.Ring[AVFrame]
	IDRing      *util.Ring[AVFrame]
	HistoryRing *util.Ring[AVFrame]
}

func (p *IDRingList) AddIDR(IDRing *util.Ring[AVFrame]) {
	p.IDRList = append(p.IDRList, IDRing)
	p.IDRing = IDRing
}

func (p *IDRingList) ShiftIDR() {
	p.IDRList = slices.Delete(p.IDRList, 0, 1)
	p.HistoryRing = p.IDRList[0]
}

type AVTrack struct {
	Codec string
	Track
	RingWriter
	IDRingList `json:"-" yaml:"-"` //最近的关键帧位置，首屏渲染
	ICodecCtx
	SSRC        uint32
	SampleRate  uint32
	PayloadType byte
}
