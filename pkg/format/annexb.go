package format

import (
	"bytes"
	"fmt"
	"io"
	"slices"

	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/codec/h265parser"

	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
)

type AnnexB struct {
	pkg.Sample
}

func (a *AnnexB) CheckCodecChange() (err error) {
	if !a.HasRaw() || a.ICodecCtx == nil {
		err = a.Demux()
		if err != nil {
			return
		}
	}
	if a.ICodecCtx == nil {
		return pkg.ErrSkip
	}
	var vps, sps, pps []byte
	a.IDR = false
	for nalu := range a.Raw.(*pkg.Nalus).RangePoint {
		if a.FourCC() == codec.FourCC_H265 {
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
				a.IDR = true
			}
		} else {
			switch codec.ParseH264NALUType(nalu.Buffers[0][0]) {
			case codec.NALU_SPS:
				sps = nalu.ToBytes()
			case codec.NALU_PPS:
				pps = nalu.ToBytes()
			case codec.NALU_IDR_Picture:
				a.IDR = true
			}
		}
	}
	if a.FourCC() == codec.FourCC_H265 {
		if vps != nil && sps != nil && pps != nil {
			var codecData h265parser.CodecData
			codecData, err = h265parser.NewCodecDataFromVPSAndSPSAndPPS(vps, sps, pps)
			if err != nil {
				return
			}
			if !bytes.Equal(codecData.Record, a.ICodecCtx.(*codec.H265Ctx).Record) {
				a.ICodecCtx = &codec.H265Ctx{
					CodecData: codecData,
				}
			}
		}
		if a.ICodecCtx.(*codec.H265Ctx).Record == nil {
			err = pkg.ErrSkip
		}
	} else {
		if sps != nil && pps != nil {
			var codecData h264parser.CodecData
			codecData, err = h264parser.NewCodecDataFromSPSAndPPS(sps, pps)
			if err != nil {
				return
			}
			if !bytes.Equal(codecData.Record, a.ICodecCtx.(*codec.H264Ctx).Record) {
				a.ICodecCtx = &codec.H264Ctx{
					CodecData: codecData,
				}
			}
		}
		if a.ICodecCtx.(*codec.H264Ctx).Record == nil {
			err = pkg.ErrSkip
		}
	}
	return
}

// String implements pkg.IAVFrame.
func (a *AnnexB) String() string {
	return fmt.Sprintf("%d %d", a.Timestamp, a.Memory.Size)
}

// Demux implements pkg.IAVFrame.
func (a *AnnexB) Demux() (err error) {
	nalus := a.GetNalus()
	var lastFourBytes [4]byte
	var b byte
	var shallow util.Memory
	shallow.Push(a.Buffers...)
	reader := shallow.NewReader()
	gotNalu := func() {
		nalu := nalus.GetNextPointer()
		for buf := range reader.ClipFront {
			nalu.PushOne(buf)
		}
		if a.ICodecCtx == nil {
			naluType := codec.ParseH264NALUType(nalu.Buffers[0][0])
			switch naluType {
			case codec.NALU_Non_IDR_Picture,
				codec.NALU_IDR_Picture,
				codec.NALU_SEI,
				codec.NALU_SPS,
				codec.NALU_PPS,
				codec.NALU_Access_Unit_Delimiter:
				a.ICodecCtx = &codec.H264Ctx{}
			}
		}
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
			if startCode > 0 && reader.Offset() >= 3 {
				if reader.Offset() == 3 {
					startCode = 3
				}
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
	return
}

func (a *AnnexB) Mux(fromBase *pkg.Sample) (err error) {
	if a.ICodecCtx == nil {
		a.ICodecCtx = fromBase.GetBase()
	}
	a.InitRecycleIndexes(0)
	delimiter2 := codec.NALU_Delimiter2[:]
	a.PushOne(delimiter2)
	if fromBase.IDR {
		switch ctx := fromBase.GetBase().(type) {
		case *codec.H264Ctx:
			a.Push(ctx.SPS(), delimiter2, ctx.PPS(), delimiter2)
		case *codec.H265Ctx:
			a.Push(ctx.SPS(), delimiter2, ctx.PPS(), delimiter2, ctx.VPS(), delimiter2)
		}
	}
	for i, nalu := range *fromBase.Raw.(*pkg.Nalus) {
		if i > 0 {
			a.PushOne(codec.NALU_Delimiter1[:])
		}
		a.Push(nalu.Buffers...)
	}
	return
}

func (a *AnnexB) Parse(reader *pkg.AnnexBReader) (hasFrame bool, err error) {
	nalus := a.BaseSample.GetNalus()
	for !hasFrame {
		nalu := nalus.GetNextPointer()
		reader.ReadNALU(&a.Memory, nalu)
		if nalu.Size == 0 {
			nalus.Reduce()
			return
		}
		tryH264Type := codec.ParseH264NALUType(nalu.Buffers[0][0])
		h265Type := codec.ParseH265NALUType(nalu.Buffers[0][0])
		if a.ICodecCtx == nil {
			a.ICodecCtx = &codec.H26XCtx{}
		}
		switch ctx := a.ICodecCtx.(type) {
		case *codec.H26XCtx:
			if tryH264Type == codec.NALU_SPS {
				ctx.SPS = nalu.ToBytes()
				nalus.Reduce()
				a.Recycle()
			} else if tryH264Type == codec.NALU_PPS {
				ctx.PPS = nalu.ToBytes()
				nalus.Reduce()
				a.Recycle()
			} else if h265Type == h265parser.NAL_UNIT_VPS {
				ctx.VPS = nalu.ToBytes()
				nalus.Reduce()
				a.Recycle()
			} else if h265Type == h265parser.NAL_UNIT_SPS {
				ctx.SPS = nalu.ToBytes()
				nalus.Reduce()
				a.Recycle()
			} else if h265Type == h265parser.NAL_UNIT_PPS {
				ctx.PPS = nalu.ToBytes()
				nalus.Reduce()
				a.Recycle()
			} else {
				if ctx.SPS != nil && ctx.PPS != nil && tryH264Type == codec.NALU_IDR_Picture {
					var codecData h264parser.CodecData
					codecData, err = h264parser.NewCodecDataFromSPSAndPPS(ctx.SPS, ctx.PPS)
					if err != nil {
						return
					}
					a.ICodecCtx = &codec.H264Ctx{
						CodecData: codecData,
					}
					*nalus = slices.Insert(*nalus, 0, util.NewMemory(ctx.SPS), util.NewMemory(ctx.PPS))
					delimiter2 := codec.NALU_Delimiter2[:]
					a.Buffers = slices.Insert(a.Buffers, 0, delimiter2, ctx.SPS, delimiter2, ctx.PPS)
					a.Size += 8 + len(ctx.SPS) + len(ctx.PPS)
				} else if ctx.VPS != nil && ctx.SPS != nil && ctx.PPS != nil && h265Type == h265parser.NAL_UNIT_CODED_SLICE_IDR_W_RADL {
					var codecData h265parser.CodecData
					codecData, err = h265parser.NewCodecDataFromVPSAndSPSAndPPS(ctx.VPS, ctx.SPS, ctx.PPS)
					if err != nil {
						return
					}
					a.ICodecCtx = &codec.H265Ctx{
						CodecData: codecData,
					}
					*nalus = slices.Insert(*nalus, 0, util.NewMemory(ctx.VPS), util.NewMemory(ctx.SPS), util.NewMemory(ctx.PPS))
					delimiter2 := codec.NALU_Delimiter2[:]
					a.Buffers = slices.Insert(a.Buffers, 0, delimiter2, ctx.VPS, delimiter2, ctx.SPS, delimiter2, ctx.PPS)
					a.Size += 24 + len(ctx.VPS) + len(ctx.SPS) + len(ctx.PPS)
				} else {
					nalus.Reduce()
					a.Recycle()
				}
			}
		case *codec.H264Ctx:
			switch tryH264Type {
			case codec.NALU_IDR_Picture:
				a.IDR = true
				hasFrame = true
			case codec.NALU_Non_IDR_Picture:
				a.IDR = false
				hasFrame = true
			}
		case *codec.H265Ctx:
			switch h265Type {
			case h265parser.NAL_UNIT_CODED_SLICE_BLA_W_LP,
				h265parser.NAL_UNIT_CODED_SLICE_BLA_W_RADL,
				h265parser.NAL_UNIT_CODED_SLICE_BLA_N_LP,
				h265parser.NAL_UNIT_CODED_SLICE_IDR_W_RADL,
				h265parser.NAL_UNIT_CODED_SLICE_IDR_N_LP,
				h265parser.NAL_UNIT_CODED_SLICE_CRA:
				a.IDR = true
				hasFrame = true
			case h265parser.NAL_UNIT_CODED_SLICE_TRAIL_N,
				h265parser.NAL_UNIT_CODED_SLICE_TRAIL_R,
				h265parser.NAL_UNIT_CODED_SLICE_TSA_N,
				h265parser.NAL_UNIT_CODED_SLICE_TSA_R,
				h265parser.NAL_UNIT_CODED_SLICE_STSA_N,
				h265parser.NAL_UNIT_CODED_SLICE_STSA_R,
				h265parser.NAL_UNIT_CODED_SLICE_RADL_N,
				h265parser.NAL_UNIT_CODED_SLICE_RADL_R,
				h265parser.NAL_UNIT_CODED_SLICE_RASL_N,
				h265parser.NAL_UNIT_CODED_SLICE_RASL_R:
				a.IDR = false
				hasFrame = true
			}
		}
	}
	return
}
