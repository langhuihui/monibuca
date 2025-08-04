package codec

import (
	"fmt"

	"github.com/deepch/vdk/codec/h265parser"
)

type H265NALUType byte

func (H265NALUType) Parse(b byte) H265NALUType {
	return H265NALUType(b & 0x7E >> 1)
}

func ParseH265NALUType(b byte) H265NALUType {
	return H265NALUType(b & 0x7E >> 1)
}

var AudNalu = []byte{0x00, 0x00, 0x00, 0x01, 0x46, 0x01, 0x10}

type (
	H265Ctx struct {
		h265parser.CodecData
	}
)

func NewH265CtxFromRecord(record []byte) (ret *H265Ctx, err error) {
	ret = &H265Ctx{}
	ret.CodecData, err = h265parser.NewCodecDataFromAVCDecoderConfRecord(record)
	if err == nil {
		ret.RecordInfo.LengthSizeMinusOne = 3
	}
	return
}

func (ctx *H265Ctx) GetInfo() string {
	return fmt.Sprintf("fps: %d, resolution: %s", ctx.FPS(), ctx.Resolution())
}

func (*H265Ctx) FourCC() FourCC {
	return FourCC_H265
}

func (h265 *H265Ctx) GetBase() ICodecCtx {
	return h265
}

func (h265 *H265Ctx) GetRecord() []byte {
	return h265.Record
}

func (h265 *H265Ctx) String() string {
	// 根据 HEVC 标准格式：hvc1.profile.compatibility.level.constraints
	profile := h265.RecordInfo.AVCProfileIndication
	compatibility := h265.RecordInfo.ProfileCompatibility
	level := h265.RecordInfo.AVCLevelIndication

	// 简单实现，使用可用字段模拟 HEVC 格式
	return fmt.Sprintf("hvc1.%d.%X.L%d.00", profile, compatibility, level)
}
