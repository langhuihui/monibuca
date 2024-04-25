package rtp

import (
	"unsafe"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	. "m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
)

type (
	RTPH264Ctx struct {
		RTPCtx
		SPS [][]byte
		PPS [][]byte
	}
	RTPH265Ctx struct {
		RTPH264Ctx
		VPS [][]byte
	}
	RTPAV1Ctx struct {
		RTPCtx
	}
	RTPVP9Ctx struct {
		RTPCtx
	}
	RTPVideo struct {
		RTPData
		IDR bool
	}
)

var _ IAVFrame = (*RTPVideo)(nil)

func (r *RTPVideo) IsIDR() bool {
	return r.IDR
}

func (h264 *RTPH264Ctx) CreateFrame(raw any) (frame IAVFrame, err error) {
	var r RTPVideo
	nalus := raw.(Nalus)
	nalutype := codec.H264NALUType(nalus.Nalus[0][0][0] & 0x1F)
	getHeader := func() rtp.Header {
		h264.SequenceNumber++
		return rtp.Header{
			SequenceNumber: h264.SequenceNumber,
			Timestamp:      uint32(nalus.PTS),
			SSRC:           h264.SSRC,
			PayloadType:    96,
		}
	}

	if nalutype == codec.NALU_IDR_Picture {
		r.IDR = true
		r.Packets = append(r.Packets, &rtp.Packet{
			Header:  getHeader(),
			Payload: h264.SPS[0],
		}, &rtp.Packet{
			Header:  getHeader(),
			Payload: h264.PPS[0],
		})
		r.Packets[1].Header.SequenceNumber++
		h264.SequenceNumber += 2
	}
	for _, nalu := range nalus.Nalus {
		var reader util.Buffers
		startIndex := len(r.Packets)
		reader.ReadFromBytes(nalu...)
		if reader.Length > 1460 {
			for reader.Length > 0 {
				mem := r.Malloc(1460)
				n := reader.ReadBytesTo(mem[1:])
				mem[0] = codec.NALU_FUA.Or(mem[1] & 0x60)
				if n < 1459 {
					r.RecycleBack(1459 - n)
				}
				r.Packets = append(r.Packets, &rtp.Packet{
					Header:  getHeader(),
					Payload: mem,
				})
			}
			r.Packets[startIndex].Payload[1] |= 1 << 7       // set start bit
			r.Packets[len(r.Packets)-1].Payload[1] |= 1 << 6 // set end bit
		} else {
			mem := r.Malloc(reader.Length)
			reader.ReadBytesTo(mem)
			r.Packets = append(r.Packets, &rtp.Packet{
				Header:  getHeader(),
				Payload: mem,
			})
		}
	}
	frame = &r
	return
}

func (r *RTPVideo) FromRaw(t *AVTrack, raw any) error {
	if t.ICodecCtx == nil {
		switch t.Codec {
		case codec.FourCC_H264:
			var ctx RTPH264Ctx
			ctx.SSRC = uint32(uintptr(unsafe.Pointer(t)))
			ctx.RTPCodecParameters = webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:    webrtc.MimeTypeH264,
					ClockRate:   90000,
					Channels:    0,
					SDPFmtpLine: "profile-level-id=42e01f;level-asymmetry-allowed=1;packetization-mode=1",
				},
				PayloadType: webrtc.PayloadType(96),
			}
			nalus := raw.(Nalus)
			ctx.SPS = nalus.Nalus[0]
			ctx.PPS = nalus.Nalus[1]
			t.ICodecCtx = &ctx
		}
	} else {
		switch t.Codec {
		case codec.FourCC_H264:

		}
	}
	// switch v := raw.(type) {
	// case Nalus:

	// case OBUs:
	// }
	return nil
}

func (r *RTPVideo) ToRaw(ICodecCtx) (any, error) {

	return nil, nil
}
