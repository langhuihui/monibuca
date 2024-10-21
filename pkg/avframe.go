package pkg

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/pkg/codecs/av1"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
)

type (
	IAudioCodecCtx interface {
		codec.ICodecCtx
		GetSampleRate() int
		GetChannels() int
		GetSampleSize() int
	}
	IVideoCodecCtx interface {
		codec.ICodecCtx
		Width() int
		Height() int
	}
	IDataFrame interface {
	}
	// Source -> Parse -> Demux -> (ConvertCtx) -> Mux(GetAllocator) -> Recycle
	IAVFrame interface {
		GetAllocator() *util.ScalableMemoryAllocator
		SetAllocator(*util.ScalableMemoryAllocator)
		Parse(*AVTrack) error                                          // get codec info, idr
		ConvertCtx(codec.ICodecCtx) (codec.ICodecCtx, IAVFrame, error) // convert codec from source stream
		Demux(codec.ICodecCtx) (any, error)                            // demux to raw format
		Mux(codec.ICodecCtx, *AVFrame)                                 // mux from raw format
		GetTimestamp() time.Duration
		GetCTS() time.Duration
		GetSize() int
		Recycle()
		String() string
		Dump(byte, io.Writer)
	}

	Nalus []util.Memory

	AudioData = util.Memory

	OBUs AudioData

	AVFrame struct {
		DataFrame
		IDR       bool
		Timestamp time.Duration // 绝对时间戳
		CTS       time.Duration // composition time stamp
		Wraps     []IAVFrame    // 封装格式
	}

	AVRing    = util.Ring[AVFrame]
	DataFrame struct {
		sync.RWMutex
		discard   bool
		Sequence  uint32    // 在一个Track中的序号
		WriteTime time.Time // 写入时间,可用于比较两个帧的先后
		Raw       any       // 裸格式
	}
)

var _ IAVFrame = (*AnnexB)(nil)

func (frame *AVFrame) Clone() {

}

func (frame *AVFrame) Reset() {
	frame.Timestamp = 0
	frame.IDR = false
	frame.CTS = 0
	frame.Raw = nil
	if len(frame.Wraps) > 0 {
		for _, wrap := range frame.Wraps {
			wrap.Recycle()
		}
		frame.Wraps = frame.Wraps[:0]
	}
}

func (frame *AVFrame) Discard() {
	frame.discard = true
	frame.Reset()
}

func (frame *AVFrame) Demux(codecCtx codec.ICodecCtx) (err error) {
	frame.Raw, err = frame.Wraps[0].Demux(codecCtx)
	return
}

func (df *DataFrame) StartWrite() (success bool) {
	if df.discard {
		return
	}
	if df.TryLock() {
		return true
	}
	df.discard = true
	return
}

func (df *DataFrame) Ready() {
	df.WriteTime = time.Now()
	df.Unlock()
}

func (nalus *Nalus) H264Type() codec.H264NALUType {
	return codec.ParseH264NALUType((*nalus)[0].Buffers[0][0])
}

func (nalus *Nalus) H265Type() codec.H265NALUType {
	return codec.ParseH265NALUType((*nalus)[0].Buffers[0][0])
}

func (nalus *Nalus) Append(bytes []byte) {
	*nalus = append(*nalus, util.Memory{Buffers: net.Buffers{bytes}, Size: len(bytes)})
}

func (nalus *Nalus) ParseAVCC(reader *util.MemoryReader, naluSizeLen int) error {
	for reader.Length > 0 {
		l, err := reader.ReadBE(naluSizeLen)
		if err != nil {
			return err
		}
		var mem util.Memory
		reader.RangeN(int(l), mem.AppendOne)
		*nalus = append(*nalus, mem)
	}
	return nil
}

func (obus *OBUs) ParseAVCC(reader *util.MemoryReader) error {
	var obuHeader av1.OBUHeader
	startLen := reader.Length
	for reader.Length > 0 {
		offset := reader.Size - reader.Length
		b, err := reader.ReadByte()
		if err != nil {
			return err
		}
		err = obuHeader.Unmarshal([]byte{b})
		if err != nil {
			return err
		}
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
		(*AudioData)(obus).AppendOne(obu)
	}
	return nil
}
