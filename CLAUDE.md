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

**Task System (`pkg/task/`):** Advanced asynchronous task management system with multiple layers:
- **Task:** Basic unit of work with lifecycle management (Start/Run/Dispose)
- **Job:** Container that manages multiple child tasks and provides event loops
- **Work:** Special type of Job that acts as a persistent queue manager (keepalive=true)
- **Channel:** Event-driven task for handling continuous data streams

### Task System Deep Dive

#### Task Hierarchy and Lifecycle
```
Work (Queue Manager)
  └── Job (Container with Event Loop)
      └── Task (Basic Work Unit)
          ├── Start() - Initialization phase
          ├── Run() - Main execution phase
          └── Dispose() - Cleanup phase
```

#### Queue-based Asynchronous Processing
The Task system supports sophisticated queue-based processing patterns:

1. **Work as Queue Manager:** Work instances stay alive indefinitely and manage queues of tasks
2. **Task Queuing:** Use `workInstance.AddTask(task, logger)` to queue tasks
3. **Automatic Lifecycle:** Tasks are automatically started, executed, and disposed
4. **Error Handling:** Built-in retry mechanisms and error propagation

**Example Pattern (from S3 plugin):**
```go
type UploadQueueTask struct {
    task.Work  // Persistent queue manager
}

type FileUploadTask struct {
    task.Task  // Individual work item
    // ... task-specific fields
}

// Initialize queue manager (typically in init())
var uploadQueueTask UploadQueueTask
m7s.Servers.AddTask(&uploadQueueTask)

// Queue individual tasks
uploadQueueTask.AddTask(&FileUploadTask{...}, logger)
```

#### Cross-Plugin Task Cooperation
Tasks can coordinate across different plugins through:

1. **Global Instance Pattern:** Plugins expose global instances for cross-plugin access
2. **Event-based Triggers:** One plugin triggers tasks in another plugin
3. **Shared Queue Managers:** Multiple plugins can use the same Work instance

**Example (MP4 → S3 Integration):**
```go
// In MP4 plugin: trigger S3 upload after recording completes
s3plugin.TriggerUpload(filePath, deleteAfter)

// S3 plugin receives trigger and queues upload task
func TriggerUpload(filePath string, deleteAfter bool) {
    if s3PluginInstance != nil {
        s3PluginInstance.QueueUpload(filePath, objectKey, deleteAfter)
    }
}
```

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

### Post-Recording Workflow

Monibuca implements a sophisticated post-recording processing pipeline:

1. **Recording Completion:** MP4 recorder finishes writing stream data
2. **Trailer Writing:** Asynchronous task moves MOOV box to file beginning for web compatibility
3. **File Optimization:** Temporary file operations ensure atomic updates
4. **External Storage Integration:** Automatic upload to S3-compatible services
5. **Cleanup:** Optional local file deletion after successful upload

This workflow uses queue-based task processing to avoid blocking the main recording pipeline.

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

### Cross-Plugin Communication Patterns

#### 1. Global Instance Pattern
```go
// Expose global instance for cross-plugin access
var s3PluginInstance *S3Plugin

func (p *S3Plugin) Start() error {
    s3PluginInstance = p  // Set global instance
    // ... rest of start logic
}

// Provide public API functions
func TriggerUpload(filePath string, deleteAfter bool) {
    if s3PluginInstance != nil {
        s3PluginInstance.QueueUpload(filePath, objectKey, deleteAfter)
    }
}
```

#### 2. Event-Driven Integration
```go
// In one plugin: trigger event after completion
if t.filePath != "" {
    t.Info("MP4 file processing completed, triggering S3 upload")
    s3plugin.TriggerUpload(t.filePath, false)
}
```

#### 3. Shared Queue Managers
Multiple plugins can share Work instances for coordinated processing.

### Asynchronous Task Development Best Practices

#### 1. Implement Task Interfaces
```go
type MyTask struct {
    task.Task
    // ... custom fields
}

func (t *MyTask) Start() error {
    // Initialize resources, validate inputs
    return nil
}

func (t *MyTask) Run() error {
    // Main work execution
    // Return task.ErrTaskComplete for successful completion
    return nil
}
```

#### 2. Use Work for Queue Management
```go
type MyQueueManager struct {
    task.Work
}

var myQueue MyQueueManager

func init() {
    m7s.Servers.AddTask(&myQueue)
}

// Queue tasks from anywhere
myQueue.AddTask(&MyTask{...}, logger)
```

#### 3. Error Handling and Retry
- Tasks automatically support retry mechanisms
- Use `task.SetRetry(maxRetry, interval)` for custom retry behavior
- Return `task.ErrTaskComplete` for successful completion
- Return other errors to trigger retry or failure handling

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
- **MP4:** MPEG-4 format with post-processing capabilities
- **SRT:** Secure reliable transport
- **S3:** File upload integration with AWS S3/MinIO compatibility

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

### Async Task Development
- Always use Work instances for queue management
- Implement proper Start/Run lifecycle in tasks
- Use global instance pattern for cross-plugin communication
- Handle errors gracefully with appropriate retry strategies

### Performance Considerations
- Memory pool is enabled by default (disable with `disable_rm`)
- Zero-copy design for media data where possible
- Lock-free data structures for high concurrency
- Efficient buffer management with ring buffers
- Queue-based processing prevents blocking main threads

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

### Task System Debugging
- Tasks automatically include detailed logging with task IDs and types
- Use `task.Debug/Info/Warn/Error` methods for consistent logging
- Task state and progress can be monitored through descriptions
- Event loop status and queue lengths are logged automatically

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

### Task System Issues
- Ensure Work instances are added to server during initialization
- Check task queue status if tasks aren't executing
- Verify proper error handling in task implementation
- Monitor task retry counts and failure reasons in logs