# FFmpeg 插件

FFmpeg 插件用于在 Monibuca 中创建和管理 FFmpeg 进程。该插件提供 gRPC 接口，允许用户创建、更新、查询、重启和关闭 FFmpeg 进程。所有 FFmpeg 进程配置将存储在数据库中，以便反复使用。

## 功能特性

- 创建 FFmpeg 进程配置并持久化存储
- 更新现有 FFmpeg 进程配置
- 查询 FFmpeg 进程（包括运行中和未运行的）
- 启动/重启 FFmpeg 进程
- 停止 FFmpeg 进程
- 在 task 框架中运行 FFmpeg 进程

## API 接口

### 创建 FFmpeg 进程

```
POST /ffmpeg/api/process
```

### 更新 FFmpeg 进程

```
PUT /ffmpeg/api/process/{id}
```

### 删除 FFmpeg 进程

```
DELETE /ffmpeg/api/process/{id}
```

### 获取 FFmpeg 进程列表

```
GET /ffmpeg/api/processes
```

### 获取单个 FFmpeg 进程详情

```
GET /ffmpeg/api/process/{id}
```

### 启动 FFmpeg 进程

```
POST /ffmpeg/api/process/{id}/start
```

### 停止 FFmpeg 进程

```
POST /ffmpeg/api/process/{id}/stop
```

### 重启 FFmpeg 进程

```
POST /ffmpeg/api/process/{id}/restart
```

## 配置选项

```yaml
ffmpeg:
  # 数据库配置
  db:
    dsn: "ffmpeg.db"
    dbtype: "sqlite"
  
  # 默认 FFmpeg 路径
  ffmpegPath: "ffmpeg"
  
  # 进程管理配置
  maxProcesses: 100
  autoRestart: true
  restartInterval: 5s
```