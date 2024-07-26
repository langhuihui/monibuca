
# Introduction
Monibuca is a highly scalable high-performance streaming server development framework developed purely for Go
# Usage

```go
package main

import (
	"context"

	"m7s.live/m7s/v5"
	_ "m7s.live/m7s/v5/plugin/debug"
	_ "m7s.live/m7s/v5/plugin/flv"
	_ "m7s.live/m7s/v5/plugin/rtmp"
)

func main() {
	m7s.Run(context.Background(), "config.yaml")
}

```


## More Example

see example directory

# Create Plugin

```go

import (
	"m7s.live/m7s/v5"
)

type MyPlugin struct {
	m7s.Plugin
}

var _ = m7s.InstallPlugin[MyPlugin]()
```