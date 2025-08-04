package plugin_stress

import (
	"m7s.live/v5"
	"m7s.live/v5/pkg/util"
	"m7s.live/v5/plugin/stress/pb"
)

type StressPlugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	pushers util.Collection[string, *m7s.PushJob]
	pullers util.Collection[string, *m7s.PullJob]
}

var _ = m7s.InstallPlugin[StressPlugin](m7s.PluginMeta{
	ServiceDesc:         &pb.Api_ServiceDesc,
	RegisterGRPCHandler: pb.RegisterApiHandler,
})

func (r *StressPlugin) Start() error {
	return nil
}
