
# mcp plugin for m7s


## 安装
直接引入即可
```go
import _ "m7s.live/v5/plugin/mcp"
```

## mcp 路由

http://localhost:8080/mcp/sse

## 编译说明
需要开启数据库支持 
- sqlite `go build -tags sqlite main.go` 
- mysql `go build -tags mysql main.go` 

或者其它数据库