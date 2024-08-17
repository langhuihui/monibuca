package test

import (
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg"
	"testing"
	"time"
)

func TestRestart(b *testing.T) {
	conf := m7s.RawConfig{"global": {"loglevel": "debug"}}
	var server *m7s.Server
	go func() {
		time.Sleep(time.Second * 2)
		server.Stop(pkg.ErrRestart)
		b.Log("server stop1")
		time.Sleep(time.Second * 2)
		server.Stop(pkg.ErrRestart)
		b.Log("server stop2")
		time.Sleep(time.Second * 2)
		server.Stop(pkg.ErrStopFromAPI)
		b.Log("server stop3")
	}()
	for {
		server = m7s.NewServer(conf)
		if err := m7s.AddRootTask(server).WaitStopped(); err != pkg.ErrRestart {
			return
		}
	}
	//if err := util.RootTask.AddTask(server).WaitStopped(); err != pkg.ErrStopFromAPI {
	//	b.Error("server.Run should return ErrStopFromAPI", err)
	//}
}
