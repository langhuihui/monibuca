package plugin_sei

import (
	"m7s.live/pro"
	"m7s.live/pro/plugin/sei/pb"
	sei "m7s.live/pro/plugin/sei/pkg"
)

var _ = m7s.InstallPlugin[SEIPlugin](sei.NewTransform, pb.RegisterApiServer, &pb.Api_ServiceDesc)

type SEIPlugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
}
