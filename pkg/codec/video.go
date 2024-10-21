package codec

import (
	"bytes"
	"encoding/binary"

	"m7s.live/v5/pkg/util"
)

type FourCC [4]byte

var (
	FourCC_H264 = FourCC{'a', 'v', 'c', '1'}
	FourCC_H265 = FourCC{'h', 'v', 'c', '1'}
	FourCC_AV1  = FourCC{'a', 'v', '0', '1'}
	FourCC_VP9  = FourCC{'v', 'p', '0', '9'}
	FourCC_VP8  = FourCC{'v', 'p', '8', '0'}
	FourCC_MP4A = FourCC{'m', 'p', '4', 'a'}
	FourCC_OPUS = FourCC{'O', 'p', 'u', 's'}
	FourCC_ALAW = FourCC{'a', 'l', 'a', 'w'}
	FourCC_ULAW = FourCC{'u', 'l', 'a', 'w'}
)

func ParseFourCC(s string) (f FourCC) {
	copy(f[:], s)
	return f
}

func (f FourCC) String() string {
	return string(f[:])
}

func (f FourCC) Name() string {
	switch f {
	case FourCC_H264:
		return "H264"
	case FourCC_H265:
		return "H265"
	case FourCC_AV1:
		return "AV1"
	case FourCC_VP9:
		return "VP9"
	case FourCC_VP8:
		return "VP8"
	case FourCC_MP4A:
		return "AAC"
	case FourCC_OPUS:
		return "OPUS"
	case FourCC_ALAW:
		return "PCMA"
	case FourCC_ULAW:
		return "PCMU"
	}
	return ""
}

func (f FourCC) Uint32() uint32 {
	return binary.BigEndian.Uint32(f[:])
}

func (f FourCC) FourCC() FourCC {
	return f
}

func (f FourCC) Is(fourcc FourCC) bool {
	return f == fourcc
}

type SPSInfo struct {
	ConstraintSetFlag uint
	ProfileIdc        uint
	LevelIdc          uint

	MbWidth  uint
	MbHeight uint

	CropLeft   uint
	CropRight  uint
	CropTop    uint
	CropBottom uint

	Width  uint
	Height uint
}

func (info *SPSInfo) Unmarshal(data []byte) (err error) {
	r := &util.GolombBitReader{R: bytes.NewReader(data)}

	if _, err = r.ReadBits(8); err != nil {
		return
	}

	if info.ProfileIdc, err = r.ReadBits(8); err != nil {
		return
	}

	// constraint_set0_flag-constraint_set6_flag,reserved_zero_2bits
	if info.ConstraintSetFlag, err = r.ReadBits(8); err != nil {
		return
	}

	// level_idc
	if info.LevelIdc, err = r.ReadBits(8); err != nil {
		return
	}

	// seq_parameter_set_id
	if _, err = r.ReadExponentialGolombCode(); err != nil {
		return
	}

	if info.ProfileIdc == 100 || info.ProfileIdc == 110 ||
		info.ProfileIdc == 122 || info.ProfileIdc == 244 ||
		info.ProfileIdc == 44 || info.ProfileIdc == 83 ||
		info.ProfileIdc == 86 || info.ProfileIdc == 118 {

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

	if info.MbWidth, err = r.ReadExponentialGolombCode(); err != nil {
		return
	}
	info.MbWidth++

	if info.MbHeight, err = r.ReadExponentialGolombCode(); err != nil {
		return
	}
	info.MbHeight++

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
		if info.CropLeft, err = r.ReadExponentialGolombCode(); err != nil {
			return
		}
		if info.CropRight, err = r.ReadExponentialGolombCode(); err != nil {
			return
		}
		if info.CropTop, err = r.ReadExponentialGolombCode(); err != nil {
			return
		}
		if info.CropBottom, err = r.ReadExponentialGolombCode(); err != nil {
			return
		}
	}

	info.Width = (info.MbWidth * 16) - info.CropLeft*2 - info.CropRight*2
	info.Height = ((2 - frame_mbs_only_flag) * info.MbHeight * 16) - info.CropTop*2 - info.CropBottom*2

	return
}
