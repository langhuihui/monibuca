package codec

import (
	"bytes"
	"fmt"
	"github.com/deepch/vdk/codec/h264parser"
)

// Start Code + NAL Unit -> NALU Header + NALU Body
// RTP Packet -> NALU Header + NALU Body

// NALU Body -> Slice Header + Slice data
// Slice data -> flags + Macroblock layer1 + Macroblock layer2 + ...
// Macroblock layer1 -> mb_type + PCM Data
// Macroblock layer2 -> mb_type + Sub_mb_pred or mb_pred + Residual Data
// Residual Data ->
type H264NALUType byte

func (b H264NALUType) Or(b2 byte) byte {
	return byte(b) | b2
}

func (b H264NALUType) Offset() int {
	switch b {
	case NALU_STAPA:
		return 1
	case NALU_STAPB:
		return 3
	case NALU_FUA:
		return 2
	case NALU_FUB:
		return 4
	}
	return 0
}

func (b H264NALUType) Byte() byte {
	return byte(b)
}
func ParseH264NALUType(b byte) H264NALUType {
	return H264NALUType(b & 0x1F)
}
func (t *H264NALUType) Parse(b byte) H264NALUType {
	*t = H264NALUType(b & 0x1F)
	return *t
}

func (H264NALUType) ParseBytes(bs []byte) H264NALUType {
	return H264NALUType(bs[0] & 0x1F)
}

const (
	// NALU Type
	NALU_Unspecified           H264NALUType = iota
	NALU_Non_IDR_Picture                    // 1
	NALU_Data_Partition_A                   // 2
	NALU_Data_Partition_B                   // 3
	NALU_Data_Partition_C                   // 4
	NALU_IDR_Picture                        // 5
	NALU_SEI                                // 6
	NALU_SPS                                // 7
	NALU_PPS                                // 8
	NALU_Access_Unit_Delimiter              // 9
	NALU_Sequence_End                       // 10
	NALU_Stream_End                         // 11
	NALU_Filler_Data                        // 12
	NALU_SPS_Extension                      // 13
	NALU_Prefix                             // 14
	NALU_SPS_Subset                         // 15
	NALU_DPS                                // 16
	NALU_Reserved1                          // 17
	NALU_Reserved2                          // 18
	NALU_Not_Auxiliary_Coded                // 19
	NALU_Coded_Slice_Extension              // 20
	NALU_Reserved3                          // 21
	NALU_Reserved4                          // 22
	NALU_Reserved5                          // 23
	NALU_STAPA                              // 24
	NALU_STAPB
	NALU_MTAP16
	NALU_MTAP24
	NALU_FUA // 28
	NALU_FUB
	// 24 - 31 NALU_NotReserved

)

var (
	NALU_AUD_BYTE   = []byte{0x00, 0x00, 0x00, 0x01, 0x09, 0xF0}
	NALU_Delimiter1 = []byte{0x00, 0x00, 0x01}
	NALU_Delimiter2 = []byte{0x00, 0x00, 0x00, 0x01}
)

// H.264/AVC视频编码标准中,整个系统框架被分为了两个层面:视频编码层面(VCL)和网络抽象层面(NAL)
// NAL - Network Abstract Layer
// raw byte sequence payload (RBSP) 原始字节序列载荷

// SplitH264 以0x00000001分割H264裸数据
func SplitH264(payload []byte) (nalus [][]byte) {
	for _, v := range bytes.SplitN(payload, NALU_Delimiter2, -1) {
		if len(v) == 0 {
			continue
		}
		nalus = append(nalus, bytes.SplitN(v, NALU_Delimiter1, -1)...)
	}
	return
}

type (
	H264Ctx struct {
		h264parser.CodecData
	}
)

func (*H264Ctx) FourCC() FourCC {
	return FourCC_H264
}

func (ctx *H264Ctx) GetInfo() string {
	return fmt.Sprintf("fps: %d, resolution: %s", ctx.FPS(), ctx.Resolution())
}

func (h264 *H264Ctx) GetBase() ICodecCtx {
	return h264
}
