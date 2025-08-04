# Logging Mechanism

Monibuca uses Go's standard library `slog` as its logging system, providing structured logging functionality.

## Log Configuration

In the global configuration, you can set the log level through the `LogLevel` field. Supported log levels are:

- trace
- debug
- info 
- warn
- error

Configuration example:

```yaml
global:
  LogLevel: "debug" # Set log level to debug
```

## Log Format

The default log format includes the following information:

- Timestamp (format: HH:MM:SS.MICROSECONDS)
- Log level
- Log message
- Structured fields

Example output:
```
15:04:05.123456 INFO server started
15:04:05.123456 ERROR failed to connect database dsn="xxx" type="mysql"
```

## Log Handlers

Monibuca uses `console-slog` as the default log handler, which provides:

1. Color output support
2. Microsecond-level timestamps
3. Structured field formatting

### Multiple Handler Support

Monibuca implements a `MultiLogHandler` mechanism, supporting multiple log handlers simultaneously. This provides the following advantages:

1. Can output logs to multiple targets simultaneously (e.g., console, file, log service)
2. Supports dynamic addition and removal of log handlers
3. Each handler can have its own log level settings
4. Supports log grouping and property inheritance

Through the plugin system, various logging methods can be extended, for example:

- LogRotate plugin: Supports log file rotation
- VMLog plugin: Supports storing logs in VictoriaMetrics time-series database

## Using Logs in Plugins

Each plugin inherits the server's log configuration. Plugins can log using the following methods:

```go
plugin.Info("message", "key1", value1, "key2", value2)  // Log INFO level
plugin.Debug("message", "key1", value1)                 // Log DEBUG level
plugin.Warn("message", "key1", value1)                  // Log WARN level
plugin.Error("message", "key1", value1)                 // Log ERROR level
```

## Log Initialization Process

1. Create default console log handler at server startup
2. Read log level settings from configuration file
3. Apply log level configuration
4. Set inherited log configuration for each plugin

## Best Practices

1. Use Log Levels Appropriately
   - trace: For most detailed tracing information
   - debug: For debugging information
   - info: For important information during normal operation
   - warn: For warning information
   - error: For error information

2. Use Structured Fields
   - Avoid concatenating variables in messages
   - Use key-value pairs to record additional information

3. Error Handling
   - Include complete error information when logging errors
   - Add relevant context information

Example:
```go
// Recommended
s.Error("failed to connect database", "error", err, "dsn", dsn)

// Not recommended
s.Error("failed to connect database: " + err.Error())
```

## Extending the Logging System

To extend the logging system, you can:

1. Implement custom `slog.Handler` interface
2. Use `LogHandler.Add()` method to add new handlers
3. Provide more complex logging functionality through the plugin system

Example of adding a custom log handler:

```go
type MyLogHandler struct {
    slog.Handler
}

// Add handler during plugin initialization
func (p *MyPlugin) Start() error {
    handler := &MyLogHandler{}
    p.Server.LogHandler.Add(handler)
    return nil
}
``` 