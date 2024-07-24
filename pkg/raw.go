package pkg

import (
	"fmt"
	"io"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
	"time"
)

var _ IAVFrame = (*RawAudio)(nil)

type RawAudio struct {
	codec.FourCC
	Timestamp time.Duration
	util.RecyclableMemory
}

func (r *RawAudio) Parse(track *AVTrack) error {
	if track.ICodecCtx == nil {
		switch r.FourCC {
		case codec.FourCC_ALAW:
			track.ICodecCtx = &codec.PCMACtx{
				AudioCtx: codec.AudioCtx{
					SampleRate: 8000,
					Channels:   1,
					SampleSize: 8,
				},
			}
		case codec.FourCC_ULAW:
			track.ICodecCtx = &codec.PCMUCtx{
				AudioCtx: codec.AudioCtx{
					SampleRate: 8000,
					Channels:   1,
					SampleSize: 8,
				},
			}
		}
	}
	return nil
}

func (r *RawAudio) ConvertCtx(ctx codec.ICodecCtx) (codec.ICodecCtx, IAVFrame, error) {
	return ctx.GetBase(), nil, nil
}

func (r *RawAudio) Demux(ctx codec.ICodecCtx) (any, error) {
	return r.Memory, nil
}

func (r *RawAudio) Mux(ctx codec.ICodecCtx, frame *AVFrame) {
	r.InitRecycleIndexes(0)
	r.Memory = frame.Raw.(util.Memory)
	r.Timestamp = frame.Timestamp
}

func (r *RawAudio) GetTimestamp() time.Duration {
	return r.Timestamp
}

func (r *RawAudio) GetCTS() time.Duration {
	return 0
}

func (r *RawAudio) GetSize() int {
	return r.Size
}

func (r *RawAudio) String() string {
	return fmt.Sprintf("RawAudio{FourCC: %s, Timestamp: %s, Size: %d}", r.FourCC, r.Timestamp, r.Size)
}

func (r *RawAudio) Dump(b byte, writer io.Writer) {
	//TODO implement me
	panic("implement me")
}

var _ IAVFrame = (*H26xFrame)(nil)

type H26xFrame struct {
	Timestamp time.Duration
	CTS       time.Duration
	Nalus
	util.RecyclableMemory
}

func (h *H26xFrame) Parse(track *AVTrack) error {
	//TODO implement me
	panic("implement me")
}

func (h *H26xFrame) ConvertCtx(ctx codec.ICodecCtx) (codec.ICodecCtx, IAVFrame, error) {
	return ctx.GetBase(), nil, nil
}

func (h *H26xFrame) Demux(ctx codec.ICodecCtx) (any, error) {
	return h.Nalus, nil
}

func (h *H26xFrame) Mux(ctx codec.ICodecCtx, frame *AVFrame) {
	h.Nalus = frame.Raw.(Nalus)
	h.Timestamp = frame.Timestamp
	h.CTS = frame.CTS
}

func (h *H26xFrame) GetTimestamp() time.Duration {
	return h.Timestamp
}

func (h *H26xFrame) GetCTS() time.Duration {
	return h.CTS
}

func (h *H26xFrame) GetSize() int {
	var size int
	for _, nalu := range h.Nalus {
		size += nalu.Size
	}
	return size
}

func (h *H26xFrame) String() string {
	//TODO implement me
	panic("implement me")
}

func (h *H26xFrame) Dump(b byte, writer io.Writer) {
	//TODO implement me
	panic("implement me")
}
