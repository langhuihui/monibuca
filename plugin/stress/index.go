package plugin_stress

import (
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
	"m7s.live/m7s/v5/plugin/stress/pb"
	"sync"
)

type StressPlugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	pushers util.Collection[string, *m7s.PushContext]
	pullers util.Collection[string, *m7s.PullContext]
}

var _ = m7s.InstallPlugin[StressPlugin](&pb.Api_ServiceDesc, pb.RegisterApiHandler)

func (r *StressPlugin) OnInit() error {
	r.pushers.L = &sync.RWMutex{}
	r.pullers.L = &sync.RWMutex{}
	return nil
}
