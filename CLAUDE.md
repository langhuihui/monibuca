# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Monibuca is a high-performance streaming server framework written in Go. It's designed to be a modular, scalable platform for real-time audio/video streaming with support for multiple protocols including RTMP, RTSP, HLS, WebRTC, GB28181, and more.

## Development Commands

### Building and Running

**Basic Run (with SQLite):**
```bash
cd example/default
go run -tags sqlite main.go
```

**Build Tags:**
- `sqlite` - Enable SQLite database support
- `sqliteCGO` - Enable SQLite with CGO
- `mysql` - Enable MySQL database support  
- `postgres` - Enable PostgreSQL database support
- `duckdb` - Enable DuckDB database support
- `disable_rm` - Disable memory pool
- `fasthttp` - Use fasthttp instead of net/http
- `taskpanic` - Enable panics for testing

**Protocol Buffer Generation:**
```bash
# Generate all proto files
sh scripts/protoc.sh

# Generate specific plugin proto
sh scripts/protoc.sh plugin_name
```

**Release Building:**
```bash
# Uses goreleaser configuration
goreleaser build
```

**Testing:**
```bash
go test ./...
```

## Architecture Overview

### Core Components

**Server (`server.go`):** Main server instance that manages plugins, streams, and configurations. Implements the central event loop and lifecycle management.

**Plugin System (`plugin.go`):** Modular architecture where functionality is provided through plugins. Each plugin implements the `IPlugin` interface and can provide:
- Protocol handlers (RTMP, RTSP, etc.)
- Media transformers
- Pull/Push proxies
- Recording capabilities
- Custom HTTP endpoints

**Configuration System (`pkg/config/`):** Hierarchical configuration system with priority order: dynamic modifications > environment variables > config files > default YAML > global config > defaults.

**Task System (`pkg/task/`):** Asynchronous task management with dependency handling, lifecycle management, and graceful shutdown capabilities.

### Key Interfaces

**Publisher:** Handles incoming media streams and manages track information
**Subscriber:** Handles outgoing media streams to clients  
**Puller:** Pulls streams from external sources
**Pusher:** Pushes streams to external destinations
**Transformer:** Processes/transcodes media streams
**Recorder:** Records streams to storage

### Stream Processing Flow

1. **Publisher** receives media data and creates tracks
2. **Tracks** handle audio/video data with specific codecs
3. **Subscribers** attach to publishers to receive media
4. **Transformers** can process streams between publishers and subscribers
5. **Plugins** provide protocol-specific implementations

## Plugin Development

### Creating a Plugin

1. Implement the `IPlugin` interface
2. Define plugin metadata using `PluginMeta`
3. Register with `InstallPlugin[YourPluginType](meta)`
4. Optionally implement protocol-specific interfaces:
   - `ITCPPlugin` for TCP servers
   - `IUDPPlugin` for UDP servers  
   - `IQUICPlugin` for QUIC servers
   - `IRegisterHandler` for HTTP endpoints

### Plugin Lifecycle

1. **Init:** Configuration parsing and initialization
2. **Start:** Network listeners and task registration
3. **Run:** Active operation
4. **Dispose:** Cleanup and shutdown

## Configuration Structure

### Global Configuration
- HTTP/TCP/UDP/QUIC listeners
- Database connections (SQLite, MySQL, PostgreSQL, DuckDB)
- Authentication settings
- Admin interface settings
- Global stream alias mappings

### Plugin Configuration
Each plugin can define its own configuration structure that gets merged with global settings.

## Database Integration

Supports multiple database backends:
- **SQLite:** Default lightweight option
- **MySQL:** Production deployments
- **PostgreSQL:** Production deployments  
- **DuckDB:** Analytics use cases

Automatic migration is handled for core models including users, proxies, and stream aliases.

## Protocol Support

### Built-in Plugins
- **RTMP:** Real-time messaging protocol
- **RTSP:** Real-time streaming protocol
- **HLS:** HTTP live streaming
- **WebRTC:** Web real-time communication
- **GB28181:** Chinese surveillance standard
- **FLV:** Flash video format
- **MP4:** MPEG-4 format
- **SRT:** Secure reliable transport

## Authentication & Security

- JWT-based authentication for admin interface
- Stream-level authentication with URL signing
- Role-based access control (admin/user)
- Webhook support for external auth integration

## Development Guidelines

### Code Style
- Follow existing patterns and naming conventions
- Use the task system for async operations
- Implement proper error handling and logging
- Use the configuration system for all settings

### Testing
- Unit tests should be placed alongside source files
- Integration tests can use the example configurations
- Use the mock.py script for protocol testing

### Performance Considerations
- Memory pool is enabled by default (disable with `disable_rm`)
- Zero-copy design for media data where possible
- Lock-free data structures for high concurrency
- Efficient buffer management with ring buffers

## Debugging

### Built-in Debug Plugin
- Performance monitoring and profiling
- Real-time metrics via Prometheus endpoint (`/api/metrics`)
- pprof integration for memory/cpu profiling

### Logging
- Structured logging with zerolog
- Configurable log levels
- Log rotation support
- Fatal crash logging

## Web Admin Interface

- Web-based admin UI served from `admin.zip`
- RESTful API for all operations
- Real-time stream monitoring
- Configuration management
- User management (when auth enabled)

## Common Issues

### Port Conflicts
- Default HTTP port: 8080
- Default gRPC port: 50051
- Check plugin-specific port configurations

### Database Connection
- Ensure proper build tags for database support
- Check DSN configuration strings
- Verify database file permissions

### Plugin Loading
- Plugins are auto-discovered from imports
- Check plugin enable/disable status
- Verify configuration merging