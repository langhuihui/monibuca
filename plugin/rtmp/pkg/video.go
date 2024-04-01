package rtmp

import (
	"time"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
)

type RTMPVideo struct {
	RTMPData
}

func (avcc *RTMPVideo) IsIDR() bool {
	return avcc.Buffers.Buffers[0][0]&0b1111_0000>>4 == 1
}

func (avcc *RTMPVideo) DecodeConfig(track *AVTrack) error {
	reader := avcc.Buffers
	b0, err := reader.ReadByte()
	if err != nil {
		return err
	}
	enhanced := b0&0b1000_0000 != 0 // https://veovera.github.io/enhanced-rtmp/docs/enhanced/enhanced-rtmp-v1.pdf
	// frameType := b0 & 0b0111_0000 >> 4
	packetType := b0 & 0b1111

	parseSequence := func() (err error) {
		switch track.Codec {
		case "h264":
			var ctx H264Ctx
			var info AVCDecoderConfigurationRecord
			if err = info.Unmarshal(&reader); err == nil {
				ctx.SPSInfo, _ = ParseSPS(info.SequenceParameterSetNALUnit)
				ctx.NalulenSize = int(info.LengthSizeMinusOne&3 + 1)
				ctx.SPS = info.SequenceParameterSetNALUnit
				ctx.PPS = info.PictureParameterSetNALUnit
				ctx.SequenceFrame = &RTMPVideo{}
				ctx.SequenceFrame.ReadFromBytes(avcc.ToBytes())
				track.ICodecCtx = &ctx
			}
		case "h265":
			// var ctx H265Ctx
		case "av1":
		}
		return
	}
	if enhanced {
		var fourCC [4]byte
		_, err = reader.Read(fourCC[:])
		if err != nil {
			return err
		}
		switch fourCC {
		case FourCC_H265:
			track.Codec = "h265"
		case FourCC_AV1:
			track.Codec = "av1"
		}
		switch packetType {
		case PacketTypeSequenceStart:
			if err = parseSequence(); err != nil {
				return err
			}
			return nil
		case PacketTypeCodedFrames:

		case PacketTypeCodedFramesX:
		}
	} else {
		b0, err = reader.ReadByte() //sequence frame flag
		if err != nil {
			return err
		}
		if codec.VideoCodecID(b0&0x0F) == codec.CodecID_H265 {
			track.Codec = "h265"
		} else {
			track.Codec = "h264"
		}
		_, err = reader.ReadBE(3) // cts == 0
		if err != nil {
			return err
		}
		if b0 == 0 {
			if err = parseSequence(); err != nil {
				return err
			}
		} else {
		}
	}
	return nil
}

func (avcc *RTMPVideo) parseH264(track *AVTrack, reader *util.Buffers, cts uint32) (any, error) {
	var nalus Nalus
	ctx := track.ICodecCtx.(*H264Ctx)
	nalus.PTS = time.Duration(avcc.Timestamp+uint32(cts)) * 90
	nalus.DTS = time.Duration(avcc.Timestamp) * 90
	if err := nalus.ParseAVCC(reader, ctx.NalulenSize); err != nil {
		return nalus, err
	}
	return nalus, nil
}

func (avcc *RTMPVideo) parseH265(track *AVTrack, reader *util.Buffers, cts uint32) (any, error) {
	var nalus Nalus
	ctx := track.ICodecCtx.(*H265Ctx)
	nalus.PTS = time.Duration(avcc.Timestamp+uint32(cts)) * 90
	nalus.DTS = time.Duration(avcc.Timestamp) * 90
	if err := nalus.ParseAVCC(reader, ctx.NalulenSize); err != nil {
		return nalus, err
	}
	return nalus, nil
}

func (avcc *RTMPVideo) parseAV1(track *AVTrack, reader *util.Buffers) (any, error) {
	var obus OBUs
	obus.PTS = time.Duration(avcc.Timestamp) * 90
	if err := obus.ParseAVCC(reader); err != nil {
		return obus, err
	}
	return obus, nil
}

func (avcc *RTMPVideo) ToRaw(track *AVTrack) (any, error) {
	reader := avcc.Buffers
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
			if err = avcc.DecodeConfig(track); err != nil {
				return nil, err
			}
			return nil, nil
		case PacketTypeCodedFrames:
			if track.Codec == "h265" {
				cts, err := reader.ReadBE(3) //cts, only h265
				if err != nil {
					return nil, err
				}
				return avcc.parseH265(track, &reader, uint32(cts))
			} else {
				return avcc.parseAV1(track, &reader)
			}
		case PacketTypeCodedFramesX:
		}
	} else {
		b0, err = reader.ReadByte() //sequence frame flag
		if err != nil {
			return nil, err
		}
		cts, err := reader.ReadBE(3)
		if err != nil {
			return nil, err
		}
		if b0 == 0 {
			if err = avcc.DecodeConfig(track); err != nil {
				return nil, err
			}
		} else {
			if track.Codec == "h265" {
				return avcc.parseH265(track, &reader, uint32(cts))
			} else {
				return avcc.parseH264(track, &reader, uint32(cts))
			}
		}
	}
	return nil, nil
}

func (avcc *RTMPVideo) FromRaw(track *AVTrack, raw any) error {
	return nil
}
