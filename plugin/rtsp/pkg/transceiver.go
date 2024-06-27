package rtsp

import (
	"github.com/pion/webrtc/v4"
	"m7s.live/m7s/v5"
)

type Stream struct {
	*NetConnection
	AudioChannelID byte
	VideoChannelID byte
}
type Sender struct {
	*m7s.Subscriber
	Stream
}

type Receiver struct {
	*m7s.Publisher
	Stream
	AudioCodecParameters *webrtc.RTPCodecParameters
	VideoCodecParameters *webrtc.RTPCodecParameters
}
