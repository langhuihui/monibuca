package mpegts

import (
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/format"
)

type VideoFrame struct {
	format.AnnexB
}

func (a *VideoFrame) Mux(fromBase *pkg.Sample) (err error) {
	if fromBase.GetBase().FourCC().Is(codec.FourCC_H265) {
		a.PushOne(codec.AudNalu)
	} else {
		a.PushOne(codec.NALU_AUD_BYTE)
	}
	return a.AnnexB.Mux(fromBase)
}
