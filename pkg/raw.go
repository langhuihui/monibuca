package pkg

import (
	"fmt"
	"github.com/deepch/vdk/codec/aacparser"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/codec/h265parser"
	"io"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
	"time"
)

var _ IAVFrame = (*RawAudio)(nil)

type RawAudio struct {
	codec.FourCC
	Timestamp time.Duration
	util.RecyclableMemory
}

func (r *RawAudio) Parse(track *AVTrack) (err error) {
	if track.ICodecCtx == nil {
		switch r.FourCC {
		case codec.FourCC_MP4A:
			ctx := &codec.AACCtx{}
			ctx.CodecData, err = aacparser.NewCodecDataFromMPEG4AudioConfigBytes(r.ToBytes())
			track.ICodecCtx = ctx
		case codec.FourCC_ALAW:
			track.ICodecCtx = &codec.PCMACtx{
				AudioCtx: codec.AudioCtx{
					SampleRate: 8000,
					Channels:   1,
					SampleSize: 8,
				},
			}
		case codec.FourCC_ULAW:
			track.ICodecCtx = &codec.PCMUCtx{
				AudioCtx: codec.AudioCtx{
					SampleRate: 8000,
					Channels:   1,
					SampleSize: 8,
				},
			}
		}
	}
	return
}

func (r *RawAudio) ConvertCtx(ctx codec.ICodecCtx) (codec.ICodecCtx, IAVFrame, error) {
	c := ctx.GetBase()
	if c.FourCC().Is(codec.FourCC_MP4A) {
		seq := &RawAudio{
			FourCC:    codec.FourCC_MP4A,
			Timestamp: r.Timestamp,
		}
		seq.SetAllocator(r.GetAllocator())
		seq.Memory.Append(c.GetRecord())
		return c, seq, nil
	}
	return c, nil, nil
}

func (r *RawAudio) Demux(ctx codec.ICodecCtx) (any, error) {
	return r.Memory, nil
}

func (r *RawAudio) Mux(ctx codec.ICodecCtx, frame *AVFrame) {
	r.InitRecycleIndexes(0)
	r.FourCC = ctx.FourCC()
	r.Memory = frame.Raw.(util.Memory)
	r.Timestamp = frame.Timestamp
}

func (r *RawAudio) GetTimestamp() time.Duration {
	return r.Timestamp
}

func (r *RawAudio) GetCTS() time.Duration {
	return 0
}

func (r *RawAudio) GetSize() int {
	return r.Size
}

func (r *RawAudio) String() string {
	return fmt.Sprintf("RawAudio{FourCC: %s, Timestamp: %s, Size: %d}", r.FourCC, r.Timestamp, r.Size)
}

func (r *RawAudio) Dump(b byte, writer io.Writer) {
	//TODO implement me
	panic("implement me")
}

var _ IAVFrame = (*H26xFrame)(nil)

type H26xFrame struct {
	codec.FourCC
	Timestamp time.Duration
	CTS       time.Duration
	Nalus
	util.RecyclableMemory
}

func (h *H26xFrame) Parse(track *AVTrack) (err error) {
	switch h.FourCC {
	case codec.FourCC_H264:
		var ctx *codec.H264Ctx
		if track.ICodecCtx != nil {
			ctx = track.ICodecCtx.GetBase().(*codec.H264Ctx)
		}
		for _, nalu := range h.Nalus {
			switch codec.ParseH264NALUType(nalu.Buffers[0][0]) {
			case h264parser.NALU_SPS:
				ctx = &codec.H264Ctx{}
				track.ICodecCtx = ctx
				ctx.RecordInfo.SPS = [][]byte{nalu.ToBytes()}
				if ctx.SPSInfo, err = h264parser.ParseSPS(ctx.SPS()); err != nil {
					return
				}
			case h264parser.NALU_PPS:
				ctx.RecordInfo.PPS = [][]byte{nalu.ToBytes()}
				ctx.CodecData, err = h264parser.NewCodecDataFromSPSAndPPS(ctx.SPS(), ctx.PPS())
				if err != nil {
					return
				}
			case codec.NALU_IDR_Picture:
				track.Value.IDR = true
			}
		}
	case codec.FourCC_H265:
		var ctx *codec.H265Ctx
		if track.ICodecCtx != nil {
			ctx = track.ICodecCtx.GetBase().(*codec.H265Ctx)
		}
		for _, nalu := range h.Nalus {
			switch codec.ParseH265NALUType(nalu.Buffers[0][0]) {
			case h265parser.NAL_UNIT_VPS:
				ctx = &codec.H265Ctx{}
				ctx.RecordInfo.VPS = [][]byte{nalu.ToBytes()}
				track.ICodecCtx = ctx
			case h265parser.NAL_UNIT_SPS:
				ctx.RecordInfo.SPS = [][]byte{nalu.ToBytes()}
				if ctx.SPSInfo, err = h265parser.ParseSPS(ctx.SPS()); err != nil {
					return
				}
			case h265parser.NAL_UNIT_PPS:
				ctx.RecordInfo.PPS = [][]byte{nalu.ToBytes()}
				ctx.CodecData, err = h265parser.NewCodecDataFromVPSAndSPSAndPPS(ctx.VPS(), ctx.SPS(), ctx.PPS())
			case h265parser.NAL_UNIT_CODED_SLICE_BLA_W_LP,
				h265parser.NAL_UNIT_CODED_SLICE_BLA_W_RADL,
				h265parser.NAL_UNIT_CODED_SLICE_BLA_N_LP,
				h265parser.NAL_UNIT_CODED_SLICE_IDR_W_RADL,
				h265parser.NAL_UNIT_CODED_SLICE_IDR_N_LP,
				h265parser.NAL_UNIT_CODED_SLICE_CRA:
				track.Value.IDR = true
			}
		}
	}
	return
}

func (h *H26xFrame) ConvertCtx(ctx codec.ICodecCtx) (codec.ICodecCtx, IAVFrame, error) {
	switch c := ctx.GetBase().(type) {
	case *codec.H264Ctx:
		return c, &H26xFrame{
			FourCC: codec.FourCC_H264,
			Nalus: []util.Memory{
				util.NewMemory(c.SPS()),
				util.NewMemory(c.PPS()),
			},
		}, nil
	case *codec.H265Ctx:
		return c, &H26xFrame{
			FourCC: codec.FourCC_H265,
			Nalus: []util.Memory{
				util.NewMemory(c.VPS()),
				util.NewMemory(c.SPS()),
				util.NewMemory(c.PPS()),
			},
		}, nil
	}
	return ctx.GetBase(), nil, nil
}

func (h *H26xFrame) Demux(ctx codec.ICodecCtx) (any, error) {
	return h.Nalus, nil
}

func (h *H26xFrame) Mux(ctx codec.ICodecCtx, frame *AVFrame) {
	h.FourCC = ctx.FourCC()
	h.Nalus = frame.Raw.(Nalus)
	h.Timestamp = frame.Timestamp
	h.CTS = frame.CTS
}

func (h *H26xFrame) GetTimestamp() time.Duration {
	return h.Timestamp
}

func (h *H26xFrame) GetCTS() time.Duration {
	return h.CTS
}

func (h *H26xFrame) GetSize() int {
	var size int
	for _, nalu := range h.Nalus {
		size += nalu.Size
	}
	return size
}

func (h *H26xFrame) String() string {
	return fmt.Sprintf("H26xFrame{FourCC: %s, Timestamp: %s, CTS: %s}", h.FourCC, h.Timestamp, h.CTS)
}

func (h *H26xFrame) Dump(b byte, writer io.Writer) {
	//TODO implement me
	panic("implement me")
}
