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
func (a *AnnexB) ConvertCtx(ctx codec.ICodecCtx, t *AVTrack) error {
	t.ICodecCtx = ctx.GetBase()
	return nil
}

// GetSize implements pkg.IAVFrame.
func (a *AnnexB) GetSize() int {
	return a.Size
}

func (a *AnnexB) GetTimestamp() time.Duration {
	return a.DTS * time.Millisecond / 90
}
func (a *AnnexB) GetCTS() time.Duration {
	return (a.PTS - a.DTS) * time.Millisecond / 90
}

// Parse implements pkg.IAVFrame.
func (a *AnnexB) Parse(t *AVTrack) (err error) {
	panic("unimplemented")
}

// String implements pkg.IAVFrame.
func (a *AnnexB) String() string {
	return fmt.Sprintf("%d %d", a.DTS, a.Memory.Size)
}

// Demux implements pkg.IAVFrame.
func (a *AnnexB) Demux(ctx codec.ICodecCtx) (any, error) {
	panic("unimplemented")
}

func (a *AnnexB) Mux(codecCtx codec.ICodecCtx, frame *AVFrame) {
	a.AppendOne(codec.NALU_Delimiter2)
	if frame.IDR {
		switch ctx := codecCtx.(type) {
		case *codec.H264Ctx:
			a.Append(ctx.SPS[0], codec.NALU_Delimiter2, ctx.PPS[0], codec.NALU_Delimiter2)
		case *codec.H265Ctx:
			a.Append(ctx.SPS[0], codec.NALU_Delimiter2, ctx.PPS[0], codec.NALU_Delimiter2, ctx.VPS[0], codec.NALU_Delimiter2)
		}
	}
	for i, nalu := range frame.Raw.(Nalus) {
		if i > 0 {
			a.AppendOne(codec.NALU_Delimiter1)
		}
		a.Append(nalu.Buffers...)
	}
}
