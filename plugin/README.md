# Plugin Development Guide

## 1. Prerequisites

### Development Tools
- Visual Studio Code
- Goland
- Cursor

### Install gRPC
```shell
$ go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
$ go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### Install gRPC-Gateway
```shell
$ go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
$ go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
$ go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### Project Setup
- Create a Go project, e.g., `MyPlugin`
- Create a `pkg` directory for exportable code
- Create a `pb` directory for gRPC proto files
- Create an `example` directory for testing the plugin

> You can also create a directory `xxx` directly in the monibuca project's plugin folder to store your plugin code

## 2. Create a Plugin

```go
package plugin_myplugin
import (
    "m7s.live/v5"
)

var _ = m7s.InstallPlugin[MyPlugin]()

type MyPlugin struct {
    m7s.Plugin
    Foo string
}
```
- `MyPlugin` struct is the plugin definition, `Foo` is a plugin property that can be configured in the configuration file
- Must embed `m7s.Plugin` struct to provide basic plugin functionality
- `m7s.InstallPlugin[MyPlugin](...)` registers the plugin so it can be loaded by monibuca

### Provide Default Configuration
Example:
```go
const defaultConfig = m7s.DefaultYaml(`tcp:
  listenaddr: :5554`)

var _ = m7s.InstallPlugin[MyPlugin](m7s.PluginMeta{
    DefaultYaml: defaultConfig,
})
```

## 3. Implement Event Callbacks (Optional)

### Initialization Callback
```go
func (config *MyPlugin) Start() (err error) {
    // Initialize things
    return
}
```
Used for plugin initialization after configuration is loaded. Return an error if initialization fails, and the plugin will be disabled.

### TCP Request Callback
```go
func (config *MyPlugin) OnTCPConnect(conn *net.TCPConn) task.ITask {
    
}
```
Called when receiving TCP connection requests if TCP listening port is configured.

### UDP Request Callback
```go
func (config *MyPlugin) OnUDPConnect(conn *net.UDPConn) task.ITask {

}
```
Called when receiving UDP connection requests if UDP listening port is configured.

### QUIC Request Callback
```go
func (config *MyPlugin) OnQUICConnect(quic.Connection) task.ITask {

}
```
Called when receiving QUIC connection requests if QUIC listening port is configured.

## 4. HTTP Interface Callbacks

### Legacy v4 Callback Style
```go
func (config *MyPlugin) API_test1(rw http.ResponseWriter, r *http.Request) {
    // do something
}
```
Accessible via `http://ip:port/myplugin/api/test1`

### Route Mapping Configuration
This method supports parameterized routing:
```go
func (config *MyPlugin) RegisterHandler() map[string]http.HandlerFunc {
    return map[string]http.HandlerFunc{
        "/test1/{streamPath...}": config.test1,
    }
}
func (config *MyPlugin) test1(rw http.ResponseWriter, r *http.Request) {
    streamPath := r.PathValue("streamPath")
    // do something
}
```

## 5. Implement Push/Pull Clients

### Implement Push Client
Push client needs to implement IPusher interface and pass the creation method to InstallPlugin.
```go
type Pusher struct {
    task.Task
    pushJob m7s.PushJob
}

func (c *Pusher) GetPushJob() *m7s.PushJob {
    return &c.pushJob
}

func NewPusher(_ config.Push) m7s.IPusher {
    return &Pusher{}
}
var _ = m7s.InstallPlugin[MyPlugin](m7s.PluginMeta{
    NewPusher: NewPusher,
})
```

### Implement Pull Client
Pull client needs to implement IPuller interface and pass the creation method to InstallPlugin.
The following Puller inherits from m7s.HTTPFilePuller for basic file and HTTP pulling. You need to override the Start method for specific pulling logic:
```go
type Puller struct {
    m7s.HTTPFilePuller
}

func NewPuller(_ config.Pull) m7s.IPuller {
    return &Puller{}
}
var _ = m7s.InstallPlugin[MyPlugin](m7s.PluginMeta{
    NewPuller: NewPuller,
})
```

## 6. Implement gRPC Service

### Create `myplugin.proto` in `pb` Directory
```proto
syntax = "proto3";
import "google/api/annotations.proto";
import "google/protobuf/empty.proto";
package myplugin;
option go_package="m7s.live/v5/plugin/myplugin/pb";

service api {
    rpc MyMethod (MyRequest) returns (MyResponse) {
        option (google.api.http) = {
            post: "/myplugin/api/bar"
            body: "foo"
        };
    }
}
message MyRequest {
    string foo = 1;
}
message MyResponse {
    string bar = 1;
}
```

### Generate gRPC Code
Add to VSCode task.json:
```json
{
    "type": "shell",
    "label": "build pb myplugin",
    "command": "protoc",
    "args": [
        "-I.",
        "-I${workspaceRoot}/pb",
        "--go_out=.",
        "--go_opt=paths=source_relative",
        "--go-grpc_out=.",
        "--go-grpc_opt=paths=source_relative",
        "--grpc-gateway_out=.",
        "--grpc-gateway_opt=paths=source_relative",
        "myplugin.proto"
    ],
    "options": {
        "cwd": "${workspaceRoot}/plugin/myplugin/pb"
    }
}
```
Or run command in pb directory:
```shell
protoc -I. -I$ProjectFileDir$/pb --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative --grpc-gateway_out=. --grpc-gateway_opt=paths=source_relative  myplugin.proto
```
Replace `$ProjectFileDir$` with the directory containing global pb files.

### Implement gRPC Service
Create api.go:
```go
package plugin_myplugin
import (
    "context"
    "m7s.live/m7s/v5"
    "m7s.live/m7s/v5/plugin/myplugin/pb"
)

func (config *MyPlugin) MyMethod(ctx context.Context, req *pb.MyRequest) (*pb.MyResponse, error) {
    return &pb.MyResponse{Bar: req.Foo}, nil
}
```

### Register gRPC Service
```go
package plugin_myplugin
import (
    "m7s.live/v5"
    "m7s.live/v5/plugin/myplugin/pb"
)

var _ = m7s.InstallPlugin[MyPlugin](m7s.PluginMeta{
	ServiceDesc:         &pb.Api_ServiceDesc,
	RegisterGRPCHandler: pb.RegisterApiHandler,
})

type MyPlugin struct {
    pb.UnimplementedApiServer
    m7s.Plugin
    Foo string
}
```

### Additional RESTful Endpoints
Same as v4:
```go
func (config *MyPlugin) API_test1(rw http.ResponseWriter, r *http.Request) {
    // do something
}
```
Accessible via GET request to `/myplugin/api/test1`

## 7. Publishing Streams

```go
publisher, err = p.Publish(streamPath, connectInfo)
```
The last two parameters are optional.

After obtaining the `publisher`, you can publish audio/video data using `publisher.WriteAudio` and `publisher.WriteVideo`.

### Define Audio/Video Data
If existing audio/video data formats don't meet your needs, you can define custom formats by implementing this interface:
```go
IAVFrame interface {
    GetSample() *Sample
    GetSize() int
    CheckCodecChange() error
    Demux() error      // demux to raw format
    Mux(*Sample) error // mux from origin format
    Recycle()
    String() string
}
```
> Define separate types for audio and video

The methods serve the following purposes:
- GetSample: Gets the Sample object containing codec context and raw data
- GetSize: Gets the size of audio/video data
- CheckCodecChange: Checks if the codec has changed
- Demux: Demuxes audio/video data to raw format for use by other formats
- Mux: Muxes from original format to custom audio/video data format
- Recycle: Recycles resources, automatically implemented when embedding RecyclableMemory
- String: Prints audio/video data information

## 8. Subscribing to Streams
```go
var suber *m7s.Subscriber
suber, err = p.Subscribe(ctx,streamPath)
go m7s.PlayBlock(suber, handleAudio, handleVideo)
```
Note that handleAudio and handleVideo are callback functions you need to implement. They take an audio/video format type as input and return an error. If the error is not nil, the subscription is terminated.

## 9. Prometheus Integration
Just implement the Collector interface, and the system will automatically collect metrics from all plugins:
```go
func (p *MyPlugin) Describe(ch chan<- *prometheus.Desc) {
  
}

func (p *MyPlugin) Collect(ch chan<- prometheus.Metric) {
  
}
``` 