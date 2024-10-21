package plugin_transcode

import (
	"m7s.live/v5"
	"m7s.live/v5/plugin/transcode/pb"
	transcode "m7s.live/v5/plugin/transcode/pkg"
)

var _ = m7s.InstallPlugin[TranscodePlugin](transcode.NewTransform, pb.RegisterApiHandler, &pb.Api_ServiceDesc)

type TranscodePlugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
}
