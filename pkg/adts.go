package pkg

import (
	"github.com/deepch/vdk/codec/aacparser"
	"io"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
	"time"
)

var _ IAVFrame = (*ADTS)(nil)

type ADTS struct {
	DTS time.Duration
	util.RecyclableMemory
}

func (A *ADTS) Parse(track *AVTrack) (err error) {
	if track.ICodecCtx == nil {
		var ctx = &codec.AACCtx{}
		var reader = A.NewReader()
		var adts []byte
		adts, err = reader.ReadBytes(7)
		if err != nil {
			return err
		}
		var hdrlen, framelen, samples int
		ctx.Config, hdrlen, framelen, samples, err = aacparser.ParseADTSHeader(adts)
		if err != nil {
			return err
		}
		track.ICodecCtx = ctx
		track.Info("ADTS", "hdrlen", hdrlen, "framelen", framelen, "samples", samples)
	}
	track.Value.Raw, err = A.Demux(track.ICodecCtx)
	return
}

func (A *ADTS) ConvertCtx(ctx codec.ICodecCtx) (codec.ICodecCtx, IAVFrame, error) {
	return ctx.GetBase(), nil, nil
}

func (A *ADTS) Demux(ctx codec.ICodecCtx) (any, error) {
	var reader = A.NewReader()
	err := reader.Skip(7)
	var mem util.Memory
	reader.Range(mem.AppendOne)
	return mem, err
}

func (A *ADTS) Mux(ctx codec.ICodecCtx, frame *AVFrame) {
	aacCtx := ctx.GetBase().(*codec.AACCtx)
	A.InitRecycleIndexes(1)
	adts := A.NextN(7)
	raw := frame.Raw.(util.Memory)
	aacparser.FillADTSHeader(adts, aacCtx.Config, raw.Size/aacCtx.GetSampleSize(), raw.Size)
	A.Append(raw.Buffers...)
}

func (A *ADTS) GetTimestamp() time.Duration {
	return A.DTS * time.Millisecond / 90
}

func (A *ADTS) GetCTS() time.Duration {
	return 0
}

func (A *ADTS) GetSize() int {
	return A.Size
}

func (A *ADTS) String() string {
	//TODO implement me
	panic("implement me")
}

func (A *ADTS) Dump(b byte, writer io.Writer) {
	//TODO implement me
	panic("implement me")
}
