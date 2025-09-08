# Gemini Context: Monibuca Project

This document provides a summary of the Monibuca project to give context for AI-assisted development.

## Project Overview

Monibuca is a modular, high-performance streaming media server framework written in Go. Its core design is lightweight and plugin-based, allowing developers to extend functionality by adding or developing plugins for different streaming protocols and features. The project's module path is `m7s.live/v4`.

The architecture is centered around a core engine (`m7s.live/v4`) that manages plugins, streams, and the main event loop. Functionality is added by importing plugins, which register themselves with the core engine.

**Key Technologies:**
- **Language:** Go
- **Architecture:** Plugin-based
- **APIs:** RESTful HTTP API, gRPC API

**Supported Protocols (based on plugins):**
- RTMP
- RTSP
- HLS
- FLV
- WebRTC
- GB28181
- SRT
- And more...

## Building and Running

### Build
To build the server, run the following command from the project root:
```bash
go build -v .
```

### Test
To run the test suite:
```bash
go test -v ./...
```

### Running the Server
The server is typically run by creating a `main.go` file that imports the core engine and the desired plugins.

**Example `main.go`:**
```go
package main

import (
	"m7s.live/v4"
	// Import desired plugins to register them
	_ "m7s.live/plugin/rtmp/v4"
	_ "m7s.live/plugin/rtsp/v4"
	_ "m7s.live/plugin/hls/v4"
	_ "m7s.live/plugin/webrtc/v4"
)

func main() {
	m7s.Run()
}
```
The server is executed by running `go run main.go`. Configuration is managed through a `config.yaml` file in the same directory.

### Docker
The project includes a `Dockerfile` to build and run in a container.
```bash
# Build the image
docker build -t monibuca .

# Run the container
docker run -p 8080:8080 monibuca
```

## Development Conventions

### Project Structure
- `server.go`: Core engine logic.
- `plugin/`: Contains individual plugins for different protocols and features.
- `pkg/`: Shared packages and utilities used across the project.
- `pb/`: Protobuf definitions for the gRPC API.
- `example/`: Example implementations and configurations.
- `doc/`: Project documentation.

### Plugin System
The primary way to add functionality is by creating or enabling plugins. A plugin is a Go package that registers itself with the core engine upon import (using the `init()` function). This modular approach keeps the core small and allows for custom builds with only the necessary features.

### API
- **RESTful API:** Defined in `api.go`, provides HTTP endpoints for controlling and monitoring the server.
- **gRPC API:** Defined in the `pb/` directory using protobuf. `protoc.sh` is used to generate the Go code from the `.proto` files.

### Code Style and CI
- The project uses `golangci-lint` for linting, as seen in the `.github/workflows/go.yml` file.
- Static analysis is configured via `staticcheck.conf` and `qodana.yaml`.
- All code should be formatted with `gofmt`.
