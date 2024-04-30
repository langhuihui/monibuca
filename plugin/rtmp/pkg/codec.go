package rtmp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"

	"github.com/cnotch/ipchub/av/codec/hevc"
	"github.com/q191201771/naza/pkg/nazabits"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
)

type (
	AudioCodecID byte
	VideoCodecID byte

	H264Ctx struct {
		codec.H264Ctx
		ConfigurationVersion byte // 8 bits Version
		AVCProfileIndication byte // 8 bits
		ProfileCompatibility byte // 8 bits
		AVCLevelIndication   byte // 8 bits
		LengthSizeMinusOne   byte
		NalulenSize          int
	}

	H265Ctx struct {
		codec.H265Ctx
		NalulenSize int
	}

	AV1Ctx struct {
		codec.AV1Ctx
		Version                          byte
		SeqProfile                       byte
		SeqLevelIdx0                     byte
		SeqTier0                         byte
		HighBitdepth                     byte
		TwelveBit                        byte
		MonoChrome                       byte
		ChromaSubsamplingX               byte
		ChromaSubsamplingY               byte
		ChromaSamplePosition             byte
		InitialPresentationDelayPresent  byte
		InitialPresentationDelayMinusOne byte
	}
	PCMACtx struct {
		codec.PCMACtx
	}
	PCMUCtx struct {
		codec.PCMUCtx
	}
	AACCtx struct {
		codec.AACCtx
		AudioSpecificConfig
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
	HVCDecoderConfigurationRecord struct {
		PicWidthInLumaSamples  uint32 // sps
		PicHeightInLumaSamples uint32 // sps

		configurationVersion uint8

		generalProfileSpace              uint8
		generalTierFlag                  uint8
		generalProfileIdc                uint8
		generalProfileCompatibilityFlags uint32
		generalConstraintIndicatorFlags  uint64
		generalLevelIdc                  uint8

		lengthSizeMinusOne uint8

		numTemporalLayers    uint8
		temporalIdNested     uint8
		parallelismType      uint8
		chromaFormat         uint8
		bitDepthLumaMinus8   uint8
		bitDepthChromaMinus8 uint8
		avgFrameRate         uint16
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

func ParseAudioCodec(name codec.FourCC) AudioCodecID {
	switch name {
	case codec.FourCC_MP4A:
		return CodecID_AAC
	case codec.FourCC_ALAW:
		return CodecID_PCMA
	case codec.FourCC_ULAW:
		return CodecID_PCMU
	case codec.FourCC_OPUS:
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

func ParseVideoCodec(name codec.FourCC) VideoCodecID {
	switch name {
	case codec.FourCC_H264:
		return CodecID_H264
	case codec.FourCC_H265:
		return CodecID_H265
	case codec.FourCC_AV1:
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

func (ctx *H264Ctx) GetSPSInfo() codec.SPSInfo {
	return ctx.SPSInfo
}

func (ctx *H264Ctx) GetSPS() [][]byte {
	return ctx.SPS
}

func (ctx *H264Ctx) GetPPS() [][]byte {
	return ctx.PPS
}

func (ctx *H264Ctx) Unmarshal(b *util.Buffers) (err error) {
	if b.Length < 7 {
		err = errors.New("not enough len")
		return
	}
	b.ReadByteTo(&ctx.ConfigurationVersion, &ctx.AVCProfileIndication, &ctx.ProfileCompatibility, &ctx.AVCLevelIndication, &ctx.LengthSizeMinusOne)
	ctx.LengthSizeMinusOne = ctx.LengthSizeMinusOne & 0x03
	ctx.NalulenSize = int(ctx.LengthSizeMinusOne) + 1
	var numOfSequenceParameterSets byte
	numOfSequenceParameterSets, err = b.ReadByteMask(0x1f)
	if err != nil {
		return
	}
	for range numOfSequenceParameterSets {
		spslen, err1 := b.ReadBE(2)
		if err1 != nil {
			return err1
		}
		spsbytes, err2 := b.ReadBytes(spslen)
		if err2 != nil {
			return err2
		}
		ctx.SPS = append(ctx.SPS, spsbytes)
	}
	if b.Length < 1 {
		err = ErrDecconfInvalid
		return
	}
	if err = ctx.SPSInfo.Unmarshal(ctx.SPS[0]); err != nil {
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
		ctx.PPS = append(ctx.PPS, ppsbytes)
	}
	return
}

func ParseHevcSPS(data []byte) (self codec.SPSInfo, err error) {
	var rawsps hevc.H265RawSPS
	if err = rawsps.Decode(data); err == nil {
		self.CropLeft, self.CropRight, self.CropTop, self.CropBottom = uint(rawsps.Conf_win_left_offset), uint(rawsps.Conf_win_right_offset), uint(rawsps.Conf_win_top_offset), uint(rawsps.Conf_win_bottom_offset)
		self.Width = uint(rawsps.Pic_width_in_luma_samples)
		self.Height = uint(rawsps.Pic_height_in_luma_samples)
	}
	return
}

var SamplingFrequencies = [...]int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350, 0, 0, 0}
var RTMP_AVC_HEAD = []byte{0x17, 0x00, 0x00, 0x00, 0x00, 0x01, 0x42, 0x00, 0x1E, 0xFF}

func (ctx *H264Ctx) GetWidth() int {
	return int(ctx.SPSInfo.Width)
}

func (ctx *H264Ctx) GetHeight() int {
	return int(ctx.SPSInfo.Height)
}

var ErrHevc = errors.New("hevc parse config error")

func (ctx *H265Ctx) GetVPS() [][]byte {
	return ctx.VPS
}

func (ctx *H265Ctx) Unmarshal(b *util.Buffers) (err error) {
	if b.Length < 23 {
		err = errors.New("not enough len")
		return
	}
	b.Skip(21)
	var x byte
	x, err = b.ReadByte()
	if err != nil {
		return ErrHevc
	}
	ctx.NalulenSize = int(x&0x03) + 1
	x, err = b.ReadByte() // number of arrays
	if err != nil {
		return ErrHevc
	}
	x, err = b.ReadByte()
	if err != nil || x&0x7f != byte(codec.NAL_UNIT_VPS) {
		return ErrHevc
	}
	numNalus, err := b.ReadBE(2)
	if err != nil {
		return ErrHevc
	}
	for range numNalus {
		vpslen, err := b.ReadBE(2)
		if err != nil {
			return ErrHevc
		}
		vps, err := b.ReadBytes(vpslen)
		if err != nil {
			return ErrHevc
		}
		ctx.VPS = append(ctx.VPS, vps)
	}
	x, err = b.ReadByte()
	if err != nil || x&0x7f != byte(codec.NAL_UNIT_SPS) {
		return ErrHevc
	}
	numNalus, err = b.ReadBE(2)
	if err != nil {
		return ErrHevc
	}
	for range numNalus {
		spslen, err := b.ReadBE(2)
		if err != nil {
			return ErrHevc
		}
		sps, err := b.ReadBytes(spslen)
		if err != nil {
			return ErrHevc
		}
		ctx.SPS = append(ctx.SPS, sps)
	}
	ctx.SPSInfo, err = ParseHevcSPS(ctx.SPS[0])
	if err != nil {
		return ErrHevc
	}
	x, err = b.ReadByte()
	if err != nil || x&0x7f != byte(codec.NAL_UNIT_PPS) {
		return ErrHevc
	}
	numNalus, err = b.ReadBE(2)
	if err != nil {
		return ErrHevc
	}
	for range numNalus {
		ppslen, err := b.ReadBE(2)
		if err != nil {
			return ErrHevc
		}
		pps, err := b.ReadBytes(ppslen)
		if err != nil {
			return ErrHevc
		}
		ctx.PPS = append(ctx.PPS, pps)
	}
	return
}

func BuildH265SeqHeaderFromVpsSpsPps(vps, sps, pps []byte) ([]byte, error) {
	sh := make([]byte, 43+len(vps)+len(sps)+len(pps))

	sh[0] = 0b1001_0000 | byte(PacketTypeSequenceStart)
	copy(sh[1:], codec.FourCC_H265[:])
	// unsigned int(8) configurationVersion = 1;
	sh[5] = 0x1

	ctx := HVCDecoderConfigurationRecord{
		configurationVersion:             1,
		lengthSizeMinusOne:               3, // 4 bytes
		generalProfileCompatibilityFlags: 0xffffffff,
		generalConstraintIndicatorFlags:  0xffffffffffff,
	}
	if err := ctx.ParseVps(vps); err != nil {
		return nil, err
	}
	if err := ctx.ParseSps(sps); err != nil {
		return nil, err
	}

	// unsigned int(2) general_profile_space;
	// unsigned int(1) general_tier_flag;
	// unsigned int(5) general_profile_idc;
	sh[6] = ctx.generalProfileSpace<<6 | ctx.generalTierFlag<<5 | ctx.generalProfileIdc
	// unsigned int(32) general_profile_compatibility_flags
	util.PutBE(sh[7:7+4], ctx.generalProfileCompatibilityFlags)
	// unsigned int(48) general_constraint_indicator_flags
	util.PutBE(sh[11:11+4], uint32(ctx.generalConstraintIndicatorFlags>>16))
	util.PutBE(sh[15:15+2], uint16(ctx.generalConstraintIndicatorFlags))
	// unsigned int(8) general_level_idc;
	sh[17] = ctx.generalLevelIdc

	// bit(4) reserved = ‘1111’b;
	// unsigned int(12) min_spatial_segmentation_idc;
	// bit(6) reserved = ‘111111’b;
	// unsigned int(2) parallelismType;
	// TODO chef: 这两个字段没有解析
	util.PutBE(sh[18:20], 0xf000)
	sh[20] = ctx.parallelismType | 0xfc

	// bit(6) reserved = ‘111111’b;
	// unsigned int(2) chromaFormat;
	sh[21] = ctx.chromaFormat | 0xfc

	// bit(5) reserved = ‘11111’b;
	// unsigned int(3) bitDepthLumaMinus8;
	sh[22] = ctx.bitDepthLumaMinus8 | 0xf8

	// bit(5) reserved = ‘11111’b;
	// unsigned int(3) bitDepthChromaMinus8;
	sh[23] = ctx.bitDepthChromaMinus8 | 0xf8

	// bit(16) avgFrameRate;
	util.PutBE(sh[24:26], ctx.avgFrameRate)

	// bit(2) constantFrameRate;
	// bit(3) numTemporalLayers;
	// bit(1) temporalIdNested;
	// unsigned int(2) lengthSizeMinusOne;
	sh[26] = 0<<6 | ctx.numTemporalLayers<<3 | ctx.temporalIdNested<<2 | ctx.lengthSizeMinusOne

	// num of vps sps pps
	sh[27] = 0x03
	i := 28
	sh[i] = byte(codec.NAL_UNIT_VPS)
	// num of vps
	util.PutBE(sh[i+1:i+3], 1)
	// length
	util.PutBE(sh[i+3:i+5], len(vps))
	copy(sh[i+5:], vps)
	i = i + 5 + len(vps)
	sh[i] = byte(codec.NAL_UNIT_SPS)
	util.PutBE(sh[i+1:i+3], 1)
	util.PutBE(sh[i+3:i+5], len(sps))
	copy(sh[i+5:], sps)
	i = i + 5 + len(sps)
	sh[i] = byte(codec.NAL_UNIT_PPS)
	util.PutBE(sh[i+1:i+3], 1)
	util.PutBE(sh[i+3:i+5], len(pps))
	copy(sh[i+5:], pps)

	return sh, nil
}
func (ctx *HVCDecoderConfigurationRecord) ParseVps(vps []byte) error {
	if len(vps) < 2 {
		return ErrHevc
	}

	rbsp := nal2rbsp(vps[2:])
	br := nazabits.NewBitReader(rbsp)

	// skip
	// vps_video_parameter_set_id u(4)
	// vps_reserved_three_2bits   u(2)
	// vps_max_layers_minus1      u(6)
	if _, err := br.ReadBits16(12); err != nil {
		return ErrHevc
	}

	vpsMaxSubLayersMinus1, err := br.ReadBits8(3)
	if err != nil {
		return ErrHevc
	}
	if vpsMaxSubLayersMinus1+1 > ctx.numTemporalLayers {
		ctx.numTemporalLayers = vpsMaxSubLayersMinus1 + 1
	}

	// skip
	// vps_temporal_id_nesting_flag u(1)
	// vps_reserved_0xffff_16bits   u(16)
	if _, err := br.ReadBits32(17); err != nil {
		return ErrHevc
	}

	return ctx.parsePtl(&br, vpsMaxSubLayersMinus1)
}

func (ctx *HVCDecoderConfigurationRecord) ParseSps(sps []byte) error {
	var err error

	if len(sps) < 2 {
		return ErrHevc
	}

	rbsp := nal2rbsp(sps[2:])
	br := nazabits.NewBitReader(rbsp)

	// sps_video_parameter_set_id
	if _, err = br.ReadBits8(4); err != nil {
		return err
	}

	spsMaxSubLayersMinus1, err := br.ReadBits8(3)
	if err != nil {
		return err
	}

	if spsMaxSubLayersMinus1+1 > ctx.numTemporalLayers {
		ctx.numTemporalLayers = spsMaxSubLayersMinus1 + 1
	}

	// sps_temporal_id_nesting_flag
	if ctx.temporalIdNested, err = br.ReadBit(); err != nil {
		return err
	}

	if err = ctx.parsePtl(&br, spsMaxSubLayersMinus1); err != nil {
		return err
	}

	// sps_seq_parameter_set_id
	if _, err = br.ReadGolomb(); err != nil {
		return err
	}

	var cf uint32
	if cf, err = br.ReadGolomb(); err != nil {
		return err
	}
	ctx.chromaFormat = uint8(cf)
	if ctx.chromaFormat == 3 {
		if _, err = br.ReadBit(); err != nil {
			return err
		}
	}

	if ctx.PicWidthInLumaSamples, err = br.ReadGolomb(); err != nil {
		return err
	}
	if ctx.PicHeightInLumaSamples, err = br.ReadGolomb(); err != nil {
		return err
	}

	conformanceWindowFlag, err := br.ReadBit()
	if err != nil {
		return err
	}
	if conformanceWindowFlag != 0 {
		if _, err = br.ReadGolomb(); err != nil {
			return err
		}
		if _, err = br.ReadGolomb(); err != nil {
			return err
		}
		if _, err = br.ReadGolomb(); err != nil {
			return err
		}
		if _, err = br.ReadGolomb(); err != nil {
			return err
		}
	}

	var bdlm8 uint32
	if bdlm8, err = br.ReadGolomb(); err != nil {
		return err
	}
	ctx.bitDepthLumaMinus8 = uint8(bdlm8)
	var bdcm8 uint32
	if bdcm8, err = br.ReadGolomb(); err != nil {
		return err
	}
	ctx.bitDepthChromaMinus8 = uint8(bdcm8)

	_, err = br.ReadGolomb()
	if err != nil {
		return err
	}
	spsSubLayerOrderingInfoPresentFlag, err := br.ReadBit()
	if err != nil {
		return err
	}
	var i uint8
	if spsSubLayerOrderingInfoPresentFlag != 0 {
		i = 0
	} else {
		i = spsMaxSubLayersMinus1
	}
	for ; i <= spsMaxSubLayersMinus1; i++ {
		if _, err = br.ReadGolomb(); err != nil {
			return err
		}
		if _, err = br.ReadGolomb(); err != nil {
			return err
		}
		if _, err = br.ReadGolomb(); err != nil {
			return err
		}
	}

	if _, err = br.ReadGolomb(); err != nil {
		return err
	}
	if _, err = br.ReadGolomb(); err != nil {
		return err
	}
	if _, err = br.ReadGolomb(); err != nil {
		return err
	}
	if _, err = br.ReadGolomb(); err != nil {
		return err
	}
	if _, err = br.ReadGolomb(); err != nil {
		return err
	}
	if _, err = br.ReadGolomb(); err != nil {
		return err
	}

	return nil
}

func (ctx *HVCDecoderConfigurationRecord) parsePtl(br *nazabits.BitReader, maxSubLayersMinus1 uint8) error {
	var err error
	var ptl HVCDecoderConfigurationRecord
	if ptl.generalProfileSpace, err = br.ReadBits8(2); err != nil {
		return err
	}
	if ptl.generalTierFlag, err = br.ReadBit(); err != nil {
		return err
	}
	if ptl.generalProfileIdc, err = br.ReadBits8(5); err != nil {
		return err
	}
	if ptl.generalProfileCompatibilityFlags, err = br.ReadBits32(32); err != nil {
		return err
	}
	if ptl.generalConstraintIndicatorFlags, err = br.ReadBits64(48); err != nil {
		return err
	}
	if ptl.generalLevelIdc, err = br.ReadBits8(8); err != nil {
		return err
	}
	ctx.updatePtl(&ptl)

	if maxSubLayersMinus1 == 0 {
		return nil
	}

	subLayerProfilePresentFlag := make([]uint8, maxSubLayersMinus1)
	subLayerLevelPresentFlag := make([]uint8, maxSubLayersMinus1)
	for i := uint8(0); i < maxSubLayersMinus1; i++ {
		if subLayerProfilePresentFlag[i], err = br.ReadBit(); err != nil {
			return err
		}
		if subLayerLevelPresentFlag[i], err = br.ReadBit(); err != nil {
			return err
		}
	}
	if maxSubLayersMinus1 > 0 {
		for i := maxSubLayersMinus1; i < 8; i++ {
			if _, err = br.ReadBits8(2); err != nil {
				return err
			}
		}
	}

	for i := uint8(0); i < maxSubLayersMinus1; i++ {
		if subLayerProfilePresentFlag[i] != 0 {
			if _, err = br.ReadBits32(32); err != nil {
				return err
			}
			if _, err = br.ReadBits32(32); err != nil {
				return err
			}
			if _, err = br.ReadBits32(24); err != nil {
				return err
			}
		}

		if subLayerLevelPresentFlag[i] != 0 {
			if _, err = br.ReadBits8(8); err != nil {
				return err
			}
		}
	}

	return nil
}

func (ctx *HVCDecoderConfigurationRecord) updatePtl(ptl *HVCDecoderConfigurationRecord) {
	ctx.generalProfileSpace = ptl.generalProfileSpace

	if ptl.generalTierFlag > ctx.generalTierFlag {
		ctx.generalLevelIdc = ptl.generalLevelIdc

		ctx.generalTierFlag = ptl.generalTierFlag
	} else {
		if ptl.generalLevelIdc > ctx.generalLevelIdc {
			ctx.generalLevelIdc = ptl.generalLevelIdc
		}
	}

	if ptl.generalProfileIdc > ctx.generalProfileIdc {
		ctx.generalProfileIdc = ptl.generalProfileIdc
	}

	ctx.generalProfileCompatibilityFlags &= ptl.generalProfileCompatibilityFlags

	ctx.generalConstraintIndicatorFlags &= ptl.generalConstraintIndicatorFlags
}

func nal2rbsp(nal []byte) []byte {
	// TODO chef:
	// 1. 输出应该可由外部申请
	// 2. 替换性能
	// 3. 该函数应该放入avc中
	return bytes.Replace(nal, []byte{0x0, 0x0, 0x3}, []byte{0x0, 0x0}, -1)
}

var (
	ErrInvalidMarker       = errors.New("invalid marker value found in AV1CodecConfigurationRecord")
	ErrInvalidVersion      = errors.New("unsupported AV1CodecConfigurationRecord version")
	ErrNonZeroReservedBits = errors.New("non-zero reserved bits found in AV1CodecConfigurationRecord")
)

func (p *AV1Ctx) Unmarshal(data *util.Buffers) (err error) {
	if data.Length < 4 {
		err = io.ErrShortWrite
		return
	}
	var b byte
	b, err = data.ReadByte()
	if err != nil {
		return
	}
	Marker := b >> 7
	if Marker != 1 {
		return ErrInvalidMarker
	}
	p.Version = b & 0x7F
	if p.Version != 1 {
		return ErrInvalidVersion
	}
	b, err = data.ReadByte()
	if err != nil {
		return
	}

	p.SeqProfile = b >> 5
	p.SeqLevelIdx0 = b & 0x1F

	b, err = data.ReadByte()
	if err != nil {
		return
	}

	p.SeqTier0 = b >> 7
	p.HighBitdepth = (b >> 6) & 0x01
	p.TwelveBit = (b >> 5) & 0x01
	p.MonoChrome = (b >> 4) & 0x01
	p.ChromaSubsamplingX = (b >> 3) & 0x01
	p.ChromaSubsamplingY = (b >> 2) & 0x01
	p.ChromaSamplePosition = b & 0x03

	b, err = data.ReadByte()
	if err != nil {
		return
	}

	if b>>5 != 0 {
		return ErrNonZeroReservedBits
	}
	p.InitialPresentationDelayPresent = (b >> 4) & 0x01
	if p.InitialPresentationDelayPresent == 1 {
		p.InitialPresentationDelayMinusOne = b & 0x0F
	} else {
		if b&0x0F != 0 {
			return ErrNonZeroReservedBits
		}
		p.InitialPresentationDelayMinusOne = 0
	}
	if data.Length > 0 {
		p.ConfigOBUs, err = data.ReadBytes(data.Length)
	}
	return nil
}
