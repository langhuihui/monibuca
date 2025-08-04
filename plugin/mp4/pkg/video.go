package mp4

import (
	"fmt"

	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
)

var _ pkg.IAVFrame = (*VideoFrame)(nil)

type VideoFrame struct {
	pkg.Sample
}

func (v *VideoFrame) Demux() (err error) {
	if v.Size == 0 {
		return fmt.Errorf("no video data to demux")
	}

	reader := v.NewReader()
	// 根据编解码器类型进行解复用
	switch ctx := v.ICodecCtx.(type) {
	case *codec.H264Ctx:
		// 对于 H.264，解析 AVCC 格式的 NAL 单元
		if err := v.ParseAVCC(&reader, int(ctx.RecordInfo.LengthSizeMinusOne)+1); err != nil {
			return fmt.Errorf("failed to parse H.264 AVCC: %w", err)
		}
	case *codec.H265Ctx:
		// 对于 H.265，解析 AVCC 格式的 NAL 单元
		if err := v.ParseAVCC(&reader, int(ctx.RecordInfo.LengthSizeMinusOne)+1); err != nil {
			return fmt.Errorf("failed to parse H.265 AVCC: %w", err)
		}
	default:
		// 对于其他格式，尝试默认的 AVCC 解析（4字节长度前缀）
		if err := v.ParseAVCC(&reader, 4); err != nil {
			return fmt.Errorf("failed to parse AVCC with default settings: %w", err)
		}
	}

	return
}

// Mux implements pkg.IAVFrame.
func (v *VideoFrame) Mux(sample *pkg.Sample) (err error) {
	v.InitRecycleIndexes(0)
	if v.ICodecCtx == nil {
		v.ICodecCtx = sample.GetBase()
	}
	switch rawData := sample.Raw.(type) {
	case *pkg.Nalus:
		// 根据编解码器类型确定 NALU 长度字段的大小
		var naluSizeLen int = 4 // 默认使用 4 字节
		switch ctx := sample.ICodecCtx.(type) {
		case *codec.H264Ctx:
			naluSizeLen = int(ctx.RecordInfo.LengthSizeMinusOne) + 1
		case *codec.H265Ctx:
			naluSizeLen = int(ctx.RecordInfo.LengthSizeMinusOne) + 1
		}
		// 为每个 NALU 添加长度前缀
		for nalu := range rawData.RangePoint {
			util.PutBE(v.NextN(naluSizeLen), nalu.Size) // 写入 NALU 长度
			v.Push(nalu.Buffers...)
		}
	}
	return
}

// String implements pkg.IAVFrame.
func (v *VideoFrame) String() string {
	return fmt.Sprintf("MP4Video[ts:%s, cts:%s, size:%d, keyframe:%t]",
		v.Timestamp, v.CTS, v.Size, v.IDR)
}
