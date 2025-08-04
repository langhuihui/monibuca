# HTTP 服务机制

Monibuca 提供了完整的 HTTP 服务支持，包括 RESTful API、WebSocket、HTTP-FLV 等多种协议支持。本文档详细说明了 HTTP 服务的实现机制和使用方法。

## HTTP 配置

### 1. 配置优先级

- 插件的 HTTP 配置优先于全局 HTTP 配置
- 如果插件没有配置 HTTP，则使用全局 HTTP 配置

### 2. 配置项说明

```yaml
# 全局配置示例
global:
  http:
    listenaddr: :8080  # 监听地址和端口
    listentlsaddr: :8081 # 监听tls地址和端口
    certfile: ""       # SSL证书文件路径
    keyfile: ""        # SSL密钥文件路径
    cors: true        # 是否允许跨域
    username: ""      # Basic认证用户名
    password: ""      # Basic认证密码

# 插件配置示例（优先于全局配置）
plugin_name:
  http:
    listenaddr: :8081
    cors: false
    username: "admin"
    password: "123456"
```

## 服务处理流程

### 1. 请求处理顺序

HTTP 服务器接收到请求后，按以下顺序处理：

1. 首先尝试转发到对应的 gRPC 服务
2. 如果没有找到对应的 gRPC 服务，则查找插件注册的 HTTP handler
3. 如果都没有找到，返回 404 错误

### 2. Handler 注册方式

插件可以通过以下两种方式注册 HTTP handler：

1. 反射注册：系统自动通过反射获取插件的处理方法
   - 方法名必须大写开头才能被反射获取（Go 语言规则）
   - 通常使用 `API_` 作为方法名前缀（推荐但不强制）
   - 方法签名必须为 `func(w http.ResponseWriter, r *http.Request)`
   - URL 路径自动生成规则：
     - 方法名中的下划线 `_` 会被转换为斜杠 `/`
     - 例如：`API_relay_` 方法将映射到 `/API/relay/*` 路径
   - 如果方法名以下划线结尾，表示这是一个通配符路径，可以匹配后续任意路径

2. 手动注册：插件实现 `IRegisterHandler` 接口进行手动注册
   - 小写开头的方法无法被反射获取，需要通过手动注册方式
   - 手动注册可以使用路径参数（如 `:id`）
   - 更灵活的路由规则配置

示例代码：
```go
// 反射注册示例
type YourPlugin struct {
    // ...
}

// 大写开头，可以被反射获取
// 自动映射到 /API/relay/*
func (p *YourPlugin) API_relay_(w http.ResponseWriter, r *http.Request) {
    // 处理通配符路径的请求
}

// 小写开头，无法被反射获取，需要手动注册
func (p *YourPlugin) handleUserRequest(w http.ResponseWriter, r *http.Request) {
    // 处理带参数的请求
}

// 手动注册示例
func (p *YourPlugin) RegisterHandler() {
    // 可以使用路径参数
    engine.GET("/api/user/:id", p.handleUserRequest)
}
```

## 中间件机制

### 1. 添加中间件

插件可以通过 `AddMiddleware` 方法添加全局中间件，用于处理所有 HTTP 请求。中间件按照添加顺序依次执行。

示例代码：
```go
func (p *YourPlugin) Start() {
    // 添加认证中间件
    p.GetCommonConf().AddMiddleware(func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            // 在请求处理前执行
            if !authenticate(r) {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }
            // 调用下一个处理器
            next(w, r)
            // 在请求处理后执行
        }
    })
}
```

### 2. 中间件使用场景

- 认证和授权
- 请求日志记录
- CORS 处理
- 请求限流
- 响应头设置
- 错误处理
- 性能监控

## 特殊协议支持

### 1. HTTP-FLV

- 支持 HTTP-FLV 直播流分发
- 自动生成 FLV 头
- 支持 GOP 缓存
- 支持 WebSocket-FLV

### 2. HTTP-MP4

- 支持 HTTP-MP4 流分发
- 支持 fMP4 文件分发

### 3. HLS
- 支持 HLS 协议
- 支持 MPEG-TS 封装

### 4. WebSocket

- 支持自定义消息协议
- 支持ws-flv
- 支持ws-mp4
