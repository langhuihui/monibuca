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

### 设备管理

#### GET 获取设备列表

GET /onvif/api/list

##### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|bylist|query|string| 否 |none|

> 返回示例

```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "VIRTUAL_IFACE": {
      "127.0.0.1:10000": {
        "channel": 0,
        "ip": "127.0.0.1",
        "path": "/onvif/device_service",
        "port": "10000",
        "status": 0,
        "stream": ""
      }
    }
  }
}
```
bylist = 1 返回数组 
```json
{
  "code": 0,
  "msg": "ok",
  "data": [
    {
      "channel": 0,
      "ip": "127.0.0.1",
      "path": "/onvif/device_service",
      "port": "10000",
      "status": 0,
      "stream": ""
    }
  ]
}
```

bylist = 0 返回字典 "ip:port" => 相机 

```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "VIRTUAL_IFACE": {
      "127.0.0.1:10000": {
        "channel": 0,
        "ip": "127.0.0.1",
        "path": "/onvif/device_service",
        "port": "10000",
        "status": 0,
        "stream": ""
      }
    }
  }
}
```

##### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|OK|none|Inline|

##### 返回数据结构

状态码 **200**

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» code|integer|true|none||none|
|» msg|string|true|none||none|
|» data|object|true|none||none|
|»» VIRTUAL_IFACE|object|true|none||none|
|»»» 127.0.0.1:10000|object|true|none||none|
|»»»» channel|integer|true|none||none|
|»»»» ip|string|true|none||none|
|»»»» path|string|true|none||none|
|»»»» port|string|true|none||none|
|»»»» status|integer|true|none||none|
|»»»» stream|string|true|none||none|

#### POST 添加设备

POST /onvif/api/adddevice

##### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|ip|query|string| 是 |ip|
|port|query|string| 是 |端口|
|iface|query|string| 是 |添加到哪个网卡上，默认VIRTUAL_IFACE|
|user|query|string| 是 |用户名|
|passwd|query|string| 是 |密码|
|channel|query|string| 是 |通道，默认0|

> 返回示例

> 200 Response

```json
{
  "code": 0,
  "msg": "ok",
  "data": {}
}
```

##### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|OK|none|Inline|

##### 返回数据结构

状态码 **200**

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» code|integer|true|none||none|
|» msg|string|true|none||none|
|» data|object|true|none||none|

#### POST 删除设备

POST /onvif/api/deldevice

##### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|ip|query|string| 是 |ip|
|port|query|string| 是 |端口|
|iface|query|string| 是 |添加到哪个网卡上，默认VIRTUAL_IFACE|

> 返回示例

> 200 Response

```json
{
  "code": 0,
  "msg": "ok",
  "data": {}
}
```

##### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|OK|none|Inline|

##### 返回数据结构

状态码 **200**

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» code|integer|true|none||none|
|» msg|string|true|none||none|
|» data|object|true|none||none|

#### GET 设备状态

GET /onvif/api/status

##### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|ip|query|string| 是 |ip|
|port|query|string| 是 |端口|
|iface|query|string| 是 |添加到哪个网卡上，默认VIRTUAL_IFACE|
|user|query|string| 是 |用户名|
|passwd|query|string| 是 |密码|
|channel|query|string| 是 |通道，默认0|

> 返回示例

> 200 Response

```json
{}
```

##### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|OK|none|Inline|

##### 返回数据结构

### 设备能力

#### GET 拉流

GET /onvif/api/pull

##### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|ip|query|string| 是 |ip|
|port|query|string| 是 |端口|
|iface|query|string| 是 |添加到哪个网卡上，默认VIRTUAL_IFACE|
|user|query|string| 是 |用户名|
|passwd|query|string| 是 |密码|
|channel|query|string| 是 |通道，默认0|

> 返回示例

> 200 Response

```json
{}
```

##### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|OK|none|Inline|

##### 返回数据结构

#### POST 关闭流

POST /onvif/api/close

##### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|ip|query|string| 是 |ip|
|port|query|string| 是 |端口|
|iface|query|string| 是 |添加到哪个网卡上，默认VIRTUAL_IFACE|
|user|query|string| 是 |用户名|
|passwd|query|string| 是 |密码|
|channel|query|string| 是 |通道，默认0|

> 返回示例

> 200 Response

```json
{}
```

##### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|OK|none|Inline|

##### 返回数据结构

#### GET 设备能力

GET /onvif/api/capability

##### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|ip|query|string| 是 |ip|
|port|query|string| 是 |端口|
|iface|query|string| 是 |添加到哪个网卡上，默认VIRTUAL_IFACE|
|user|query|string| 是 |用户名|
|passwd|query|string| 是 |密码|
|channel|query|string| 是 |通道，默认0|

> 返回示例

> 200 Response

```json
{}
```

##### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|OK|none|Inline|

##### 返回数据结构

#### GET 获取图像属性

GET /onvif/api/imageprofile

##### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|ip|query|string| 是 |ip|
|port|query|string| 是 |端口|
|iface|query|string| 是 |添加到哪个网卡上，默认VIRTUAL_IFACE|
|user|query|string| 是 |用户名|
|passwd|query|string| 是 |密码|
|channel|query|string| 是 |通道，默认0|

> 返回示例

> 200 Response

```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "BacklightCompensation": {
      "Mode": "",
      "Level": 0
    },
    "Brightness": 0,
    "ColorSaturation": 0,
    "Contrast": 0,
    "Exposure": {
      "Mode": "",
      "Priority": "",
      "Window": {
        "Bottom": 0,
        "Top": 0,
        "Right": 0,
        "Left": 0
      },
      "MinExposureTime": 0,
      "MaxExposureTime": 0,
      "MinGain": 0,
      "MaxGain": 0,
      "MinIris": 0,
      "MaxIris": 0,
      "ExposureTime": 0,
      "Gain": 0,
      "Iris": 0
    },
    "Focus": {
      "AutoFocusMode": "",
      "DefaultSpeed": 0,
      "NearLimit": 0,
      "FarLimit": 0,
      "Extension": ""
    },
    "IrCutFilter": "",
    "Sharpness": 0,
    "WideDynamicRange": {
      "Mode": "",
      "Level": 0
    },
    "WhiteBalance": {
      "Mode": "",
      "CrGain": 0,
      "CbGain": 0,
      "Extension": ""
    },
    "Extension": {
      "ImageStabilization": {
        "Mode": "",
        "Level": 0,
        "Extension": ""
      },
      "Extension": {
        "IrCutFilterAutoAdjustment": {
          "BoundaryType": "",
          "BoundaryOffset": 0,
          "ResponseTime": "",
          "Extension": ""
        },
        "Extension": {
          "ToneCompensation": {
            "Mode": "",
            "Level": 0,
            "Extension": ""
          },
          "Defogging": {
            "Mode": "",
            "Level": 0,
            "Extension": ""
          },
          "NoiseReduction": {
            "Level": 0
          },
          "Extension": ""
        }
      }
    }
  }
}
```

##### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|OK|none|Inline|

##### 返回数据结构

#### POST 设置图像属性

POST /onvif/api/setimageprofile

> Body 请求参数

```json
{
  "ForcePersistence": false,
  "ImageSettings": {
    "BacklightCompensation": {
      "Mode": "",
      "Level": 0
    },
    "Brightness": 0,
    "ColorSaturation": 0,
    "Contrast": 0,
    "Exposure": {
      "Mode": "",
      "Priority": "",
      "Window": {
        "Bottom": 0,
        "Top": 0,
        "Right": 0,
        "Left": 0
      },
      "MinExposureTime": 0,
      "MaxExposureTime": 0,
      "MinGain": 0,
      "MaxGain": 0,
      "MinIris": 0,
      "MaxIris": 0,
      "ExposureTime": 0,
      "Gain": 0,
      "Iris": 0
    },
    "Focus": {
      "AutoFocusMode": "",
      "DefaultSpeed": 0,
      "NearLimit": 0,
      "FarLimit": 0,
      "Extension": ""
    },
    "IrCutFilter": "",
    "Sharpness": 0,
    "WideDynamicRange": {
      "Mode": "",
      "Level": 0
    },
    "WhiteBalance": {
      "Mode": "",
      "CrGain": 0,
      "CbGain": 0,
      "Extension": ""
    },
    "Extension": {
      "ImageStabilization": {
        "Mode": "",
        "Level": 0,
        "Extension": ""
      },
      "Extension": {
        "IrCutFilterAutoAdjustment": {
          "BoundaryType": "",
          "BoundaryOffset": 0,
          "ResponseTime": "",
          "Extension": ""
        },
        "Extension": {
          "ToneCompensation": {
            "Mode": "",
            "Level": 0,
            "Extension": ""
          },
          "Defogging": {
            "Mode": "",
            "Level": 0,
            "Extension": ""
          },
          "NoiseReduction": {
            "Level": 0
          },
          "Extension": ""
        }
      }
    }
  }
}
```

##### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|ip|query|string| 是 |ip|
|port|query|string| 是 |端口|
|iface|query|string| 是 |添加到哪个网卡上，默认VIRTUAL_IFACE|
|user|query|string| 是 |用户名|
|passwd|query|string| 是 |密码|
|channel|query|string| 是 |通道，默认0|
|body|body|object| 否 |none|
|» ForcePersistence|body|boolean| 是 |none|
|» ImageSettings|body|object| 是 |none|
|»» BacklightCompensation|body|object| 是 |none|
|»»» Mode|body|string| 是 |none|
|»»» Level|body|integer| 是 |none|
|»» Brightness|body|integer| 是 |none|
|»» ColorSaturation|body|integer| 是 |none|
|»» Contrast|body|integer| 是 |none|
|»» Exposure|body|object| 是 |none|
|»»» Mode|body|string| 是 |none|
|»»» Priority|body|string| 是 |none|
|»»» Window|body|object| 是 |none|
|»»»» Bottom|body|integer| 是 |none|
|»»»» Top|body|integer| 是 |none|
|»»»» Right|body|integer| 是 |none|
|»»»» Left|body|integer| 是 |none|
|»»» MinExposureTime|body|integer| 是 |none|
|»»» MaxExposureTime|body|integer| 是 |none|
|»»» MinGain|body|integer| 是 |none|
|»»» MaxGain|body|integer| 是 |none|
|»»» MinIris|body|integer| 是 |none|
|»»» MaxIris|body|integer| 是 |none|
|»»» ExposureTime|body|integer| 是 |none|
|»»» Gain|body|integer| 是 |none|
|»»» Iris|body|integer| 是 |none|
|»» Focus|body|object| 是 |none|
|»»» AutoFocusMode|body|string| 是 |none|
|»»» DefaultSpeed|body|integer| 是 |none|
|»»» NearLimit|body|integer| 是 |none|
|»»» FarLimit|body|integer| 是 |none|
|»»» Extension|body|string| 是 |none|
|»» IrCutFilter|body|string| 是 |none|
|»» Sharpness|body|integer| 是 |none|
|»» WideDynamicRange|body|object| 是 |none|
|»»» Mode|body|string| 是 |none|
|»»» Level|body|integer| 是 |none|
|»» WhiteBalance|body|object| 是 |none|
|»»» Mode|body|string| 是 |none|
|»»» CrGain|body|integer| 是 |none|
|»»» CbGain|body|integer| 是 |none|
|»»» Extension|body|string| 是 |none|
|»» Extension|body|object| 是 |none|
|»»» ImageStabilization|body|object| 是 |none|
|»»»» Mode|body|string| 是 |none|
|»»»» Level|body|integer| 是 |none|
|»»»» Extension|body|string| 是 |none|
|»»» Extension|body|object| 是 |none|
|»»»» IrCutFilterAutoAdjustment|body|object| 是 |none|
|»»»»» BoundaryType|body|string| 是 |none|
|»»»»» BoundaryOffset|body|integer| 是 |none|
|»»»»» ResponseTime|body|string| 是 |none|
|»»»»» Extension|body|string| 是 |none|
|»»»» Extension|body|object| 是 |none|
|»»»»» ToneCompensation|body|object| 是 |none|
|»»»»»» Mode|body|string| 是 |none|
|»»»»»» Level|body|integer| 是 |none|
|»»»»»» Extension|body|string| 是 |none|
|»»»»» Defogging|body|object| 是 |none|
|»»»»»» Mode|body|string| 是 |none|
|»»»»»» Level|body|integer| 是 |none|
|»»»»»» Extension|body|string| 是 |none|
|»»»»» NoiseReduction|body|object| 是 |none|
|»»»»»» Level|body|integer| 是 |none|
|»»»»» Extension|body|string| 是 |none|

> 返回示例

```json
{
  "code": 0,
  "msg": "ok",
  "data": null
}
```

```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "Token": "PTZPresetToken_2"
  }
}
```

##### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|OK|none|Inline|

##### 返回数据结构

状态码 **200**

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» code|integer|true|none||none|
|» msg|string|true|none||none|
|» data|object|true|none||none|
|»» Token|string|true|none||none|

### PTZ控制

#### GET 获取预置点

GET /onvif/api/ptzpreset

##### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|ip|query|string| 是 |ip|
|port|query|string| 是 |端口|
|iface|query|string| 是 |添加到哪个网卡上，默认VIRTUAL_IFACE|
|user|query|string| 是 |用户名|
|passwd|query|string| 是 |密码|
|channel|query|string| 是 |通道，默认0|

> 返回示例

```json
{
  "code": 0,
  "msg": "ok",
  "data": null
}
```

```json
{
  "code": 0,
  "msg": "ok",
  "data": [
    {
      "Token": "PTZPresetToken_1",
      "Name": "大门",
      "PTZPosition": {
        "PanTilt": {
          "X": 0,
          "Y": 0,
          "Space": null
        },
        "Zoom": {
          "X": 0,
          "Space": null
        }
      }
    }
  ]
}
```

##### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|OK|none|Inline|

##### 返回数据结构

状态码 **200**

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» code|integer|true|none||none|
|» msg|string|true|none||none|
|» data|[object]|true|none||none|
|»» Token|string|false|none||none|
|»» Name|string|false|none||none|
|»» PTZPosition|object|false|none||none|
|»»» PanTilt|object|true|none||none|
|»»»» X|integer|true|none||none|
|»»»» Y|integer|true|none||none|
|»»»» Space|null|true|none||none|
|»»» Zoom|object|true|none||none|
|»»»» X|integer|true|none||none|
|»»»» Space|null|true|none||none|

#### POST 设置预置点

POST /onvif/api/setptzpreset

> Body 请求参数

```json
{}
```

##### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|ip|query|string| 是 |ip|
|port|query|string| 是 |端口|
|iface|query|string| 是 |添加到哪个网卡上，默认VIRTUAL_IFACE|
|user|query|string| 是 |用户名|
|passwd|query|string| 是 |密码|
|channel|query|string| 是 |通道，默认0|
|name|query|string| 是 |预置点名称|
|preset_token|query|string| 否 |预置点token,如果是新增可以不填，如果是修改，必填|
|body|body|object| 否 |none|

> 返回示例

```json
{
  "code": 0,
  "msg": "ok",
  "data": null
}
```

```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "Token": "PTZPresetToken_2"
  }
}
```

##### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|OK|none|Inline|

##### 返回数据结构

状态码 **200**

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» code|integer|true|none||none|
|» msg|string|true|none||none|
|» data|object|true|none||none|
|»» Token|string|true|none||none|

#### POST 跳转预置点

POST /onvif/api/gotoptzpreset

> Body 请求参数

```json
{
  "PanTilt": {
    "X": 1,
    "Y": 1,
    "Space": "http://www.onvif.org/ver10/tptz/ZoomSpaces/VelocityGenericSpace"
  },
  "Zoom": {
    "X": 1,
    "Space": "http://www.onvif.org/ver10/tptz/ZoomSpaces/VelocityGenericSpace"
  }
}
```

##### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|ip|query|string| 是 |ip|
|port|query|string| 是 |端口|
|iface|query|string| 是 |添加到哪个网卡上，默认VIRTUAL_IFACE|
|user|query|string| 是 |用户名|
|passwd|query|string| 是 |密码|
|channel|query|string| 是 |通道，默认0|
|preset_token|query|string| 是 |预置点token|
|body|body|object| 否 |none|
|» PanTilt|body|object| 否 |none|
|»» X|body|integer| 否 |none|
|»» Y|body|integer| 否 |none|
|»» Space|body|string| 否 |none|
|» Zoom|body|object| 否 |none|
|»» X|body|integer| 否 |none|
|»» Space|body|string| 否 |none|

> 返回示例

```json
{
  "code": 0,
  "msg": "ok",
  "data": null
}
```

```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "Token": "PTZPresetToken_2"
  }
}
```

```json
{
  "code": 0,
  "msg": "ok",
  "data": {}
}
```

##### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|OK|none|Inline|

##### 返回数据结构

状态码 **200**

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» code|integer|true|none||none|
|» msg|string|true|none||none|
|» data|object|true|none||none|

#### POST 删除预置点

POST /onvif/api/removeptzpreset

##### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|ip|query|string| 是 |ip|
|port|query|string| 是 |端口|
|iface|query|string| 是 |添加到哪个网卡上，默认VIRTUAL_IFACE|
|user|query|string| 是 |用户名|
|passwd|query|string| 是 |密码|
|channel|query|string| 是 |通道，默认0|
|preset_token|query|string| 是 |预置点token|

> 返回示例

```json
{
  "code": 0,
  "msg": "ok",
  "data": null
}
```

```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "Token": "PTZPresetToken_2"
  }
}
```

```json
{
  "code": 0,
  "msg": "ok",
  "data": {}
}
```

##### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|OK|none|Inline|

##### 返回数据结构

状态码 **200**

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» code|integer|true|none||none|
|» msg|string|true|none||none|
|» data|object|true|none||none|

#### POST ptz移动

POST /onvif/api/ptz

> Body 请求参数

```json
{
  "PanTilt": {
    "X": 1,
    "Y": 1,
    "Space": "http://www.onvif.org/ver10/tptz/ZoomSpaces/VelocityGenericSpace"
  },
  "Zoom": {
    "X": 1,
    "Space": "http://www.onvif.org/ver10/tptz/ZoomSpaces/VelocityGenericSpace"
  }
}
```

##### 请求参数

|名称|位置|类型|必选|说明|
|---|---|---|---|---|
|ip|query|string| 是 |ip|
|port|query|string| 是 |端口|
|iface|query|string| 是 |添加到哪个网卡上，默认VIRTUAL_IFACE|
|user|query|string| 是 |用户名|
|passwd|query|string| 是 |密码|
|channel|query|string| 是 |通道，默认0|
|mode|query|integer| 否 |控制模式 0：绝对 1：相对 2：连续|
|body|body|object| 否 |none|
|» PanTilt|body|object| 是 |none|
|»» X|body|integer| 是 |none|
|»» Y|body|integer| 是 |none|
|»» Space|body|string| 是 |none|
|» Zoom|body|object| 是 |none|
|»» X|body|integer| 是 |none|
|»» Space|body|string| 是 |none|

> 返回示例

```json
{
  "code": 0,
  "msg": "ok",
  "data": null
}
```

```json
{
  "code": 0,
  "msg": "ok",
  "data": {
    "Token": "PTZPresetToken_2"
  }
}
```

```json
{
  "code": 0,
  "msg": "ok",
  "data": {}
}
```

##### 返回结果

|状态码|状态码含义|说明|数据模型|
|---|---|---|---|
|200|OK|none|Inline|

##### 返回数据结构

状态码 **200**

|名称|类型|必选|约束|中文名|说明|
|---|---|---|---|---|---|
|» code|integer|true|none||none|
|» msg|string|true|none||none|
|» data|object|true|none||none|



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