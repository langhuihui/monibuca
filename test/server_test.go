package test

import (
	"context"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg"
	"testing"
	"time"
)

func TestRestart(b *testing.T) {
	ctx := context.TODO()
	var server = m7s.NewServer()
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
	if err := server.Run(ctx, map[string]map[string]any{"global": {"loglevel": "debug"}}); err != pkg.ErrStopFromAPI {
		b.Error("server.Run should return ErrStopFromAPI", err)
	}
}
