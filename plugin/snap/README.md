# Snap 插件

Snap 插件提供了对流媒体的截图功能，支持定时截图、按关键帧截图以及手动触发截图。同时支持水印功能和历史截图查询。

## 配置说明

```yaml
snap:
  snapwatermark:
    text: ""              # 水印文字内容
    fontpath: ""          # 水印字体文件路径
    fontcolor: "rgba(255,165,0,1)" # 水印字体颜色，支持rgba格式
    fontsize: 36          # 水印字体大小
    offsetx: 0           # 水印位置X偏移
    offsety: 0           # 水印位置Y偏移
  snaptimeinterval: 1m   # 截图时间间隔，默认1分钟
  snapsavepath: "snaps"  # 截图保存路径
  filter: ".*"           # 截图流过滤器，支持正则表达式
  snapiframeinterval: 3  # 间隔多少帧截图
  snapmode: 1            # 截图模式：0-时间间隔，1-关键帧间隔
  snapquerytimedelta: 3  # 查询截图时允许的最大时间差（秒）
```

## HTTP API

### 1. 手动触发截图

```http
GET /{streamPath}
```

参数说明：
- `streamPath`: 流路径

响应：
- 成功：返回 JPEG 图片
- 失败：返回错误信息

### 2. 查询历史截图

```http
GET /query?streamPath={streamPath}&snapTime={timestamp}
```

参数说明：
- `streamPath`: 流路径
- `snapTime`: Unix时间戳（秒）

响应：
- 成功：返回最接近请求时间的 JPEG 图片
- 失败：返回错误信息
  - 404：未找到截图或时间差超出配置范围
  - 400：参数错误
  - 500：服务器内部错误

## 截图模式说明

### 时间间隔模式 (snapmode: 0)
- 按照配置的 `snaptimeinterval` 定时对流进行截图
- 适合需要固定时间间隔截图的场景

### 关键帧间隔模式 (snapmode: 1)
- 按照配置的 `snapiframeinterval` 对关键帧进行截图
- 适合需要按视频内容变化进行截图的场景

### HTTP请求模式 (snapmode: 2)
- 通过 HTTP API 手动触发截图
- 适合需要实时获取画面的场景

## 水印功能

支持为截图添加文字水印，可配置：
- 水印文字内容
- 字体文件
- 字体颜色（RGBA格式）
- 字体大小
- 位置偏移

## 数据库记录

每次截图都会在数据库中记录以下信息：
- 流名称（StreamName）
- 截图模式（SnapMode）
- 截图时间（SnapTime）
- 截图路径（SnapPath）
- 创建时间（CreatedAt）

## 使用示例

1. 基础配置示例：
```yaml
snap:
  snaptimeinterval: 30s
  snapsavepath: "./snapshots"
  snapmode: 1
  snapiframeinterval: 5
```

2. 带水印的配置示例：
```yaml
snap:
  snapwatermark:
    text: "测试水印"
    fontpath: "/path/to/font.ttf"
    fontcolor: "rgba(255,0,0,0.5)"
    fontsize: 48
    offsetx: 20
    offsety: 20
  snapmode: 0
  snaptimeinterval: 1m
```

3. API调用示例：
```bash
# 手动触发截图
curl http://localhost:8080/snap/live/stream1

# 查询历史截图
curl http://localhost:8080/snap/query?streamPath=live/stream1&snapTime=1677123456
``` 