package rtp

import (
	"testing"

	"github.com/pion/webrtc/v3"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/util"
)

func TestRTPH264Ctx_CreateFrame(t *testing.T) {
	var ctx = &H264Ctx{}
	ctx.RTPCodecParameters = webrtc.RTPCodecParameters{
		PayloadType: 96,
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "packetization-mode=1; sprop-parameter-sets=J2QAKaxWgHgCJ+WagICAgQ==,KO48sA==; profile-level-id=640029",
		},
	}
	var randStr = util.RandomString(1500)
	var avFrame = &pkg.AVFrame{}
	var mem util.Memory
	mem.Append([]byte(randStr))
	avFrame.Raw = []util.Memory{mem}
	frame := new(Video)
	frame.Mux(ctx, avFrame)
	var track = &pkg.AVTrack{}
	err := frame.Parse(track)
	if err != nil {
		t.Error(err)
		return
	}
	if s := string(track.Value.Raw.(pkg.Nalus)[0].ToBytes()); s != randStr {
		t.Error("not equal", len(s), len(randStr))
	}
}
