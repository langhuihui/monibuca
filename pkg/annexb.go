package pkg

import (
	"encoding/binary"
	"fmt"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/codec/h265parser"
	"io"
	"time"

	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
)

var _ IAVFrame = (*AnnexB)(nil)

type AnnexB struct {
	Hevc bool
	PTS  time.Duration
	DTS  time.Duration
	util.RecyclableMemory
}

func (a *AnnexB) Dump(t byte, w io.Writer) {
	m := a.GetAllocator().Borrow(4 + a.Size)
	binary.BigEndian.PutUint32(m, uint32(a.Size))
	a.CopyTo(m[4:])
	w.Write(m)
}

// DecodeConfig implements pkg.IAVFrame.
func (a *AnnexB) ConvertCtx(ctx codec.ICodecCtx) (codec.ICodecCtx, IAVFrame, error) {
	return ctx.GetBase(), nil, nil
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
	if a.Hevc {
		if t.ICodecCtx == nil {
			t.ICodecCtx = &codec.H265Ctx{}
		}
	} else {
		if t.ICodecCtx == nil {
			t.ICodecCtx = &codec.H264Ctx{}
		}
	}
	if t.Value.Raw, err = a.Demux(t.ICodecCtx); err != nil {
		return
	}
	for _, nalu := range t.Value.Raw.(Nalus) {
		if a.Hevc {
			ctx := t.ICodecCtx.(*codec.H265Ctx)
			switch codec.ParseH265NALUType(nalu.Buffers[0][0]) {
			case h265parser.NAL_UNIT_VPS:
				ctx.RecordInfo.VPS = [][]byte{nalu.ToBytes()}
			case h265parser.NAL_UNIT_SPS:
				ctx.RecordInfo.SPS = [][]byte{nalu.ToBytes()}
			case h265parser.NAL_UNIT_PPS:
				ctx.RecordInfo.PPS = [][]byte{nalu.ToBytes()}
				ctx.CodecData, err = h265parser.NewCodecDataFromVPSAndSPSAndPPS(ctx.VPS(), ctx.SPS(), ctx.PPS())
			case h265parser.NAL_UNIT_CODED_SLICE_BLA_W_LP,
				h265parser.NAL_UNIT_CODED_SLICE_BLA_W_RADL,
				h265parser.NAL_UNIT_CODED_SLICE_BLA_N_LP,
				h265parser.NAL_UNIT_CODED_SLICE_IDR_W_RADL,
				h265parser.NAL_UNIT_CODED_SLICE_IDR_N_LP,
				h265parser.NAL_UNIT_CODED_SLICE_CRA:
				t.Value.IDR = true
			}
		} else {
			ctx := t.ICodecCtx.(*codec.H264Ctx)
			switch codec.ParseH264NALUType(nalu.Buffers[0][0]) {
			case codec.NALU_SPS:
				ctx.RecordInfo.SPS = [][]byte{nalu.ToBytes()}
			case codec.NALU_PPS:
				ctx.RecordInfo.PPS = [][]byte{nalu.ToBytes()}
				ctx.CodecData, err = h264parser.NewCodecDataFromSPSAndPPS(ctx.SPS(), ctx.PPS())
			case codec.NALU_IDR_Picture:
				t.Value.IDR = true
			}
		}
	}
	return
}

// String implements pkg.IAVFrame.
func (a *AnnexB) String() string {
	return fmt.Sprintf("%d %d", a.DTS, a.Memory.Size)
}

// Demux implements pkg.IAVFrame.
func (a *AnnexB) Demux(codecCtx codec.ICodecCtx) (ret any, err error) {
	var nalus Nalus
	var lastFourBytes [4]byte
	var b byte
	var shallow util.Memory
	shallow.Append(a.Buffers...)
	reader := shallow.NewReader()

	gotNalu := func() {
		var nalu util.Memory
		for buf := range reader.ClipFront {
			nalu.AppendOne(buf)
		}
		nalus = append(nalus, nalu)

	}

	for {
		b, err = reader.ReadByte()
		if err == nil {
			copy(lastFourBytes[:], lastFourBytes[1:])
			lastFourBytes[3] = b
			var startCode = 0
			if lastFourBytes == codec.NALU_Delimiter2 {
				startCode = 4
			} else if [3]byte(lastFourBytes[1:]) == codec.NALU_Delimiter1 {
				startCode = 3
			}
			if startCode > 0 {
				reader.Unread(startCode)
				if reader.Offset() > 0 {
					gotNalu()
				}
				reader.Skip(startCode)
				for range reader.ClipFront {
				}
			}
		} else if err == io.EOF {
			if reader.Offset() > 0 {
				gotNalu()
			}
			err = nil
			break
		}
	}
	ret = nalus
	return
}

func (a *AnnexB) Mux(codecCtx codec.ICodecCtx, frame *AVFrame) {
	delimiter2 := codec.NALU_Delimiter2[:]
	a.AppendOne(delimiter2)
	if frame.IDR {
		switch ctx := codecCtx.(type) {
		case *codec.H264Ctx:
			a.Append(ctx.SPS(), delimiter2, ctx.PPS(), delimiter2)
		case *codec.H265Ctx:
			a.Append(ctx.SPS(), delimiter2, ctx.PPS(), delimiter2, ctx.VPS(), delimiter2)
		}
	}
	for i, nalu := range frame.Raw.(Nalus) {
		if i > 0 {
			a.AppendOne(codec.NALU_Delimiter1[:])
		}
		a.Append(nalu.Buffers...)
	}
}
