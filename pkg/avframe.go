package pkg

import (
	"net"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/pkg/codecs/av1"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
)

type (
	ICodecCtx interface {
		CreateFrame(*AVFrame) (IAVFrame, error)
		FourCC() codec.FourCC
	}
	IAudioCodecCtx interface {
		ICodecCtx
		GetSampleRate() int
		GetChannels() int
		GetSampleSize() int
	}
	IVideoCodecCtx interface {
		ICodecCtx
		GetWidth() int
		GetHeight() int
	}
	IDataFrame interface {
	}
	IAVFrame interface {
		GetScalableMemoryAllocator() *util.ScalableMemoryAllocator
		Parse(*AVTrack) (bool, bool, any, error)
		DecodeConfig(*AVTrack, ICodecCtx) error
		ToRaw(ICodecCtx) (any, error)
		GetTimestamp() time.Duration
		GetSize() int
		Recycle()
		String() string
	}

	Nalu = [][]byte

	Nalus struct {
		PTS   time.Duration
		DTS   time.Duration
		Nalus []Nalu
	}
	AVFrame struct {
		DataFrame
		IDR       bool
		Timestamp time.Duration // 绝对时间戳
		Wraps     []IAVFrame      // 封装格式
	}
	AVRing    = util.Ring[AVFrame]
	DataFrame struct {
		sync.RWMutex `json:"-" yaml:"-"` // 读写锁
		discard      bool
		Sequence     uint32    // 在一个Track中的序号
		BytesIn      int       // 输入字节数用于计算BPS
		WriteTime    time.Time // 写入时间,可用于比较两个帧的先后
		Raw          any       `json:"-" yaml:"-"` // 裸格式
	}
)

func (frame *AVFrame) Reset() {
	frame.BytesIn = 0
	frame.Timestamp = 0
	for _, wrap := range frame.Wraps {
		wrap.Recycle()
		wrap = nil
	}
}

func (df *DataFrame) StartWrite() bool {
	if df.TryLock() {
		return true
	} else {
		df.discard = true
		return false
	}
}

func (df *DataFrame) Ready() {
	df.WriteTime = time.Now()
	df.Unlock()
}

func (nalus *Nalus) H264Type() codec.H264NALUType {
	return codec.ParseH264NALUType(nalus.Nalus[0][0][0])
}

func (nalus *Nalus) H265Type() codec.H265NALUType {
	return codec.ParseH265NALUType(nalus.Nalus[0][0][0])
}

func (nalus *Nalus) Append(bytes ...[]byte) {
	nalus.Nalus = append(nalus.Nalus, bytes)
}

func (nalus *Nalus) ParseAVCC(reader *util.Buffers, naluSizeLen int) error {
	for reader.Length > 0 {
		l, err := reader.ReadBE(naluSizeLen)
		if err != nil {
			return err
		}
		nalu, err := reader.ReadBytes(int(l))
		if err != nil {
			return err
		}
		nalus.Append(nalu)
	}
	return nil
}

type OBUs struct {
	PTS  time.Duration
	OBUs []net.Buffers
}

func (obus *OBUs) Append(bytes ...[]byte) {
	obus.OBUs = append(obus.OBUs, bytes)
}

func (obus *OBUs) ParseAVCC(reader *util.Buffers) error {
	var obuHeader av1.OBUHeader
	for reader.Length > 0 {
		offset := reader.Offset
		b, _ := reader.ReadByte()
		obuHeader.Unmarshal([]byte{b})
		// if log.Trace {
		// 	vt.Trace("obu", zap.Any("type", obuHeader.Type), zap.Bool("iframe", vt.Value.IFrame))
		// }
		obuSize, _, _ := reader.LEB128Unmarshal()
		end := reader.Offset
		size := end - offset + int(obuSize)
		reader = &util.Buffers{Buffers: reader.Buffers}
		reader.Skip(offset)
		obu, err := reader.ReadBytes(size)
		if err != nil {
			return err
		}
		obus.Append(obu)
	}
	return nil
}
