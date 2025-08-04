package pkg

import (
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
		GetSample() *Sample
		GetSize() int
		CheckCodecChange() error
		Demux() error      // demux to raw format
		Mux(*Sample) error // mux from origin format
		Recycle()
		String() string
	}
	ISequenceCodecCtx[T any] interface {
		GetSequenceFrame() T
	}
	BaseSample struct {
		Raw                 IRaw // 裸格式用于转换的中间格式
		IDR                 bool
		TS0, Timestamp, CTS time.Duration // 原始 TS、修正 TS、Composition Time Stamp
	}
	Sample struct {
		codec.ICodecCtx
		util.RecyclableMemory
		*BaseSample
	}
	Nalus = util.ReuseArray[util.Memory]

	AudioData = util.Memory

	OBUs AudioData

	AVFrame struct {
		DataFrame
		*Sample
		Wraps []IAVFrame // 封装格式
	}
	IRaw interface {
		util.Resetter
		Count() int
	}
	AVRing    = util.Ring[AVFrame]
	DataFrame struct {
		sync.RWMutex
		discard   bool
		Sequence  uint32    // 在一个Track中的序号
		WriteTime time.Time // 写入时间,可用于比较两个帧的先后
	}
)

func (sample *Sample) GetSize() int {
	return sample.Size
}

func (sample *Sample) GetSample() *Sample {
	return sample
}

func (sample *Sample) CheckCodecChange() (err error) {
	return
}

func (sample *Sample) Demux() error {
	return nil
}

func (sample *Sample) Mux(from *Sample) error {
	sample.ICodecCtx = from.GetBase()
	return nil
}

func ConvertFrameType(from, to IAVFrame) (err error) {
	fromSampe, toSample := from.GetSample(), to.GetSample()
	if !fromSampe.HasRaw() {
		if err = from.Demux(); err != nil {
			return
		}
	}
	toSample.SetAllocator(fromSampe.GetAllocator())
	toSample.BaseSample = fromSampe.BaseSample
	return to.Mux(fromSampe)
}

func (b *BaseSample) HasRaw() bool {
	return b.Raw != nil && b.Raw.Count() > 0
}

// 90Hz
func (b *BaseSample) GetDTS() time.Duration {
	return b.Timestamp * 90 / time.Millisecond
}

func (b *BaseSample) GetPTS() time.Duration {
	return (b.Timestamp + b.CTS) * 90 / time.Millisecond
}

func (b *BaseSample) SetDTS(dts time.Duration) {
	b.Timestamp = dts * time.Millisecond / 90
}

func (b *BaseSample) SetPTS(pts time.Duration) {
	b.CTS = pts*time.Millisecond/90 - b.Timestamp
}

func (b *BaseSample) SetTS32(ts uint32) {
	b.Timestamp = time.Duration(ts) * time.Millisecond
}

func (b *BaseSample) GetTS32() uint32 {
	return uint32(b.Timestamp / time.Millisecond)
}

func (b *BaseSample) SetCTS32(ts uint32) {
	b.CTS = time.Duration(ts) * time.Millisecond
}

func (b *BaseSample) GetCTS32() uint32 {
	return uint32(b.CTS / time.Millisecond)
}

func (b *BaseSample) GetNalus() *util.ReuseArray[util.Memory] {
	if b.Raw == nil {
		b.Raw = &Nalus{}
	}
	return b.Raw.(*util.ReuseArray[util.Memory])
}

func (b *BaseSample) GetAudioData() *AudioData {
	if b.Raw == nil {
		b.Raw = &AudioData{}
	}
	return b.Raw.(*AudioData)
}

func (b *BaseSample) ParseAVCC(reader *util.MemoryReader, naluSizeLen int) error {
	array := b.GetNalus()
	for reader.Length > 0 {
		l, err := reader.ReadBE(naluSizeLen)
		if err != nil {
			return err
		}
		reader.RangeN(int(l), array.GetNextPointer().PushOne)
	}
	return nil
}

func (frame *AVFrame) Reset() {
	if len(frame.Wraps) > 0 {
		for _, wrap := range frame.Wraps {
			wrap.Recycle()
		}
		frame.BaseSample.IDR = false
		frame.BaseSample.TS0 = 0
		frame.BaseSample.Timestamp = 0
		frame.BaseSample.CTS = 0
		if frame.Raw != nil {
			frame.Raw.Reset()
		}
	}
}

func (frame *AVFrame) Discard() {
	frame.discard = true
	frame.Reset()
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
		(*AudioData)(obus).PushOne(obu)
	}
	return nil
}

func (obus *OBUs) Reset() {
	((*util.Memory)(obus)).Reset()
}

func (obus *OBUs) Count() int {
	return (*util.Memory)(obus).Count()
}
