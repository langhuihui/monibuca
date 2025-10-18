# AV1 裸格式支持 - 修改说明

## 问题描述

项目中原本只考虑了 H.264/H.265（H26x）编码，使用 `Nalus`（NALU 数组）作为裸格式的中间表示。但 AV1 编码使用的是 OBU（Open Bitstream Unit）而不是 NALU，因此需要重新设计裸格式的处理方式，以支持不同编码格式的中转。

## 核心概念

### 什么是裸格式（Raw Format）？

在视频流处理中，裸格式是指从容器格式（如 RTMP、RTP）解包后，但还未重新封装到另一种容器格式之前的中间表示。对于：
- **H.264/H.265**: 裸格式是 NALU (Network Abstraction Layer Unit) 数组
- **AV1**: 裸格式是 OBU (Open Bitstream Unit) 数组

这些裸格式用于在不同协议间中转视频流，例如从 RTMP 推流到 RTP/WebRTC 播放。

## 关键修改

### 1. pkg/avframe.go - 核心类型定义

#### OBUs 类型重新定义
```go
// 修改前
OBUs AudioData  // AudioData = gomem.Memory

// 修改后
OBUs = util.ReuseArray[gomem.Memory]  // 与 Nalus 类型一致
```

这个修改使得 OBUs 和 Nalus 都是 `util.ReuseArray[gomem.Memory]` 类型，提供了统一的数组接口。

#### 添加 GetOBUs() 方法
```go
func (b *BaseSample) GetOBUs() *OBUs {
    if b.Raw == nil {
        b.Raw = &OBUs{}
    }
    return b.Raw.(*OBUs)
}
```

这与 `GetNalus()` 方法对应，用于获取 AV1 的裸格式数据。

#### 更新 OBUs 方法实现
```go
func (obus *OBUs) ParseAVCC(reader *gomem.MemoryReader) error {
    obus.Reset()  // 重置数组
    // ... 解析 OBU 并添加到数组中
    obus.GetNextPointer().PushOne(obu)
}

func (obus *OBUs) Reset() {
    (*util.ReuseArray[gomem.Memory])(obus).Reset()
}

func (obus *OBUs) Count() int {
    return (*util.ReuseArray[gomem.Memory])(obus).Count()
}
```

### 2. pkg/format/raw.go - AV1 原始格式

添加了新的 `AV1Frame` 类型：

```go
type AV1Frame struct {
    pkg.Sample
}

func (a *AV1Frame) GetSize() (ret int) {
    if obus, ok := a.Raw.(*pkg.OBUs); ok {
        for obu := range obus.RangePoint {
            ret += obu.Size
        }
    }
    return
}

func (a *AV1Frame) Demux() error {
    a.Raw = &a.Memory
    return nil
}

func (a *AV1Frame) Mux(from *pkg.Sample) (err error) {
    a.InitRecycleIndexes(0)
    obus := from.Raw.(*pkg.OBUs)
    for obu := range obus.RangePoint {
        a.Push(obu.Buffers...)
    }
    a.ICodecCtx = from.GetBase()
    return
}
```

### 3. plugin/rtmp/pkg/video.go - RTMP 协议支持

#### parseAV1 方法修改
```go
func (avcc *VideoFrame) parseAV1(reader *gomem.MemoryReader) error {
    obus := avcc.GetOBUs()  // 使用 GetOBUs() 方法
    if err := obus.ParseAVCC(reader); err != nil {
        return err
    }
    return nil
}
```

#### Mux 方法添加 AV1 支持
```go
case *codec.AV1Ctx:
    if avcc.ICodecCtx == nil {
        ctx := &AV1Ctx{AV1Ctx: c}
        configBytes := make([]byte, 4+len(c.ConfigOBUs))
        configBytes[0] = 0b1001_0000 | byte(PacketTypeSequenceStart)
        copy(configBytes[1:], codec.FourCC_AV1[:])
        copy(configBytes[5:], c.ConfigOBUs)
        ctx.SequenceFrame.PushOne(configBytes)
        ctx.SequenceFrame.BaseSample = &BaseSample{}
        avcc.ICodecCtx = ctx
    }
    obus := fromBase.Raw.(*OBUs)
    avcc.InitRecycleIndexes(obus.Count())
    head := avcc.NextN(5)
    if fromBase.IDR {
        head[0] = 0b1001_0000 | byte(PacketTypeCodedFrames)
    } else {
        head[0] = 0b1010_0000 | byte(PacketTypeCodedFrames)
    }
    copy(head[1:], codec.FourCC_AV1[:])
    for obu := range obus.RangePoint {
        avcc.Push(obu.Buffers...)
    }
```

### 4. plugin/rtp/pkg/video.go - RTP 协议支持

#### CheckCodecChange 方法修改
将 `nalus := r.Raw.(*Nalus)` 移到各个 case 分支内部，避免对 AV1 进行错误的类型断言。

#### Demux 方法添加 AV1 支持
```go
case *AV1Ctx:
    obus := r.GetOBUs()
    obus.Reset()
    for _, packet := range r.Packets {
        if len(packet.Payload) > 0 {
            obus.GetNextPointer().PushOne(packet.Payload)
        }
    }
    return nil
```

#### Mux 方法添加 AV1 支持
```go
case *codec.AV1Ctx:
    var ctx AV1Ctx
    ctx.AV1Ctx = base
    ctx.PayloadType = 99
    ctx.MimeType = webrtc.MimeTypeAV1
    ctx.ClockRate = 90000
    ctx.SSRC = uint32(uintptr(unsafe.Pointer(&ctx)))
    codecCtx = &ctx

// ... 在 Mux 处理中
case *AV1Ctx:
    ctx := &c.RTPCtx
    var lastPacket *rtp.Packet
    for obu := range baseFrame.Raw.(*OBUs).RangePoint {
        mem := r.NextN(obu.Size)
        obu.NewReader().Read(mem)
        lastPacket = r.Append(ctx, pts, mem)
    }
    if lastPacket != nil {
        lastPacket.Header.Marker = true
    }
```

## 设计原则

1. **统一的数组结构**：OBUs 和 Nalus 都使用 `util.ReuseArray[gomem.Memory]`，提供一致的接口。

2. **类型安全**：通过 `GetOBUs()` 和 `GetNalus()` 方法进行类型转换，避免直接的类型断言错误。

3. **协议独立**：在各协议（RTMP、RTP）的实现中分别处理 H26x 和 AV1，保持代码的清晰性。

4. **扩展性**：新的设计使得添加其他编码格式（如 VP8、VP9）更加容易。

## 注意事项

1. **AV1 的 RTP 封装**：当前实现是简化版本，每个 OBU 作为一个完整的 RTP 包。实际的 RFC 标准可能需要更复杂的分片和聚合逻辑。

2. **HLS 支持**：AV1 在 HLS 中的支持目前被跳过，需要 gohlslib 库的支持。

3. **IDR 帧检测**：AV1 的关键帧检测逻辑可能需要根据 OBU 类型进一步完善。

## 测试建议

1. 测试 RTMP Enhanced 模式下的 AV1 推流
2. 测试 RTP/WebRTC 下的 AV1 传输
3. 测试不同协议之间的 AV1 转码

## 未来改进

1. 完善 AV1 RTP 封装，支持分片和聚合
2. 添加 AV1 在 HLS 中的支持（需要库支持）
3. 添加 AV1 关键帧的准确检测
4. 性能优化和内存池管理
