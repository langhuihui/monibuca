package pkg

import (
	"fmt"
	"time"

	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
)

type AnnexB struct {
	PTS time.Duration
	DTS time.Duration
	util.RecyclableMemory
}

// DecodeConfig implements pkg.IAVFrame.
func (a *AnnexB) DecodeConfig(t *AVTrack, ctx ICodecCtx) error {
	switch c := ctx.(type) {
	case codec.IH264Ctx:
		var annexb264 Annexb264Ctx
		annexb264.H264Ctx = *c.GetH264Ctx()
		t.ICodecCtx = &annexb264
	}
	return nil
}

// GetSize implements pkg.IAVFrame.
func (a *AnnexB) GetSize() int {
	return a.Size
}

func (a *AnnexB) GetTimestamp() time.Duration {
	return a.DTS * time.Millisecond / 90
}

// Parse implements pkg.IAVFrame.
func (a *AnnexB) Parse(t *AVTrack) (isIDR bool, isSeq bool, raw any, err error) {
	panic("unimplemented")
}

// String implements pkg.IAVFrame.
func (a *AnnexB) String() string {
	return fmt.Sprintf("%d %d", a.DTS, a.Memory.Size)
}

// ToRaw implements pkg.IAVFrame.
func (a *AnnexB) ToRaw(ctx ICodecCtx) (any, error) {
	// var nalus Nalus
	// nalus.PTS = a.PTS
	// nalus.DTS = a.DTS
	panic("unimplemented")
}

type Annexb264Ctx struct {
	codec.H264Ctx
}

type Annexb265Ctx struct {
	codec.H265Ctx
}

func (a *Annexb264Ctx) CreateFrame(frame *AVFrame) (IAVFrame, error) {
	var annexb AnnexB
	// annexb.RecyclableBuffers.ScalableMemoryAllocator = frame.Wraps[0].GetScalableMemoryAllocator()
	annexb.ReadFromBytes(codec.NALU_Delimiter2)
	if frame.IDR {
		annexb.ReadFromBytes(a.SPS[0], codec.NALU_Delimiter2, a.PPS[0], codec.NALU_Delimiter2)
	}
	var nalus = frame.Raw.(Nalus)
	for i, nalu := range nalus.Nalus {
		if i > 0 {
			annexb.ReadFromBytes(codec.NALU_Delimiter1)
		}
		annexb.ReadFromBytes(nalu...)
	}
	return &annexb, nil
}
