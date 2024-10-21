package plugin_stress

import (
	"sync"

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

var _ = m7s.InstallPlugin[StressPlugin](&pb.Api_ServiceDesc, pb.RegisterApiHandler)

func (r *StressPlugin) OnInit() error {
	r.pushers.L = &sync.RWMutex{}
	r.pullers.L = &sync.RWMutex{}
	return nil
}
