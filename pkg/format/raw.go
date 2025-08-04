package format

import (
	"bytes"
	"fmt"

	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/codec/h265parser"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
)

var _ pkg.IAVFrame = (*RawAudio)(nil)

type RawAudio struct {
	pkg.Sample
}

func (r *RawAudio) GetSize() int {
	return r.Raw.(*util.Memory).Size
}

func (r *RawAudio) Demux() error {
	r.Raw = &r.Memory
	return nil
}

func (r *RawAudio) Mux(from *pkg.Sample) (err error) {
	r.InitRecycleIndexes(0)
	r.Memory = *from.Raw.(*util.Memory)
	r.ICodecCtx = from.GetBase()
	return
}

func (r *RawAudio) String() string {
	return fmt.Sprintf("RawAudio{FourCC: %s, Timestamp: %s, Size: %d}", r.FourCC(), r.Timestamp, r.Size)
}

var _ pkg.IAVFrame = (*H26xFrame)(nil)

type H26xFrame struct {
	pkg.Sample
}

func (h *H26xFrame) CheckCodecChange() (err error) {
	if h.ICodecCtx == nil {
		return pkg.ErrUnsupportCodec
	}
	var hasVideoFrame bool
	switch ctx := h.GetBase().(type) {
	case *codec.H264Ctx:
		var sps, pps []byte
		for nalu := range h.Raw.(*pkg.Nalus).RangePoint {
			switch codec.ParseH264NALUType(nalu.Buffers[0][0]) {
			case codec.NALU_SPS:
				sps = nalu.ToBytes()
			case codec.NALU_PPS:
				pps = nalu.ToBytes()
			case codec.NALU_IDR_Picture:
				h.IDR = true
			case codec.NALU_Non_IDR_Picture:
				hasVideoFrame = true
			}
		}
		if sps != nil && pps != nil {
			var codecData h264parser.CodecData
			codecData, err = h264parser.NewCodecDataFromSPSAndPPS(sps, pps)
			if err != nil {
				return
			}
			if !bytes.Equal(codecData.Record, ctx.Record) {
				h.ICodecCtx = &codec.H264Ctx{
					CodecData: codecData,
				}
			}
		}
	case *codec.H265Ctx:
		var vps, sps, pps []byte
		for nalu := range h.Raw.(*pkg.Nalus).RangePoint {
			switch codec.ParseH265NALUType(nalu.Buffers[0][0]) {
			case h265parser.NAL_UNIT_VPS:
				vps = nalu.ToBytes()
			case h265parser.NAL_UNIT_SPS:
				sps = nalu.ToBytes()
			case h265parser.NAL_UNIT_PPS:
				pps = nalu.ToBytes()
			case h265parser.NAL_UNIT_CODED_SLICE_BLA_W_LP,
				h265parser.NAL_UNIT_CODED_SLICE_BLA_W_RADL,
				h265parser.NAL_UNIT_CODED_SLICE_BLA_N_LP,
				h265parser.NAL_UNIT_CODED_SLICE_IDR_W_RADL,
				h265parser.NAL_UNIT_CODED_SLICE_IDR_N_LP,
				h265parser.NAL_UNIT_CODED_SLICE_CRA:
				h.IDR = true
			case 1, 2, 3, 4, 5, 6, 7, 8, 9:
				hasVideoFrame = true
			}
		}
		if vps != nil && sps != nil && pps != nil {
			var codecData h265parser.CodecData
			codecData, err = h265parser.NewCodecDataFromVPSAndSPSAndPPS(vps, sps, pps)
			if err != nil {
				return
			}
			if !bytes.Equal(codecData.Record, ctx.Record) {
				h.ICodecCtx = &codec.H265Ctx{
					CodecData: codecData,
				}
			}
		}
	}
	// Return ErrSkip if no video frames are present (only metadata NALUs)
	if !hasVideoFrame && !h.IDR {
		return pkg.ErrSkip
	}
	return
}

func (r *H26xFrame) GetSize() (ret int) {
	switch raw := r.Raw.(type) {
	case *pkg.Nalus:
		for nalu := range raw.RangePoint {
			ret += nalu.Size
		}
	}
	return
}

func (h *H26xFrame) String() string {
	return fmt.Sprintf("H26xFrame{FourCC: %s, Timestamp: %s, CTS: %s}", h.FourCC, h.Timestamp, h.CTS)
}
