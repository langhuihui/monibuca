# 插件开发指南

## 1. 准备工作

### 开发工具
- Visual Studio Code
- Goland
- Cursor
### 安装gRPC
```shell
$ go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
$ go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2
```

### 安装gRPC-Gateway
```shell
$ go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
$ go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
$ go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

### 创建工程
- 创建一个go 工程，例如：`MyPlugin`
- 创建目录`pkg`，用来存放可导出的代码
- 创建目录`pb`，用来存放gRPC的proto文件
- 创建目录`example`， 用来测试插件

> 也可以直接在 monibuca 项目的 plugin 中创建一个目录`xxx`, 用来存放插件代码
## 2. 创建插件

```go
package plugin_myplugin
import (
    "m7s.live/m7s/v5"
)

var _ = m7s.InstallPlugin[MyPlugin]()

type MyPlugin struct {
	m7s.Plugin
	Foo string
}

```
- MyPlugin 结构体就是插件定义，Foo 是插件的一个属性，可以在配置文件中配置。
- 必须嵌入`m7s.Plugin`结构体，这样插件就具备了插件的基本功能。
- `m7s.InstallPlugin[MyPlugin](...)` 用来注册插件，这样插件就可以被 monibuca 加载。
### 传入默认配置
例如：
```go
const defaultConfig = m7s.DefaultYaml(`tcp:
  listenaddr: :5554`)

var _ = m7s.InstallPlugin[MyPlugin](defaultConfig)
```
## 3. 实现事件回调（可选）
### 初始化回调
```go
func (config *MyPlugin) OnInit() (err error) {
    // 初始化一些东西
    return
}
```
用于插件的初始化，此时插件的配置已经加载完成，可以在这里做一些初始化工作。返回错误则插件初始化失败，插件将进入禁用状态。

### 接受 TCP 请求回调

```go
func (config *MyPlugin) OnTCPConnect(conn *net.TCPConn) task.ITask {
	
}
```
当配置了 tcp 监听端口后，收到 tcp 连接请求时，会调用此回调。

### 接受 UDP 请求回调
```go
func (config *MyPlugin) OnUDPConnect(conn *net.UDPConn) task.ITask {

}
```
当配置了 udp 监听端口后，收到 udp 连接请求时，会调用此回调。

### 接受 QUIC 请求回调
```go
func (config *MyPlugin) OnQUICConnect(quic.Connection) task.ITask {

}
```
当配置了 quic 监听端口后，收到 quic 连接请求时，会调用此回调。

## 4. HTTP 接口回调
### 延续 v4 的回调
```go
func (config *MyPlugin) API_test1(rw http.ResponseWriter, r *http.Request) {
	        // do something
}
```
可以通过`http://ip:port/myplugin/api/test1`来访问`API_test1`方法。

### 通过配置映射表
这种方式可以实现带参数的路由，例如：
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
## 5. 实现推拉流客户端

### 实现推流客户端
推流客户端就是想要实现一个 IPusher，然后将创建 IPusher 的方法传入 InstallPlugin 中。
```go
type Pusher struct {
  pullCtx m7s.PullJob
}

func (c *Pusher) GetPullJob() *m7s.PullJob {
	return &c.pullCtx
}

func NewPusher(_ config.Push) m7s.IPusher {
	return &Pusher{}
}
var _ = m7s.InstallPlugin[MyPlugin](NewPusher)

```

### 实现拉流客户端
拉流客户端就是想要实现一个 IPuller，然后将创建 IPuller 的方法传入 InstallPlugin 中。
下面这个 Puller 继承了 m7s.HTTPFilePuller，可以实现基本的文件和 HTTP拉流。具体拉流逻辑需要覆盖 Run 方法。
```go
type Puller struct {
	m7s.HTTPFilePuller
}

func NewPuller(_ config.Pull) m7s.IPuller {
	return &Puller{}
}
var _ = m7s.InstallPlugin[MyPlugin](NewPuller)
```

## 6. 实现gRPC服务
实现 gRPC 可以自动生成对应的 restFul 接口，方便调用。
### 在`pb`目录下创建`myplugin.proto`文件
```proto
syntax = "proto3";
import "google/api/annotations.proto";
import "google/protobuf/empty.proto";
package myplugin;
option go_package="m7s.live/m7s/v5/plugin/myplugin/pb";

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
以上的定义只中包含了实现对应 restFul 的路由，可以通过 post 请求`/myplugin/api/bar`来调用`MyMethod`方法。
### 生成gRPC代码
- 可以使用 vscode 的 task.json中加入
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
    },
```
- 或者在 pb 目录下运行命令行:
```shell
protoc -I. -I$ProjectFileDir$/pb --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative --grpc-gateway_out=. --grpc-gateway_opt=paths=source_relative  myplugin.proto
```
把其中的 $ProjectFileDir$ 替换成包含全局 pb 的目录，全局 pb 文件就在 monibuca 项目的 pb 目录下。

### 实现gRPC服务
创建 api.go 文件
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
### 注册gRPC服务
```go
package plugin_myplugin
import (
    "m7s.live/m7s/v5"
	"m7s.live/m7s/v5/plugin/myplugin/pb"
)

var _ = m7s.InstallPlugin[MyPlugin](&pb.Api_ServiceDesc, pb.RegisterApiHandler)

type MyPlugin struct {
	pb.UnimplementedApiServer
	m7s.Plugin
	Foo string
}

```
### 额外的 restFul 接口
和 v4 相同
```go
func (config *MyPlugin)  API_test1(rw http.ResponseWriter, r *http.Request) {
	        // do something
}
```
就可以通过 get 请求`/myplugin/api/test1`来调用`API_test1`方法。

## 5. 发布流

```go

publisher, err = p.Publish(streamPath, connectInfo)
```
后面两个入参是可选的

得到 `publisher` 后，就可以通过调用 `publisher.WriteAudio`、`publisher.WriteVideo` 来发布音视频数据。
### 定义音视频数据
如果先有的音视频数据格式无法满足需求，可以自定义音视频数据格式。
但需要满足转换格式的要求。即需要实现下面这个接口：
```go
IAVFrame interface {
    GetAllocator() *util.ScalableMemoryAllocator
    SetAllocator(*util.ScalableMemoryAllocator)
    Parse(*AVTrack) error                                          // get codec info, idr
    ConvertCtx(codec.ICodecCtx) (codec.ICodecCtx, IAVFrame, error) // convert codec from source stream
    Demux(codec.ICodecCtx) (any, error)                            // demux to raw format
    Mux(codec.ICodecCtx, *AVFrame)                                 // mux from raw format
    GetTimestamp() time.Duration
    GetCTS() time.Duration
    GetSize() int
    Recycle()
    String() string
    Dump(byte, io.Writer)
}
```
> 音频和视频需要定义两个不同的类型

其中 `Parse` 方法用于解析音视频数据，`ConvertCtx` 方法用于转换音视频数据格式的上下文，`Demux` 方法用于解封装音视频数据，`Mux` 方法用于封装音视频数据，`Recycle` 方法用于回收资源。
- GetAllocator 方法用于获取内存分配器。(嵌入 RecyclableMemory 会自动实现)
- SetAllocator 方法用于设置内存分配器。(嵌入 RecyclableMemory 会自动实现)
- Parse方法主要从数据中识别关键帧，序列帧等重要信息。
- ConvertCtx 会在需要转换协议的时候调用，传入原始的协议上下文，返回新的协议上下文（即自定义格式的上下文）。
- Demux 会在需要解封装音视频数据的时候调用，传入协议上下文，返回解封装后的音视频数据，用于给其他格式封装使用。
- Mux 会在需要封装音视频数据的时候调用，传入协议上下文和解封装后的音视频数据，用于封装成自定义格式的音视频数据。
- Recycle 方法会在嵌入 RecyclableMemory 时自动实现，无需手动实现。
- String 方法用于打印音视频数据的信息。
- GetSize 方法用于获取音视频数据的大小。
- GetTimestamp 方法用于获取音视频数据的时间戳(单位：纳秒)。
- GetCTS 方法用于获取音视频数据的Composition Time Stamp(单位：纳秒)。PTS = DTS+CTS
- Dump 方法用于打印音视频数据的二进制数据。

### 6. 订阅流
```go
var suber *m7s.Subscriber
suber, err = p.Subscribe(ctx,streamPath)
go m7s.PlayBlock(suber, handleAudio, handleVideo)
```
这里需要注意的是 handleAudio, handleVideo 是处理音视频数据的回调函数，需要自己实现。
handleAudio/Video 的入参是一个你需要接受到的音视频格式类型,返回 error，如果返回的 error 不是 nil，则订阅中止。