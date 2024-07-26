# 介绍
monibuca 是一款纯 go 开发的扩展性极强的高性能流媒体服务器开发框架

# 使用
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

## 更多示例

查看 example 目录

# 创建插件

```go

import (
	"m7s.live/m7s/v5"
)

type MyPlugin struct {
	m7s.Plugin
}

var _ = m7s.InstallPlugin[MyPlugin]()
```