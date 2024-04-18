package rtp

import . "m7s.live/m7s/v5/pkg"

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
)

type RTPVideo struct {
	RTPData
}

func (r *RTPVideo) FromRaw(*AVTrack, any) error {
	panic("unimplemented")
}

func (r *RTPVideo) ToRaw(*AVTrack) (any, error) {
	return r.Payload, nil
}
