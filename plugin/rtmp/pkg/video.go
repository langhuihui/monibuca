package rtmp

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"time"

	"github.com/deepch/vdk/codec/h264parser"

	. "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
)

type VideoFrame RTMPData

// 过滤掉异常的 NALU
func (avcc *VideoFrame) filterH264(naluSizeLen int) {
	reader := avcc.NewReader()
	lenReader := reader.NewReader()
	reader.Skip(5)
	var afterFilter util.Memory
	lenReader.RangeN(5, afterFilter.PushOne)
	allocator := avcc.GetAllocator()
	var hasBadNalu bool
	for {
		naluLen, err := reader.ReadBE(naluSizeLen)
		if err != nil {
			break
		}
		var lenBuffer net.Buffers
		lenReader.RangeN(naluSizeLen, func(b []byte) {
			lenBuffer = append(lenBuffer, b)
		})
		lenReader.Skip(int(naluLen))
		var naluBuffer net.Buffers
		reader.RangeN(int(naluLen), func(b []byte) {
			naluBuffer = append(naluBuffer, b)
		})
		badType := codec.ParseH264NALUType(naluBuffer[0][0])
		// 替换之前打印 badType 的逻辑，解码并打印 SliceType
		if badType == 5 { // NALU type for Coded slice of a non-IDR picture or Coded slice of an IDR picture
			naluData := bytes.Join(naluBuffer, nil) // bytes 包已导入
			if len(naluData) > 0 {
				// h264parser 包已导入 as "github.com/deepch/vdk/codec/h264parser"
				// ParseSliceHeaderFromNALU 返回的第一个值就是 SliceType
				sliceType, err := h264parser.ParseSliceHeaderFromNALU(naluData)
				if err == nil {
					println("Decoded SliceType:", sliceType.String())
				} else {
					println("Error parsing H.264 slice header:", err.Error())
				}
			} else {
				println("NALU data is empty, cannot parse H.264 slice header.")
			}
		}

		switch badType {
		case 5, 6, 7, 8, 1, 2, 3, 4:
			afterFilter.Push(lenBuffer...)
			afterFilter.Push(naluBuffer...)
		default:
			hasBadNalu = true
			if allocator != nil {
				for _, nalu := range lenBuffer {
					allocator.Free(nalu)
				}
				for _, nalu := range naluBuffer {
					allocator.Free(nalu)
				}
			}
		}
	}
	if hasBadNalu {
		avcc.Memory = afterFilter
	}
}

func (avcc *VideoFrame) filterH265(naluSizeLen int) {
	//TODO
}

func (avcc *VideoFrame) CheckCodecChange() (err error) {
	old := avcc.ICodecCtx
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
	avcc.IDR = b0&0b0111_0000>>4 == 1
	packetType := b0 & 0b1111
	codecId := VideoCodecID(b0 & 0x0F)
	var fourCC codec.FourCC
	parseSequence := func() (err error) {
		avcc.IDR = false
		switch fourCC {
		case codec.FourCC_H264:
			if old != nil && avcc.Memory.Equal(&old.(*H264Ctx).SequenceFrame.Memory) {
				avcc.ICodecCtx = old
				break
			}
			newCtx := &H264Ctx{}
			newCtx.SequenceFrame.CopyFrom(&avcc.Memory)
			newCtx.SequenceFrame.BaseSample = &BaseSample{}
			newCtx.H264Ctx, err = codec.NewH264CtxFromRecord(newCtx.SequenceFrame.Buffers[0][reader.Offset():])
			if err == nil {
				avcc.ICodecCtx = newCtx
			} else {
				return
			}
		case codec.FourCC_H265:
			if old != nil && avcc.Memory.Equal(&old.(*H265Ctx).SequenceFrame.Memory) {
				avcc.ICodecCtx = old
				break
			}
			newCtx := H265Ctx{
				Enhanced: enhanced,
			}
			newCtx.SequenceFrame.CopyFrom(&avcc.Memory)
			newCtx.SequenceFrame.BaseSample = &BaseSample{}
			newCtx.H265Ctx, err = codec.NewH265CtxFromRecord(newCtx.SequenceFrame.Buffers[0][reader.Offset():])
			if err == nil {
				avcc.ICodecCtx = newCtx
			} else {
				return
			}
		case codec.FourCC_AV1:
			var newCtx AV1Ctx
			if err = newCtx.Unmarshal(&reader); err == nil {
				avcc.ICodecCtx = &newCtx
			} else {
				return
			}
		}
		return ErrSkip
	}
	if enhanced {
		reader.Read(fourCC[:])
		switch packetType {
		case PacketTypeSequenceStart:
			err = parseSequence()
			return
		case PacketTypeCodedFrames:
			switch old.(type) {
			case *H265Ctx:
				var cts uint32
				if cts, err = reader.ReadBE(3); err != nil {
					return err
				}
				avcc.CTS = time.Duration(cts) * time.Millisecond
				// avcc.filterH265(int(ctx.RecordInfo.LengthSizeMinusOne) + 1)
			case *AV1Ctx:
				// return avcc.parseAV1(reader)
			}
		case PacketTypeCodedFramesX:
			// avcc.filterH265(int(old.(*H265Ctx).RecordInfo.LengthSizeMinusOne) + 1)
		}
	} else {
		b0, err = reader.ReadByte() //sequence frame flag
		if err != nil {
			return
		}
		if codecId == CodecID_H265 {
			fourCC = codec.FourCC_H265
		} else {
			fourCC = codec.FourCC_H264
		}
		var cts uint32
		cts, err = reader.ReadBE(3)
		if err != nil {
			return
		}
		avcc.CTS = time.Duration(cts) * time.Millisecond
		if b0 == 0 {
			if err = parseSequence(); err != nil {
				return
			}
		} else {
			// switch ctx := old.(type) {
			// case *codec.H264Ctx:
			// 	avcc.filterH264(int(ctx.RecordInfo.LengthSizeMinusOne) + 1)
			// case *H265Ctx:
			// 	avcc.filterH265(int(ctx.RecordInfo.LengthSizeMinusOne) + 1)
			// }
			// if avcc.Size <= 5 {
			// 	return old, ErrSkip
			// }
		}
	}
	return
}

func (avcc *VideoFrame) parseH264(ctx *H264Ctx, reader *util.MemoryReader) (err error) {
	return avcc.ParseAVCC(reader, int(ctx.RecordInfo.LengthSizeMinusOne)+1)
}

func (avcc *VideoFrame) parseH265(ctx *H265Ctx, reader *util.MemoryReader) (err error) {
	return avcc.ParseAVCC(reader, int(ctx.RecordInfo.LengthSizeMinusOne)+1)
}

func (avcc *VideoFrame) parseAV1(reader *util.MemoryReader) error {
	var obus OBUs
	if err := obus.ParseAVCC(reader); err != nil {
		return err
	}
	avcc.Raw = &obus
	return nil
}

func (avcc *VideoFrame) Demux() error {
	reader := avcc.NewReader()
	b0, err := reader.ReadByte()
	if err != nil {
		return err
	}

	enhanced := b0&0b1000_0000 != 0 // https://veovera.github.io/enhanced-rtmp/docs/enhanced/enhanced-rtmp-v1.pdf
	// frameType := b0 & 0b0111_0000 >> 4
	packetType := b0 & 0b1111

	if enhanced {
		err = reader.Skip(4) // fourcc
		if err != nil {
			return err
		}
		switch packetType {
		case PacketTypeSequenceStart:
			// see Parse()
			return nil
		case PacketTypeCodedFrames:
			switch ctx := avcc.ICodecCtx.(type) {
			case *H265Ctx:
				var cts uint32
				if cts, err = reader.ReadBE(3); err != nil {
					return err
				}
				avcc.CTS = time.Duration(cts) * time.Millisecond
				err = avcc.parseH265(ctx, &reader)
			case *AV1Ctx:
				err = avcc.parseAV1(&reader)
			}
		case PacketTypeCodedFramesX: // no cts
			err = avcc.parseH265(avcc.ICodecCtx.(*H265Ctx), &reader)
		}
		return err
	} else {
		b0, err = reader.ReadByte() //sequence frame flag
		if err != nil {
			return err
		}
		var cts uint32
		if cts, err = reader.ReadBE(3); err != nil {
			return err
		}
		avcc.SetCTS32(cts)
		switch ctx := avcc.ICodecCtx.(type) {
		case *H265Ctx:
			if b0 == 0 {
				// nalus.Append(ctx.VPS())
				// nalus.Append(ctx.SPS())
				// nalus.Append(ctx.PPS())
			} else {
				err = avcc.parseH265(ctx, &reader)
				return err
			}

		case *H264Ctx:
			if b0 == 0 {
				// nalus.Append(ctx.SPS())
				// nalus.Append(ctx.PPS())
			} else {
				err = avcc.parseH264(ctx, &reader)
				return err
			}
		}
		return err
	}
}

func (avcc *VideoFrame) muxOld26x(codecID VideoCodecID, fromBase *Sample) {
	nalus := fromBase.Raw.(*Nalus)
	avcc.InitRecycleIndexes(len(*nalus)) // Recycle partial data
	head := avcc.NextN(5)
	head[0] = util.Conditional[byte](fromBase.IDR, 0x10, 0x20) | byte(codecID)
	head[1] = 1
	util.PutBE(head[2:5], fromBase.CTS/time.Millisecond) // cts
	for nalu := range nalus.RangePoint {
		naluLenM := avcc.NextN(4)
		naluLen := uint32(nalu.Size)
		binary.BigEndian.PutUint32(naluLenM, naluLen)
		avcc.Push(nalu.Buffers...)
	}
}

func (avcc *VideoFrame) Mux(fromBase *Sample) (err error) {
	switch c := fromBase.GetBase().(type) {
	case *AV1Ctx:
		panic(c)
	case *codec.H264Ctx:
		if avcc.ICodecCtx == nil {
			ctx := &H264Ctx{H264Ctx: c}
			ctx.SequenceFrame.PushOne(append([]byte{0x17, 0, 0, 0, 0}, c.Record...))
			ctx.SequenceFrame.BaseSample = &BaseSample{}
			avcc.ICodecCtx = ctx
		}
		avcc.muxOld26x(CodecID_H264, fromBase)
	case *codec.H265Ctx:
		if true {
			if avcc.ICodecCtx == nil {
				ctx := &H265Ctx{H265Ctx: c, Enhanced: true}
				b := make(util.Buffer, len(ctx.Record)+5)
				if ctx.Enhanced {
					b[0] = 0b1001_0000 | byte(PacketTypeSequenceStart)
					copy(b[1:], codec.FourCC_H265[:])
				} else {
					b[0], b[1], b[2], b[3], b[4] = 0x1C, 0, 0, 0, 0
				}
				copy(b[5:], ctx.Record)
				ctx.SequenceFrame.PushOne(b)
				ctx.SequenceFrame.BaseSample = &BaseSample{}
				avcc.ICodecCtx = ctx
			}
			nalus := fromBase.Raw.(*Nalus)
			avcc.InitRecycleIndexes(nalus.Count()) // Recycle partial data
			head := avcc.NextN(8)
			if fromBase.IDR {
				head[0] = 0b1001_0000 | byte(PacketTypeCodedFrames)
			} else {
				head[0] = 0b1010_0000 | byte(PacketTypeCodedFrames)
			}
			copy(head[1:], codec.FourCC_H265[:])
			util.PutBE(head[5:8], fromBase.CTS/time.Millisecond) // cts
			for nalu := range nalus.RangePoint {
				naluLenM := avcc.NextN(4)
				naluLen := uint32(nalu.Size)
				binary.BigEndian.PutUint32(naluLenM, naluLen)
				avcc.Push(nalu.Buffers...)
			}
		} else {
			avcc.muxOld26x(CodecID_H265, fromBase)
		}
	}
	return
}
