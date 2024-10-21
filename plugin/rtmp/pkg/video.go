package rtmp

import (
	"encoding/binary"
	"io"
	"time"

	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/codec/h265parser"

	. "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
)

var _ IAVFrame = (*RTMPVideo)(nil)

type RTMPVideo struct {
	RTMPData
	CTS uint32
}

func (avcc *RTMPVideo) GetCTS() time.Duration {
	return time.Duration(avcc.CTS) * time.Millisecond
}

func (avcc *RTMPVideo) Parse(t *AVTrack) (err error) {
	if avcc.Size <= 10 {
		err = io.ErrShortBuffer
		return
	}
	reader := avcc.NewReader()
	var b0 byte
	b0, err = reader.ReadByte()
	if err != nil {
		return
	}
	enhanced := b0&0b1000_0000 != 0 // https://veovera.github.io/enhanced-rtmp/docs/enhanced/enhanced-rtmp-v1.pdf
	t.Value.IDR = b0&0b0111_0000>>4 == 1
	packetType := b0 & 0b1111
	var fourCC codec.FourCC
	parseSequence := func() (err error) {
		t.Value.IDR = false
		var cloneFrame RTMPVideo
		cloneFrame.CopyFrom(&avcc.Memory)
		switch fourCC {
		case codec.FourCC_H264:
			var ctx codec.H264Ctx
			ctx.Record = cloneFrame.Buffers[0][reader.Offset():]
			if _, err = ctx.RecordInfo.Unmarshal(ctx.Record); err == nil {
				t.SequenceFrame = &cloneFrame
				t.ICodecCtx = &ctx
				ctx.SPSInfo, err = h264parser.ParseSPS(ctx.SPS())
			}
		case codec.FourCC_H265:
			var ctx H265Ctx
			ctx.Enhanced = enhanced
			ctx.Record = cloneFrame.Buffers[0][reader.Offset():]
			if _, err = ctx.RecordInfo.Unmarshal(ctx.Record); err == nil {
				ctx.RecordInfo.LengthSizeMinusOne = 3 // Unmarshal wrong LengthSizeMinusOne
				t.SequenceFrame = &cloneFrame
				t.ICodecCtx = &ctx
				ctx.SPSInfo, err = h265parser.ParseSPS(ctx.SPS())
			}
		case codec.FourCC_AV1:
			var ctx AV1Ctx
			if err = ctx.Unmarshal(reader); err == nil {
				t.SequenceFrame = &cloneFrame
				t.ICodecCtx = &ctx
			}
		}
		return
	}
	if enhanced {
		reader.ReadBytesTo(fourCC[:])
		switch packetType {
		case PacketTypeSequenceStart:
			err = parseSequence()
			return
		case PacketTypeCodedFrames:

		case PacketTypeCodedFramesX:
		}
	} else {
		b0, err = reader.ReadByte() //sequence frame flag
		if err != nil {
			return
		}
		if VideoCodecID(b0&0x0F) == CodecID_H265 {
			fourCC = codec.FourCC_H265
		} else {
			fourCC = codec.FourCC_H264
		}
		_, err = reader.ReadBE(3) // cts == 0
		if err != nil {
			return
		}
		if b0 == 0 {
			if err = parseSequence(); err != nil {
				return
			}
		} else {
			// var naluLen int
			// for reader.Length > 0 {
			// 	naluLen, err = reader.ReadBE(4) // naluLenM
			// 	if err != nil {
			// 		return
			// 	}
			// 	var nalus net.Buffers
			// 	n := reader.WriteNTo(naluLen, &nalus)
			// 	if n != naluLen {
			// 		err = fmt.Errorf("naluLen:%d != n:%d", naluLen, n)
			// 		return
			// 	}
			// }
		}
	}
	return
}

func (avcc *RTMPVideo) ConvertCtx(from codec.ICodecCtx) (to codec.ICodecCtx, seq IAVFrame, err error) {
	var enhanced = true //TODO
	switch fourCC := from.FourCC(); fourCC {
	case codec.FourCC_H264:
		h264ctx := from.GetBase().(*codec.H264Ctx)
		var seqFrame RTMPData
		seqFrame.AppendOne(append([]byte{0x17, 0, 0, 0, 0}, h264ctx.Record...))
		//if t.Enabled(context.TODO(), TraceLevel) {
		//	c := t.FourCC().String()
		//	size := seqFrame.GetSize()
		//	data := seqFrame.String()
		//	t.Trace("decConfig", "codec", c, "size", size, "data", data)
		//}
		return h264ctx, seqFrame.WrapVideo(), err
	case codec.FourCC_H265:
		h265ctx := from.GetBase().(*codec.H265Ctx)
		b := make(util.Buffer, len(h265ctx.Record)+5)
		if enhanced {
			b[0] = 0b1001_0000 | byte(PacketTypeSequenceStart)
			copy(b[1:], codec.FourCC_H265[:])
		} else {
			b[0], b[1], b[2], b[3], b[4] = 0x1C, 0, 0, 0, 0
		}
		copy(b[5:], h265ctx.Record)
		var ctx H265Ctx
		ctx.Enhanced = enhanced
		ctx.H265Ctx = *h265ctx
		var seqFrame RTMPData
		seqFrame.AppendOne(b)
		return &ctx, seqFrame.WrapVideo(), err
	case codec.FourCC_AV1:
	}
	return
}

func (avcc *RTMPVideo) parseH264(ctx *codec.H264Ctx, reader *util.MemoryReader) (any, error) {
	var nalus Nalus
	if err := nalus.ParseAVCC(reader, int(ctx.RecordInfo.LengthSizeMinusOne)+1); err != nil {
		return nalus, err
	}
	return nalus, nil
}

func (avcc *RTMPVideo) parseH265(ctx *H265Ctx, reader *util.MemoryReader) (any, error) {
	var nalus Nalus
	if err := nalus.ParseAVCC(reader, int(ctx.RecordInfo.LengthSizeMinusOne)+1); err != nil {
		return nalus, err
	}
	return nalus, nil
}

func (avcc *RTMPVideo) parseAV1(reader *util.MemoryReader) (any, error) {
	var obus OBUs
	if err := obus.ParseAVCC(reader); err != nil {
		return obus, err
	}
	return obus, nil
}

func (avcc *RTMPVideo) Demux(codecCtx codec.ICodecCtx) (any, error) {
	reader := avcc.NewReader()
	b0, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}

	enhanced := b0&0b1000_0000 != 0 // https://veovera.github.io/enhanced-rtmp/docs/enhanced/enhanced-rtmp-v1.pdf
	// frameType := b0 & 0b0111_0000 >> 4
	packetType := b0 & 0b1111

	if enhanced {
		err = reader.Skip(4) // fourcc
		if err != nil {
			return nil, err
		}
		switch packetType {
		case PacketTypeSequenceStart:
			// see Parse()
			return nil, nil
		case PacketTypeCodedFrames:
			switch ctx := codecCtx.(type) {
			case *H265Ctx:
				if avcc.CTS, err = reader.ReadBE(3); err != nil {
					return nil, err
				}
				return avcc.parseH265(ctx, reader)
			case *AV1Ctx:
				return avcc.parseAV1(reader)
			}
		case PacketTypeCodedFramesX: // no cts
			return avcc.parseH265(codecCtx.(*H265Ctx), reader)
		}
	} else {
		b0, err = reader.ReadByte() //sequence frame flag
		if err != nil {
			return nil, err
		}
		if avcc.CTS, err = reader.ReadBE(3); err != nil {
			return nil, err
		}
		var nalus Nalus
		switch ctx := codecCtx.(type) {
		case *H265Ctx:
			if b0 == 0 {
				nalus.Append(ctx.VPS())
				nalus.Append(ctx.SPS())
				nalus.Append(ctx.PPS())
			} else {
				return avcc.parseH265(ctx, reader)
			}

		case *codec.H264Ctx:
			if b0 == 0 {
				nalus.Append(ctx.SPS())
				nalus.Append(ctx.PPS())
			} else {
				return avcc.parseH264(ctx, reader)
			}
		}
		return nalus, nil
	}
	return nil, nil
}

func (avcc *RTMPVideo) muxOld26x(codecID VideoCodecID, from *AVFrame) {
	nalus := from.Raw.(Nalus)
	avcc.InitRecycleIndexes(len(nalus)) // Recycle partial data
	head := avcc.NextN(5)
	head[0] = util.Conditional[byte](from.IDR, 0x10, 0x20) | byte(codecID)
	head[1] = 1
	util.PutBE(head[2:5], from.CTS/time.Millisecond) // cts
	for _, nalu := range nalus {
		naluLenM := avcc.NextN(4)
		naluLen := uint32(nalu.Size)
		binary.BigEndian.PutUint32(naluLenM, naluLen)
		avcc.Append(nalu.Buffers...)
	}
}

func (avcc *RTMPVideo) Mux(codecCtx codec.ICodecCtx, from *AVFrame) {
	avcc.Timestamp = uint32(from.Timestamp / time.Millisecond)
	switch ctx := codecCtx.(type) {
	case *AV1Ctx:
		panic(ctx)
	case *codec.H264Ctx:
		avcc.muxOld26x(CodecID_H264, from)
	case *H265Ctx:
		if ctx.Enhanced {
			nalus := from.Raw.(Nalus)
			avcc.InitRecycleIndexes(len(nalus)) // Recycle partial data
			head := avcc.NextN(8)
			if from.IDR {
				head[0] = 0b1001_0000 | byte(PacketTypeCodedFrames)
			} else {
				head[0] = 0b1010_0000 | byte(PacketTypeCodedFrames)
			}
			copy(head[1:], codec.FourCC_H265[:])
			util.PutBE(head[5:8], from.CTS/time.Millisecond) // cts
			for _, nalu := range nalus {
				naluLenM := avcc.NextN(4)
				naluLen := uint32(nalu.Size)
				binary.BigEndian.PutUint32(naluLenM, naluLen)
				avcc.Append(nalu.Buffers...)
			}
		} else {
			avcc.muxOld26x(CodecID_H265, from)
		}
	}
}
