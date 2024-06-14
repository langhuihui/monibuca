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
		GetInfo() string
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
		Wraps     []IAVFrame    // 封装格式
	}
	AVRing    = util.Ring[AVFrame]
	DataFrame struct {
		sync.RWMutex `json:"-" yaml:"-"` // 读写锁
		discard      bool
		Sequence     uint32    // 在一个Track中的序号
		WriteTime    time.Time // 写入时间,可用于比较两个帧的先后
		Raw          any       `json:"-" yaml:"-"` // 裸格式
	}
)

var _ IAVFrame = (*AnnexB)(nil)

func (frame *AVFrame) Reset() {
	frame.Timestamp = 0
	if len(frame.Wraps) > 0 {
		for _, wrap := range frame.Wraps {
			wrap.Recycle()
		}
		frame.Wraps = frame.Wraps[:0]
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

func (nalus *Nalus) ParseAVCC(reader *util.MemoryReader, naluSizeLen int) error {
	for reader.Length > 0 {
		l, err := reader.ReadBE(naluSizeLen)
		if err != nil {
			return err
		}
		for nalu := range reader.RangeN(l) {
			nalus.Append(nalu)
		}
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

func (obus *OBUs) ParseAVCC(reader *util.MemoryReader) error {
	var obuHeader av1.OBUHeader
	startLen := reader.Length
	for reader.Length > 0 {
		offset := reader.Size - reader.Length
		b, _ := reader.ReadByte()
		obuHeader.Unmarshal([]byte{b})
		// if log.Trace {
		// 	vt.Trace("obu", zap.Any("type", obuHeader.Type), zap.Bool("iframe", vt.Value.IFrame))
		// }
		obuSize, _, _ := reader.LEB128Unmarshal()
		end := reader.Size - reader.Length
		size := end - offset + int(obuSize)
		reader = &util.MemoryReader{Memory: reader.Memory, Length: startLen - offset}
		obu, err := reader.ReadBytes(size)
		if err != nil {
			return err
		}
		obus.Append(obu)
	}
	return nil
}
