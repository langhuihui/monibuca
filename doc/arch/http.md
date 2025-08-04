# HTTP Service Mechanism

Monibuca provides comprehensive HTTP service support, including RESTful API, WebSocket, HTTP-FLV, and other protocols. This document details the implementation mechanism and usage of the HTTP service.

## HTTP Configuration

### 1. Configuration Priority

- Plugin HTTP configuration takes precedence over global HTTP configuration
- If a plugin doesn't have HTTP configuration, global HTTP configuration is used

### 2. Configuration Items

```yaml
# Global configuration example
global:
  http:
    listenaddr: :8080  # Listen address and port
    listentlsaddr: :8081 # TLS listen address and port
    certfile: ""       # SSL certificate file path
    keyfile: ""        # SSL key file path
    cors: true        # Whether to allow CORS
    username: ""      # Basic auth username
    password: ""      # Basic auth password

# Plugin configuration example (takes precedence over global config)
plugin_name:
  http:
    listenaddr: :8081
    cors: false
    username: "admin"
    password: "123456"
```

## Service Processing Flow

### 1. Request Processing Order

When the HTTP server receives a request, it processes it in the following order:

1. First attempts to forward to the corresponding gRPC service
2. If no corresponding gRPC service is found, looks for plugin-registered HTTP handlers
3. If nothing is found, returns a 404 error

### 2. Handler Registration Methods

Plugins can register HTTP handlers in two ways:

1. Reflection Registration: The system automatically obtains plugin handling methods through reflection
   - Method names must start with uppercase to be reflected (Go language rule)
   - Usually use `API_` as method name prefix (recommended but not mandatory)
   - Method signature must be `func(w http.ResponseWriter, r *http.Request)`
   - URL path auto-generation rules:
     - Underscores `_` in method names are converted to slashes `/`
     - Example: `API_relay_` method maps to `/API/relay/*` path
   - If a method name ends with underscore, it indicates a wildcard path that matches any subsequent path

2. Manual Registration: Plugin implements `IRegisterHandler` interface for manual registration
   - Lowercase methods can't be reflected, must be registered manually
   - Manual registration can use path parameters (like `:id`)
   - More flexible routing rule configuration

Example code:
```go
// Reflection registration example
type YourPlugin struct {
    // ...
}

// Uppercase start, can be reflected
// Automatically maps to /API/relay/*
func (p *YourPlugin) API_relay_(w http.ResponseWriter, r *http.Request) {
    // Handle wildcard path requests
}

// Lowercase start, can't be reflected, needs manual registration
func (p *YourPlugin) handleUserRequest(w http.ResponseWriter, r *http.Request) {
    // Handle parameterized requests
}

// Manual registration example
func (p *YourPlugin) RegisterHandler() {
    // Can use path parameters
    engine.GET("/api/user/:id", p.handleUserRequest)
}
```

## Middleware Mechanism

### 1. Adding Middleware

Plugins can add global middleware using the `AddMiddleware` method to handle all HTTP requests. Middleware executes in the order it was added.

Example code:
```go
func (p *YourPlugin) Start() {
    // Add authentication middleware
    p.GetCommonConf().AddMiddleware(func(next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            // Execute before request handling
            if !authenticate(r) {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }
            // Call next handler
            next(w, r)
            // Execute after request handling
        }
    })
}
```

### 2. Middleware Use Cases

- Authentication and Authorization
- Request Logging
- CORS Handling
- Request Rate Limiting
- Response Header Setting
- Error Handling
- Performance Monitoring

## Special Protocol Support

### 1. HTTP-FLV

- Supports HTTP-FLV live stream distribution
- Automatically generates FLV headers
- Supports GOP caching
- Supports WebSocket-FLV

### 2. HTTP-MP4

- Supports HTTP-MP4 stream distribution
- Supports fMP4 file distribution

### 3. HLS
- Supports HLS protocol
- Supports MPEG-TS encapsulation

### 4. WebSocket

- Supports custom message protocols
- Supports ws-flv
- Supports ws-mp4 