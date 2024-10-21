package rtmp

import (
	"errors"
	"fmt"
	"io"

	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
)

type (
	AudioCodecID byte
	VideoCodecID byte

	H265Ctx struct {
		codec.H265Ctx
		Enhanced bool
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

var ErrDecconfInvalid = errors.New("decode error")

var SamplingFrequencies = [...]int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350, 0, 0, 0}
var RTMP_AVC_HEAD = []byte{0x17, 0x00, 0x00, 0x00, 0x00, 0x01, 0x42, 0x00, 0x1E, 0xFF}

var ErrHevc = errors.New("hevc parse config error")

var (
	ErrInvalidMarker       = errors.New("invalid marker value found in AV1CodecConfigurationRecord")
	ErrInvalidVersion      = errors.New("unsupported AV1CodecConfigurationRecord version")
	ErrNonZeroReservedBits = errors.New("non-zero reserved bits found in AV1CodecConfigurationRecord")
)

func (p *AV1Ctx) GetInfo() string {
	return fmt.Sprintf("% 02X", p.ConfigOBUs)
}

func (p *AV1Ctx) Unmarshal(data *util.MemoryReader) (err error) {
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
