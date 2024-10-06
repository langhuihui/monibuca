
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
## build tags

| Build Tag | Description |
|-----------|-------------|
| disable_rm | Disables the memory pool |
| sqlite | Enables the sqlite DB |
| duckdb | Enables the duckdb DB |
| taskpanic | Throws panic, for testing |

## More Example

see example directory

# Prometheus

```yaml
scrape_configs:
  - job_name: "monibuca"
    metrics_path: "/api/metrics"
    static_configs:
      - targets: ["localhost:8080"]
```

# Create Plugin

see [plugin](./plugin/README.md)