package main

import (
	"context"
	"time"

	"m7s.live/m7s/v5"
	_ "m7s.live/m7s/v5/plugin/demo"
)

func main() {
	ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(time.Second*10))
	m7s.Run(ctx)
}
