# SetAliasStream 的逻辑
```mermaid
graph LR
subgraph 创建
    direction LR
    subgraph "创建前1"
    direction TB
    C[Subscriber:1] --> A[StreamPath:1] --o B[Publisher:1]
    end
    subgraph "创建前2"
    direction TB
    C0[Subscriber:0] --> A0[StreamPath:0] --o B0[Publisher:0]
    end
    subgraph "创建前3"
    direction TB
    C01[Subscriber:2] --> A01[StreamPath:2] --o B01[Publisher:2]
    A02[StreamPath:1] --> B02[Publisher:1]
    end
    subgraph "无副作用"
    direction TB
    C1[Subscriber:1] --> A1[StreamPath:1] --oB1[Publisher:1]
    D1[StreamPath:2] --> A1
    E1[Subscriber:2] --> D1
    end
    subgraph "目标流不存在"
    direction TB
    C2[Subscriber:0] --> A2[StreamPath:0] --o B2[Publisher:0]
    D2[StreamPath:2] --> A22[StreamPath:1] --o B22[Wait:1]
    E2[Subscriber:2] --> D2
    end
    subgraph "别名顶替"
    direction TB
    B32[Publisher:2]
    D3[StreamPath:2] --> A3[StreamPath:1] --o B3[Publisher:1]
    E3[Subscriber:2] --> D3
    end
end
    subgraph "存在Publisher:2"
    C52[Subscriber:2] --> A52[StreamPath:2] --o B52[Publisher:2]
    end
    subgraph "不存在Publisher:2"
    D6[StreamPath:2] --o B6[Wait:2]
    E6[Subscriber:2] --> D6
    end
    subgraph "存在Publisher:3"
    C73[Subscriber:2] --> D73[StreamPath:2] --> A73[StreamPath:3] --o B73[Publisher:3]
    end
    创建前1 --> |StreamPath:2->StreamPath:1| 无副作用
    创建前2 --> |StreamPath:2->StreamPath:1| 目标流不存在
    创建前3 --> |StreamPath:2->StreamPath:1| 别名顶替
    创建 --> |删除| 存在Publisher:2
    创建 --> |删除| 不存在Publisher:2
    创建 --> |修改| 存在Publisher:3
    创建 --> |修改| 不存在Publisher:3触发拉流请求
```

# Publisher Start 时对 Alias 的处理
```mermaid
graph LR
subgraph 发布前
    direction TB
    C[Subscriber:2] -->D[StreamPath:2] -->A[StreamPath:1] --o B[Wait:1]
end
subgraph 发布后
    direction TB
    C2[Subscriber:2] -->D2[StreamPath:2] -->A2[StreamPath:1] --o B2[Publisher:1]
end
subgraph 发布前2
    direction TB
    C3[Subscriber:2] -->D3[StreamPath:2] -->A3[StreamPath:1] --o B3[Publisher:0]
end
subgraph 发布后2
    direction TB
    C4[Subscriber:2] -->D4[StreamPath:2] -->A4[StreamPath:1] --o B4[Publisher:1]
end
  发布前 --> |Publisher:1| 发布后
  发布前2 --> |Publisher:1| 发布后2
```

# Publisher Dispose 时对 Alias 的处理

```mermaid
graph LR
subgraph 销毁前
    direction TB
    C2[Subscriber:2] -->D2[StreamPath:2] -->A2[StreamPath:1] --o B2[Publisher:1]
end
subgraph 销毁后
    direction TB
    C3[Subscriber:2] -->D3[StreamPath:2] -->A3[StreamPath:1]
    D3 --o B3[Publisher:2]
end
销毁前 --> |Publisher:1| 销毁后
```

# Subscriber Start 时对 Alias 的处理

```mermaid 
graph LR
subgraph 订阅前
    direction TB
    D[StreamPath:2] -->A[StreamPath:1] --o B[Publisher:1]
end
subgraph 订阅后
    direction TB
    C2[Subscriber:2] -->D2[StreamPath:2] -->A2[StreamPath:1] --o B2[Publisher:1]
end
subgraph 订阅前2
    direction TB
    D3[StreamPath:2] -->A3[StreamPath:1]
end
subgraph 订阅后2
    direction TB
    C4[Subscriber:2] -->D4[StreamPath:2] -->A4[StreamPath:1] --o B3[Wait:1]
end
订阅前 --> |Subscriber:2| 订阅后
订阅前2 --> |Subscriber:2| 订阅后2
```

1. Publisher Start 时对 Alias 的处理逻辑：

```mermaid
graph TD
    A[Publisher Start] --> B{检查是否存在相同StreamPath的Publisher}
    B -->|是| C[调用takeOver处理旧Publisher]
    B -->|否| D[将新Publisher添加到Streams]
    D --> E[唤醒等待该StreamPath的订阅者]
    E --> F[遍历AliasStreams]
    F --> G{Alias的StreamPath是否匹配?}
    G -->|是| H{Alias的Publisher是否为空?}
    H -->|是| I[设置Alias的Publisher为新Publisher]
    H -->|否| J[将Alias的订阅者转移到新Publisher]
    G -->|否| K[继续遍历]
    I --> L[唤醒等待该Alias的订阅者]
    J --> M
    L --> M[结束]
```

2. Publisher Dispose 时对 Alias 的处理：

```mermaid
graph TD
    A[Publisher Dispose] --> B{是否因为被踢出而停止?}
    B -->|否| C[从Streams中移除Publisher]
    C --> D[遍历AliasStreams]
    D --> E{Alias是否指向该Publisher?}
    E -->|是| F{是否自动移除?}
    F -->|是| G[从AliasStreams中移除Alias]
    F -->|否| H[保留Alias]
    E -->|否| I[继续遍历]
    G --> J[处理订阅者]
    H --> J
    J --> K[结束]
```

3. Subscriber Start 时对 Alias 的处理：

```mermaid
graph TD
    A[Subscriber Start] --> B{检查AliasStreams中是否存在匹配的Alias}
    B -->|是| C{Alias的Publisher是否存在?}
    C -->|是| D[将订阅者添加到Alias的Publisher]
    C -->|否| E[触发OnSubscribe事件]
    B -->|否| F[检查StreamAlias中是否有匹配的正则表达式]
    F -->|是| G[创建新的AliasStream]
    G --> H{对应的Publisher是否存在?}
    H -->|是| I[将订阅者添加到Publisher]
    H -->|否| J[触发OnSubscribe事件]
    F -->|否| K{Streams中是否存在对应的Publisher?}
    K -->|是| L[将订阅者添加到Publisher]
    K -->|否| M[将订阅者添加到等待列表]
    M --> N[触发OnSubscribe事件]
```

4. API 中调用 SetAliasStream 增加别名的逻辑：

```mermaid
graph TD
    A[SetAliasStream - 增加别名] --> B{AliasStreams中是否已存在该别名?}
    B -->|是| C[更新现有AliasStream]
    B -->|否| D[创建新的AliasStream]
    C --> E{StreamPath是否变更?}
    E -->|是| F{新StreamPath的Publisher是否存在?}
    F -->|是| G[转移订阅者到新Publisher]
    F -->|否| H[唤醒等待新StreamPath的订阅者]
    E -->|否| I[结束]
    D --> J{StreamPath的Publisher是否存在?}
    J -->|是| K[替换现有流或唤醒等待的订阅者]
    J -->|否| L[结束]
```

5. API 中调用 SetAliasStream 删除别名的逻辑：

```mermaid
graph TD
    A[SetAliasStream - 删除别名] --> B{AliasStreams中是否存在该别名?}
    B -->|是| C[从AliasStreams中移除别名]
    C --> D{Alias的Publisher是否存在?}
    D -->|是| E{Streams中是否存在同名的Publisher?}
    E -->|是| F[将Alias的订阅者转移到同名Publisher]
    E -->|否| H
    D -->|否| H[等待源Publisher]
    B -->|否| I[结束]
```
