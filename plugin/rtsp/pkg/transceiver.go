package rtsp

import (
	"github.com/pion/webrtc/v4"
	"m7s.live/m7s/v5"
)

type Receiver struct {
	*m7s.Publisher
	*NetConnection
	AudioCodecParameters *webrtc.RTPCodecParameters
	VideoCodecParameters *webrtc.RTPCodecParameters
	AudioChannelID       byte
	VideoChannelID       byte
}
