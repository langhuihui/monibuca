package rtmp

import (
	. "m7s.live/m7s/v5/pkg"
)

type RTMPAudio struct {
	RTMPData
}

func (avcc *RTMPAudio) DecodeConfig(track *AVTrack) error {
	reader := avcc.Buffers
	b0, err := reader.ReadByte()
	if err != nil {
		return err
	}
	b1, err := reader.ReadByte()
	if err != nil {
		return err
	}
	if b1 == 0 {
		switch b0 & 0b1111_0000 >> 4 {
		case 7:
			track.Codec = "pcmu"
			var ctx G711Ctx
			track.ICodecCtx = &ctx
		case 8:
			track.Codec = "pcma"
			var ctx G711Ctx
			track.ICodecCtx = &ctx
		case 10:
			track.Codec = "aac"
			var ctx AACCtx
			b0, err = reader.ReadByte()
			if err != nil {
				return err
			}
			b1, err = reader.ReadByte()
			if err != nil {
				return err
			}
			ctx.AudioObjectType = b0 >> 3
			ctx.SamplingFrequencyIndex = (b0 & 0x07 << 1) | (b1 >> 7)
			ctx.ChannelConfiguration = (b1 >> 3) & 0x0F
			ctx.FrameLengthFlag = (b1 >> 2) & 0x01
			ctx.DependsOnCoreCoder = (b1 >> 1) & 0x01
			ctx.ExtensionFlag = b1 & 0x01
			ctx.SequenceFrame = &RTMPAudio{}
			ctx.SequenceFrame.ReadFromBytes(avcc.ToBytes())
			track.ICodecCtx = &ctx
		}
	}
	return nil
}

func (avcc *RTMPAudio) ToRaw(track *AVTrack) (any, error) {
	reader := avcc.Buffers
	if track.Codec == "aac" {
		err := reader.Skip(2)
		return reader.Buffers, err
	} else {
		err := reader.Skip(1)
		return reader.Buffers, err
	}
}

func (avcc *RTMPAudio) FromRaw(track *AVTrack, raw any) error {
	return nil
}
