package rtp

import (
	"github.com/pion/webrtc/v4"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/util"
	"testing"
)

func TestRTPH264Ctx_CreateFrame(t *testing.T) {
	var ctx = &RTPH264Ctx{
		RTPCtx: RTPCtx{
			RTPCodecParameters: webrtc.RTPCodecParameters{
				PayloadType: 96,
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:    webrtc.MimeTypeH264,
					ClockRate:   90000,
					SDPFmtpLine: "packetization-mode=1; sprop-parameter-sets=J2QAKaxWgHgCJ+WagICAgQ==,KO48sA==; profile-level-id=640029",
				},
			},
		},
	}
	var randStr = util.RandomString(1500)
	var avFrame = &pkg.AVFrame{}
	var mem util.Memory
	mem.Append([]byte(randStr))
	avFrame.Raw = pkg.Nalus{
		Nalus: []util.Memory{mem},
	}
	f, err := ctx.CreateFrame(avFrame)
	if err != nil {
		t.Error(err)
		return
	}
	frame := f.(*RTPVideo)
	var track = &pkg.AVTrack{}
	_, _, raw, err := frame.Parse(track)
	if err != nil {
		t.Error(err)
		return
	}
	if s := string(raw.(pkg.Nalus).Nalus[0].ToBytes()); s != randStr {
		t.Error("not equal", len(s), len(randStr))
	}
}
