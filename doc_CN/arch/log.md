# 日志机制

Monibuca 使用 Go 标准库的 `slog` 作为日志系统，提供了结构化的日志记录功能。

## 日志配置

在全局配置中，可以通过 `LogLevel` 字段来设置日志级别。支持的日志级别有：

- trace
- debug
- info 
- warn
- error

配置示例:

```yaml
global:
  LogLevel: "debug" # 设置日志级别为 debug
```

## 日志格式

默认的日志格式包含以下信息：

- 时间戳 (格式: HH:MM:SS.MICROSECONDS)
- 日志级别
- 日志消息
- 结构化字段

示例输出:
```
15:04:05.123456 INFO server started
15:04:05.123456 ERROR failed to connect database dsn="xxx" type="mysql"
```

## 日志处理器

Monibuca 使用 `console-slog` 作为默认的日志处理器，它提供了:

1. 彩色输出支持
2. 微秒级时间戳
3. 结构化字段格式化

### 多处理器支持

Monibuca 实现了 `MultiLogHandler` 机制，支持同时使用多个日志处理器。这提供了以下优势：

1. 可以同时将日志输出到多个目标（如控制台、文件、日志服务等）
2. 支持动态添加和移除日志处理器
3. 每个处理器可以有自己的日志级别设置
4. 支持日志分组和属性继承

通过插件系统，可以扩展多种日志处理方式，例如：

- LogRotate 插件：支持日志文件轮转
- VMLog 插件：支持将日志存储到 VictoriaMetrics 时序数据库

## 在插件中使用日志

每个插件都会继承服务器的日志配置。插件可以通过以下方式记录日志：

```go
plugin.Info("message", "key1", value1, "key2", value2)  // 记录 INFO 级别日志
plugin.Debug("message", "key1", value1)                 // 记录 DEBUG 级别日志
plugin.Warn("message", "key1", value1)                  // 记录 WARN 级别日志
plugin.Error("message", "key1", value1)                 // 记录 ERROR 级别日志
```

## 日志初始化流程

1. 服务器启动时创建默认的控制台日志处理器
2. 从配置文件读取日志级别设置
3. 应用日志级别配置
4. 为每个插件设置继承的日志配置

## 最佳实践

1. 合理使用日志级别
   - trace: 用于最详细的追踪信息
   - debug: 用于调试信息
   - info: 用于正常运行时的重要信息
   - warn: 用于警告信息
   - error: 用于错误信息

2. 使用结构化字段
   - 避免在消息中拼接变量
   - 使用 key-value 对记录额外信息

3. 错误处理
   - 记录错误时包含完整的错误信息
   - 添加相关的上下文信息

示例:
```go
// 推荐
s.Error("failed to connect database", "error", err, "dsn", dsn)

// 不推荐
s.Error("failed to connect database: " + err.Error())
```

## 扩展日志系统

要扩展日志系统，可以通过以下方式：

1. 实现自定义的 `slog.Handler` 接口
2. 使用 `LogHandler.Add()` 方法添加新的处理器
3. 可以通过插件系统提供更复杂的日志功能

例如添加自定义日志处理器：

```go
type MyLogHandler struct {
    slog.Handler
}

// 在插件初始化时添加处理器
func (p *MyPlugin) Start() error {
    handler := &MyLogHandler{}
    p.Server.LogHandler.Add(handler)
    return nil
}
```
