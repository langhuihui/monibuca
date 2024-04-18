package pkg

import (
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bluenviron/mediacommon/pkg/codecs/av1"
	"m7s.live/m7s/v5/pkg/util"
)

type (
	ICodecCtx interface {
		GetSequenceFrame() IAVFrame
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
		DecodeConfig(*AVTrack) error
		ToRaw(*AVTrack) (any, error)
		FromRaw(*AVTrack, any) error
		GetTimestamp() time.Duration
		GetSize() int
		Recycle()
		IsIDR() bool
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
		Timestamp time.Duration // 绝对时间戳
		Wrap      IAVFrame      // 封装格式
	}
	DataFrame struct {
		sync.Cond   `json:"-" yaml:"-"`
		readerCount atomic.Int32 // 读取者数量
		Sequence    uint32       // 在一个Track中的序号
		BytesIn     int          // 输入字节数用于计算BPS
		WriteTime   time.Time    // 写入时间,可用于比较两个帧的先后
		CanRead     bool         // 是否可读取
		Raw         any          `json:"-" yaml:"-"` // 裸格式
	}
)

func (frame *AVFrame) Reset() {
	frame.BytesIn = 0
	frame.Timestamp = 0
	if frame.Wrap != nil {
		frame.Wrap.Recycle()
		frame.Wrap = nil
	}
}

func (df *DataFrame) IsWriting() bool {
	return !df.CanRead
}

func (df *DataFrame) IsDiscarded() bool {
	return df.L == nil
}

func (df *DataFrame) Discard() int32 {
	df.L = nil //标记为废弃
	return df.readerCount.Load()
}

func (df *DataFrame) ReaderEnter() int32 {
	return df.readerCount.Add(1)
}

func (df *DataFrame) ReaderCount() int32 {
	return df.readerCount.Load()
}

func (df *DataFrame) ReaderLeave() int32 {
	return df.readerCount.Add(-1)
}

func (df *DataFrame) StartWrite() bool {
	if df.readerCount.Load() > 0 {
		df.Discard() //标记为废弃
		return false
	} else {
		df.Init()
		df.CanRead = false //标记为正在写入
		return true
	}
}

func (df *DataFrame) Ready() {
	df.WriteTime = time.Now()
	df.CanRead = true //标记为可读取
	df.Broadcast()
}

func (df *DataFrame) Init() {
	df.L = EmptyLocker
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
