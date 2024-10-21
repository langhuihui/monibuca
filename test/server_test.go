package test

import (
	"errors"
	"testing"
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg"
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
	for err := pkg.ErrRestart; errors.Is(err, pkg.ErrRestart); {
		server = m7s.NewServer(conf)
		err = m7s.Servers.Add(server).WaitStopped()
	}
	//if err := util.RootTask.AddTask(server).WaitStopped(); err != pkg.ErrStopFromAPI {
	//	b.Error("server.Run should return ErrStopFromAPI", err)
	//}
}
