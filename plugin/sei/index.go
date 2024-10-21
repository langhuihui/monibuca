package plugin_sei

import (
	"m7s.live/v5"
	"m7s.live/v5/plugin/sei/pb"
	sei "m7s.live/v5/plugin/sei/pkg"
)

var _ = m7s.InstallPlugin[SEIPlugin](sei.NewTransform, pb.RegisterApiServer, &pb.Api_ServiceDesc)

type SEIPlugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
}
