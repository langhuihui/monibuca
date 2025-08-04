package format

import (
	"bytes"
	"fmt"

	"github.com/deepch/vdk/codec/aacparser"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
)

var _ pkg.IAVFrame = (*Mpeg2Audio)(nil)

type Mpeg2Audio struct {
	pkg.Sample
}

func (A *Mpeg2Audio) CheckCodecChange() (err error) {
	old := A.ICodecCtx
	if old == nil || old.FourCC().Is(codec.FourCC_MP4A) {
		var reader = A.NewReader()
		var adts []byte
		adts, err = reader.ReadBytes(7)
		if err != nil {
			return
		}
		var hdrlen, framelen, samples int
		var conf aacparser.MPEG4AudioConfig
		conf, hdrlen, framelen, samples, err = aacparser.ParseADTSHeader(adts)
		if err != nil {
			return
		}
		b := &bytes.Buffer{}
		aacparser.WriteMPEG4AudioConfig(b, conf)
		if old == nil || !bytes.Equal(b.Bytes(), old.GetRecord()) {
			var ctx = &codec.AACCtx{}
			ctx.ConfigBytes = b.Bytes()
			A.ICodecCtx = ctx
			if false {
				println("ADTS", "hdrlen", hdrlen, "framelen", framelen, "samples", samples, "config", ctx.Config)
			}
			// track.Info("ADTS", "hdrlen", hdrlen, "framelen", framelen, "samples", samples)
		} else {

		}
	}
	return
}

func (A *Mpeg2Audio) Demux() (err error) {
	var reader = A.NewReader()
	mem := A.GetAudioData()
	if A.ICodecCtx.FourCC().Is(codec.FourCC_MP4A) {
		err = reader.Skip(7)
		if err != nil {
			return
		}
	}
	reader.Range(mem.PushOne)
	return
}

func (A *Mpeg2Audio) Mux(frame *pkg.Sample) (err error) {
	if A.ICodecCtx == nil {
		A.ICodecCtx = frame.GetBase()
	}
	raw := frame.Raw.(*pkg.AudioData)
	aacCtx, ok := A.ICodecCtx.(*codec.AACCtx)
	if ok {
		A.InitRecycleIndexes(1)
		adts := A.NextN(7)
		aacparser.FillADTSHeader(adts, aacCtx.Config, raw.Size/aacCtx.GetSampleSize(), raw.Size)
	} else {
		A.InitRecycleIndexes(0)
	}
	A.Push(raw.Buffers...)
	return
}

func (A *Mpeg2Audio) String() string {
	return fmt.Sprintf("ADTS{size:%d}", A.Size)
}
