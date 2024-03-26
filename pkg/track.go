package pkg

import (
	"log/slog"
	"slices"
	"time"

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
	BufferTime time.Duration       //发布者配置中的缓冲时间（时光回溯）
	ICodecCtx
	SSRC        uint32
	SampleRate  uint32
	PayloadType byte
}

func (av *AVTrack) Narrow(gop int) {
	if l := av.Size - gop; l > 12 {
		av.Debug("resize", "before", av.Size, "after", av.Size-5)
		//缩小缓冲环节省内存
		av.Reduce(5)
	}
}

func (av *AVTrack) AddIDR(r *util.Ring[AVFrame]) {
	if av.BufferTime > 0 {
		av.IDRingList.AddIDR(r)
		if av.HistoryRing == nil {
			av.HistoryRing = av.IDRing
		}
	} else {
		av.IDRing = r
	}
}
