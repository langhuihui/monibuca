# ONVIF Plugin for Monibuca v5

ONVIF 插件用于发现和管理 ONVIF 设备，支持自动发现、添加和拉流等功能。

## 配置说明

```yaml
onvif:
  discoverInterval: 30 # 设备发现间隔（秒）
  autoPull: true      # 是否自动拉流
  autoAdd: true       # 是否自动添加发现的设备
  interfaces:         # 网卡配置
    - interfaceName: eth0
      username: admin
      password: admin
  devices:           # 设备配置
    - ip: 192.168.1.100
      username: admin
      password: admin
```

## API 接口

### 1. 设备管理
#### 1.1 设备列表
- 路径：`/onvif/list`
- 方法：GET
- 描述：获取所有已发现和添加的设备列表

#### 1.2 添加设备
- 路径：`/onvif/add`
- 方法：POST
- 描述：手动添加 ONVIF 设备
- 参数：
  ```json
  {
    "ip": "192.168.1.100",
    "port": "80",
    "user": "admin",
    "passwd": "admin",
    "path": "",
    "channel": 0
  }
  ```

#### 1.3 移除设备
- 路径：`/onvif/remove`
- 方法：POST
- 描述：移除已添加的设备
- 参数：
  ```json
  {
    "ip": "192.168.1.100"
  }
  ```

#### 1.4 设备发现
- 路径：`/onvif/discovery`
- 方法：GET
- 描述：手动触发设备发现

### 2. PTZ 控制
#### 2.1 云台移动
- 路径：`/onvif/ptz/move`
- 方法：POST
- 描述：控制设备云台移动
- 参数：
  ```json
  {
    "ip": "192.168.1.100",
    "mode": 0,        // 0:绝对移动 1:相对移动 2:连续移动
    "pan": 0.0,       // 水平移动 -1.0 到 1.0
    "tilt": 0.0,      // 垂直移动 -1.0 到 1.0
    "zoom": 0.0,      // 缩放 -1.0 到 1.0
    "speed": 1.0      // 移动速度 0.0 到 1.0
  }
  ```

#### 2.2 获取预置点
- 路径：`/onvif/ptz/preset/get`
- 方法：POST
- 描述：获取设备预置点列表
- 参数：
  ```json
  {
    "ip": "192.168.1.100"
  }
  ```

#### 2.3 设置预置点
- 路径：`/onvif/ptz/preset/set`
- 方法：POST
- 描述：设置预置点
- 参数：
  ```json
  {
    "ip": "192.168.1.100",
    "preset_token": "1",
    "preset_name": "position1"
  }
  ```

#### 2.4 调用预置点
- 路径：`/onvif/ptz/preset/goto`
- 方法：POST
- 描述：移动到预置点位置
- 参数：
  ```json
  {
    "ip": "192.168.1.100",
    "preset_token": "1"
  }
  ```

### 3. 图像设置
#### 3.1 获取图像参数
- 路径：`/onvif/imaging/get`
- 方法：POST
- 描述：获取设备图像参数
- 参数：
  ```json
  {
    "ip": "192.168.1.100"
  }
  ```

#### 3.2 设置图像参数
- 路径：`/onvif/imaging/set`
- 方法：POST
- 描述：设置设备图像参数
- 参数：
  ```json
  {
    "ip": "192.168.1.100",
    "brightness": 50.0,
    "color_saturation": 50.0,
    "contrast": 50.0,
    "sharpness": 50.0,
    "force": false
  }
  ```

## 功能特性

1. 自动发现 ONVIF 设备
2. 支持手动添加设备
3. 自动获取设备视频流地址
4. 支持多通道选择
5. 支持自动拉流
6. 支持设备认证管理
7. PTZ 云台控制
   - 绝对移动
   - 相对移动
   - 连续移动
   - 预置点管理
8. 图像参数设置
   - 亮度
   - 对比度
   - 饱和度
   - 锐度

## 依赖要求

- Monibuca v5.0.0 或更高版本
- Go 1.18 或更高版本

## 安装方法

该插件已经包含在 Monibuca v5 的主仓库中，无需单独安装。只需在配置文件中启用该插件即可使用。 

## VIRTUAL_IFACE 说明
VIRTUAL_IFACE 在 plugin-onvifpro 插件中被用作一个特殊的接口名称，主要用于以下目的：
虚拟接口标识: 它作为一个常量字符串，用于标识一种特殊的设备管理模式，可能用于手动添加或配置的设备，而不是通过网络接口自动发现的设备。
流路径解析: 在解析流路径时，VIRTUAL_IFACE 被用作接口名称，以便正确提取设备地址。
设备列表组织: 设备列表使用 VIRTUAL_IFACE 作为 key 来组织设备，方便管理。