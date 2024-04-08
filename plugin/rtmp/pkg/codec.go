package rtmp

import (
	"bytes"
	"encoding/binary"
	"errors"

	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
)

type (
	AudioCodecID byte
	VideoCodecID byte

	H264Ctx struct {
		SequenceFrame *RTMPVideo
		codec.SPSInfo
		NalulenSize int
		SPS         []byte
		PPS         []byte
	}

	H265Ctx struct {
		H264Ctx
		VPS []byte
	}

	G711Ctx struct {
		SampleRate int
		Channels   int
		SampleSize int
	}

	AACCtx struct {
		G711Ctx
		AudioSpecificConfig
		SequenceFrame *RTMPAudio
	}

	GASpecificConfig struct {
		FrameLengthFlag    byte // 1 bit
		DependsOnCoreCoder byte // 1 bit
		ExtensionFlag      byte // 1 bit
	}

	AudioSpecificConfig struct {
		AudioObjectType        byte // 5 bits
		SamplingFrequencyIndex byte // 4 bits
		ChannelConfiguration   byte // 4 bits
		GASpecificConfig
	}
	AVCDecoderConfigurationRecord struct {
		ConfigurationVersion       byte // 8 bits Version
		AVCProfileIndication       byte // 8 bits
		ProfileCompatibility       byte // 8 bits
		AVCLevelIndication         byte // 8 bits
		Reserved1                  byte // 6 bits
		LengthSizeMinusOne         byte // 2 bits 非常重要,每个NALU包前面都(lengthSizeMinusOne & 3)+1个字节的NAL包长度描述
		Reserved2                  byte // 3 bits
		NumOfSequenceParameterSets byte // 5 bits SPS 的个数,计算方法是 numOfSequenceParameterSets & 0x1F
		NumOfPictureParameterSets  byte // 8 bits PPS 的个数

		SequenceParameterSetLength  uint16 // 16 byte SPS Length
		SequenceParameterSetNALUnit []byte // n byte  SPS
		PictureParameterSetLength   uint16 // 16 byte PPS Length
		PictureParameterSetNALUnit  []byte // n byte  PPS
	}
)

const (
	ADTS_HEADER_SIZE              = 7
	CodecID_AAC      AudioCodecID = 0xA
	CodecID_PCMA     AudioCodecID = 7
	CodecID_PCMU     AudioCodecID = 8
	CodecID_OPUS     AudioCodecID = 0xC
	CodecID_H264     VideoCodecID = 7
	CodecID_H265     VideoCodecID = 0xC
	CodecID_AV1      VideoCodecID = 0xD
)

func (codecId AudioCodecID) String() string {
	switch codecId {
	case CodecID_AAC:
		return "aac"
	case CodecID_PCMA:
		return "pcma"
	case CodecID_PCMU:
		return "pcmu"
	case CodecID_OPUS:
		return "opus"
	}
	return "unknow"
}

func ParseAudioCodec(name string) AudioCodecID {
	switch name {
	case "aac":
		return CodecID_AAC
	case "pcma":
		return CodecID_PCMA
	case "pcmu":
		return CodecID_PCMU
	case "opus":
		return CodecID_OPUS
	}
	return 0
}

func (codecId VideoCodecID) String() string {
	switch codecId {
	case CodecID_H264:
		return "h264"
	case CodecID_H265:
		return "h265"
	case CodecID_AV1:
		return "av1"
	}
	return "unknow"
}

func ParseVideoCodec(name string) VideoCodecID {
	switch name {
	case "h264":
		return CodecID_H264
	case "h265":
		return CodecID_H265
	case "av1":
		return CodecID_AV1
	}
	return 0
}

func (p *AVCDecoderConfigurationRecord) Marshal(b []byte) (n int) {
	b[0] = 1
	b[1] = p.AVCProfileIndication
	b[2] = p.ProfileCompatibility
	b[3] = p.AVCLevelIndication
	b[4] = p.LengthSizeMinusOne | 0xfc
	b[5] = uint8(1) | 0xe0
	n += 6

	binary.BigEndian.PutUint16(b[n:], p.SequenceParameterSetLength)
	n += 2
	copy(b[n:], p.SequenceParameterSetNALUnit)
	n += len(p.SequenceParameterSetNALUnit)
	b[n] = uint8(1)
	n++
	binary.BigEndian.PutUint16(b[n:], p.PictureParameterSetLength)
	n += 2
	copy(b[n:], p.PictureParameterSetNALUnit)
	n += len(p.PictureParameterSetNALUnit)

	return
}

var ErrDecconfInvalid = errors.New("decode error")

func (p *AVCDecoderConfigurationRecord) Unmarshal(b *util.Buffers) (err error) {
	if b.Length < 7 {
		err = errors.New("not enough len")
		return
	}
	b.ReadByteTo(&p.ConfigurationVersion, &p.AVCProfileIndication, &p.ProfileCompatibility, &p.AVCLevelIndication, &p.LengthSizeMinusOne)
	p.LengthSizeMinusOne = p.LengthSizeMinusOne & 0x03
	p.NumOfSequenceParameterSets, err = b.ReadByteMask(0x1f)
	if err != nil {
		return
	}
	var sps, pps [][]byte
	for range p.NumOfSequenceParameterSets {
		spslen, err1 := b.ReadBE(2)
		if err1 != nil {
			return err1
		}
		spsbytes, err2 := b.ReadBytes(spslen)
		if err2 != nil {
			return err2
		}
		sps = append(sps, spsbytes)
	}
	p.SequenceParameterSetLength = uint16(len(sps[0]))
	p.SequenceParameterSetNALUnit = sps[0]
	if b.Length < 1 {
		err = ErrDecconfInvalid
		return
	}

	ppscount, err1 := b.ReadByte()
	if err1 != nil {
		return err1
	}
	for range ppscount {
		ppslen, err1 := b.ReadBE(2)
		if err1 != nil {
			return err1
		}
		ppsbytes, err2 := b.ReadBytes(ppslen)
		if err2 != nil {
			return err2
		}
		pps = append(pps, ppsbytes)
	}
	if ppscount >= 1 {
		p.PictureParameterSetLength = uint16(len(pps[0]))
		p.PictureParameterSetNALUnit = pps[0]
	} else {
		err = ErrDecconfInvalid
	}
	return
}

func ParseSPS(data []byte) (self codec.SPSInfo, err error) {
	r := &util.GolombBitReader{R: bytes.NewReader(data)}

	if _, err = r.ReadBits(8); err != nil {
		return
	}

	if self.ProfileIdc, err = r.ReadBits(8); err != nil {
		return
	}

	// constraint_set0_flag-constraint_set6_flag,reserved_zero_2bits
	if _, err = r.ReadBits(8); err != nil {
		return
	}

	// level_idc
	if self.LevelIdc, err = r.ReadBits(8); err != nil {
		return
	}

	// seq_parameter_set_id
	if _, err = r.ReadExponentialGolombCode(); err != nil {
		return
	}

	if self.ProfileIdc == 100 || self.ProfileIdc == 110 ||
		self.ProfileIdc == 122 || self.ProfileIdc == 244 ||
		self.ProfileIdc == 44 || self.ProfileIdc == 83 ||
		self.ProfileIdc == 86 || self.ProfileIdc == 118 {

		var chroma_format_idc uint
		if chroma_format_idc, err = r.ReadExponentialGolombCode(); err != nil {
			return
		}

		if chroma_format_idc == 3 {
			// residual_colour_transform_flag
			if _, err = r.ReadBit(); err != nil {
				return
			}
		}

		// bit_depth_luma_minus8
		if _, err = r.ReadExponentialGolombCode(); err != nil {
			return
		}
		// bit_depth_chroma_minus8
		if _, err = r.ReadExponentialGolombCode(); err != nil {
			return
		}
		// qpprime_y_zero_transform_bypass_flag
		if _, err = r.ReadBit(); err != nil {
			return
		}

		var seq_scaling_matrix_present_flag uint
		if seq_scaling_matrix_present_flag, err = r.ReadBit(); err != nil {
			return
		}

		if seq_scaling_matrix_present_flag != 0 {
			for i := 0; i < 8; i++ {
				var seq_scaling_list_present_flag uint
				if seq_scaling_list_present_flag, err = r.ReadBit(); err != nil {
					return
				}
				if seq_scaling_list_present_flag != 0 {
					var sizeOfScalingList uint
					if i < 6 {
						sizeOfScalingList = 16
					} else {
						sizeOfScalingList = 64
					}
					lastScale := uint(8)
					nextScale := uint(8)
					for j := uint(0); j < sizeOfScalingList; j++ {
						if nextScale != 0 {
							var delta_scale uint
							if delta_scale, err = r.ReadSE(); err != nil {
								return
							}
							nextScale = (lastScale + delta_scale + 256) % 256
						}
						if nextScale != 0 {
							lastScale = nextScale
						}
					}
				}
			}
		}
	}

	// log2_max_frame_num_minus4
	if _, err = r.ReadExponentialGolombCode(); err != nil {
		return
	}

	var pic_order_cnt_type uint
	if pic_order_cnt_type, err = r.ReadExponentialGolombCode(); err != nil {
		return
	}
	if pic_order_cnt_type == 0 {
		// log2_max_pic_order_cnt_lsb_minus4
		if _, err = r.ReadExponentialGolombCode(); err != nil {
			return
		}
	} else if pic_order_cnt_type == 1 {
		// delta_pic_order_always_zero_flag
		if _, err = r.ReadBit(); err != nil {
			return
		}
		// offset_for_non_ref_pic
		if _, err = r.ReadSE(); err != nil {
			return
		}
		// offset_for_top_to_bottom_field
		if _, err = r.ReadSE(); err != nil {
			return
		}
		var num_ref_frames_in_pic_order_cnt_cycle uint
		if num_ref_frames_in_pic_order_cnt_cycle, err = r.ReadExponentialGolombCode(); err != nil {
			return
		}
		for i := uint(0); i < num_ref_frames_in_pic_order_cnt_cycle; i++ {
			if _, err = r.ReadSE(); err != nil {
				return
			}
		}
	}

	// max_num_ref_frames
	if _, err = r.ReadExponentialGolombCode(); err != nil {
		return
	}

	// gaps_in_frame_num_value_allowed_flag
	if _, err = r.ReadBit(); err != nil {
		return
	}

	if self.MbWidth, err = r.ReadExponentialGolombCode(); err != nil {
		return
	}
	self.MbWidth++

	if self.MbHeight, err = r.ReadExponentialGolombCode(); err != nil {
		return
	}
	self.MbHeight++

	var frame_mbs_only_flag uint
	if frame_mbs_only_flag, err = r.ReadBit(); err != nil {
		return
	}
	if frame_mbs_only_flag == 0 {
		// mb_adaptive_frame_field_flag
		if _, err = r.ReadBit(); err != nil {
			return
		}
	}

	// direct_8x8_inference_flag
	if _, err = r.ReadBit(); err != nil {
		return
	}

	var frame_cropping_flag uint
	if frame_cropping_flag, err = r.ReadBit(); err != nil {
		return
	}
	if frame_cropping_flag != 0 {
		if self.CropLeft, err = r.ReadExponentialGolombCode(); err != nil {
			return
		}
		if self.CropRight, err = r.ReadExponentialGolombCode(); err != nil {
			return
		}
		if self.CropTop, err = r.ReadExponentialGolombCode(); err != nil {
			return
		}
		if self.CropBottom, err = r.ReadExponentialGolombCode(); err != nil {
			return
		}
	}

	self.Width = (self.MbWidth * 16) - self.CropLeft*2 - self.CropRight*2
	self.Height = ((2 - frame_mbs_only_flag) * self.MbHeight * 16) - self.CropTop*2 - self.CropBottom*2

	return
}

// func ParseHevcSPS(data []byte) (self codec.SPSInfo, err error) {
// 	var rawsps hevc.H265RawSPS
// 	if err = rawsps.Decode(data); err == nil {
// 		self.CropLeft, self.CropRight, self.CropTop, self.CropBottom = uint(rawsps.Conf_win_left_offset), uint(rawsps.Conf_win_right_offset), uint(rawsps.Conf_win_top_offset), uint(rawsps.Conf_win_bottom_offset)
// 		self.Width = uint(rawsps.Pic_width_in_luma_samples)
// 		self.Height = uint(rawsps.Pic_height_in_luma_samples)
// 	}
// 	return
// }

var SamplingFrequencies = [...]int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350, 0, 0, 0}

func (ctx *H264Ctx) GetSequenceFrame() IAVFrame {
	return ctx.SequenceFrame
}

func (ctx *H264Ctx) GetWidth() int {
	return int(ctx.SPSInfo.Width)
}

func (ctx *H264Ctx) GetHeight() int {
	return int(ctx.SPSInfo.Height)
}

func (ctx *G711Ctx) GetSampleRate() int {
	return ctx.SampleRate
}

func (ctx *G711Ctx) GetChannels() int {
	return ctx.Channels
}

func (ctx *G711Ctx) GetSampleSize() int {
	return ctx.SampleSize
}

func (ctx *G711Ctx) GetSequenceFrame() IAVFrame {
	return nil
}

func (ctx *AACCtx) GetSequenceFrame() IAVFrame {
	return ctx.SequenceFrame
}
