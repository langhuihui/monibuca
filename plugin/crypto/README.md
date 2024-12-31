# Monibuca 加密插件

该插件提供了对视频流进行加密的功能，支持多种加密算法，可以使用静态密钥或动态密钥。

## 配置说明

在 config.yaml 中添加如下配置：

```yaml
crypto:
  isStatic: false      # 是否使用静态密钥
  algo: "aes_ctr"      # 加密算法：支持 aes_ctr、xor_s、xor_c
  encryptLen: 1024     # 加密字节长度
  secret:
    key: "your key"    # 加密密钥
    iv: "your iv"      # 加密向量（仅 aes_ctr 和 xor_c 需要）
  onpub:
    transform:
      .* : $0          # 哪些流需要加密，正则表达式，这里是所有流
```

### 加密算法说明

1. `aes_ctr`: AES-CTR 模式加密
   - key 长度要求：32字节
   - iv 长度要求：16字节

2. `xor_s`: 简单异或加密
   - key 长度要求：32字节
   - 不需要 iv

3. `xor_c`: 复杂异或加密
   - key 长度要求：32字节
   - iv 长度要求：16字节

## 密钥获取

### API 接口

获取加密密钥的 API 接口：

```
GET /crypto?stream={streamPath}
```

参数说明：
- stream: 流路径

返回示例：
```text
{key}.{iv}
```

且返回的密钥格式为 rawstd base64 编码

### 密钥生成规则

1. 静态密钥模式 (isStatic: true)
   - 直接使用配置文件中的 key 和 iv

2. 动态密钥模式 (isStatic: false)
   - key = md5(配置的密钥 + 流路径)
   - iv = md5(流路径)的前16字节


## 注意事项

1. 加密仅对视频帧的关键数据部分进行加密，保留了 NALU 头部信息
2. 使用动态密钥时，确保配置文件中设置了有效的 secret.key
3. 使用 AES-CTR 或 XOR-C 算法时，必须同时配置 key 和 iv
4. 建议在生产环境中使用动态密钥模式，提高安全性 