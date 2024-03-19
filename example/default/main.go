package main

import (
	"context"

	m7s "m7s.live/monibuca/v5"
	_ "m7s.live/monibuca/v5/example/plugin-demo"
)

func main() {
	m7s.Run(context.Background())
}
