package pkg

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
)

type AnnexB struct {
	PTS time.Duration
	DTS time.Duration
	util.RecyclableMemory
}

func (a *AnnexB) Dump(t byte, w io.Writer) {
	m := a.Borrow(4 + a.Size)
	binary.BigEndian.PutUint32(m, uint32(a.Size))
	a.CopyTo(m[4:])
	w.Write(m)
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
	annexb.Append(codec.NALU_Delimiter2)
	if frame.IDR {
		annexb.Append(a.SPS[0], codec.NALU_Delimiter2, a.PPS[0], codec.NALU_Delimiter2)
	}
	var nalus = frame.Raw.(Nalus)
	for i, nalu := range nalus.Nalus {
		if i > 0 {
			annexb.Append(codec.NALU_Delimiter1)
		}
		annexb.Append(nalu.Buffers...)
	}
	return &annexb, nil
}
