package test

import (
	"context"
	"testing"
	"time"

	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg"
)

func TestRestart(b *testing.T) {
	ctx := context.TODO()
	var server = m7s.NewServer()
	go func() {
		time.Sleep(time.Second * 2)
		server.Stop(pkg.ErrRestart)
		time.Sleep(time.Second * 2)
		server.Stop(pkg.ErrRestart)
		time.Sleep(time.Second * 2)
		server.Stop(pkg.ErrStopFromAPI)
	}()
	if server.Run(ctx, "test") != pkg.ErrStopFromAPI {
		b.Error("server.Run should return ErrStopFromAPI")
	}
}
