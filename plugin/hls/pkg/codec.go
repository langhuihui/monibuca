package hls

import (
	"m7s.live/v5/pkg"
	mpegts "m7s.live/v5/pkg/format/ts"
)

type VideoCodecCtx struct {
	pkg.IVideoCodecCtx
	mpegts.MpegtsPESFrame
}
