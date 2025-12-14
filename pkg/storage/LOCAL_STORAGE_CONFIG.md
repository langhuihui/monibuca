# Local Storage 主备存储配置说明

## 配置格式

### 1. 旧格式（字符串）- 兼容
```yaml
onpub:
  record:
    "(.*)":
      filepath: "/data/record/$0"
      storage:
        local: "/data/record"  # 直接字符串路径
```

### 2. 新格式（只有主存储）
```yaml
onpub:
  record:
    "(.*)":
      filepath: "/data/record/$0"
      storage:
        local:
          path: "/data/ssd"
          diskthreshold: 90
```

### 3. 新格式（主备两级存储）
```yaml
onpub:
  record:
    "(.*)":
      filepath: "/data/record/$0"
      storage:
        local:
          path: "/data/ssd"                # 主存储路径
          backuppath: "/data/hdd"          # 备用存储路径
          diskthreshold: 80                # 主存储阈值
          backupdiskthreshold: 95          # 备用存储阈值
```

## 配置字段说明

### LocalStorageConfig 字段

| 字段 | 类型 | 必填 | 默认值 | 说明 |
|------|------|------|--------|------|
| `path` | string | 是 | - | 主存储路径（绝对路径或相对路径） |
| `backuppath` | string | 否 | "" | 备用存储路径（可选） |
| `diskthreshold` | int | 否 | 0 | 主存储磁盘使用率阈值（0-100，0表示不检查） |
| `backupdiskthreshold` | int | 否 | 0 | 备用存储磁盘使用率阈值（0-100） |

## 配置示例

### 示例1：单级存储（旧格式）
```yaml
storage:
  local: "/data/record"
```

**说明**：兼容旧配置，等价于：
```yaml
storage:
  local:
    path: "/data/record"
    diskthreshold: 0
```

### 示例2：只有主存储（新格式）
```yaml
storage:
  local:
    path: "/data/ssd"
    diskthreshold: 90
```

**说明**：
- 只配置主存储
- 当磁盘使用率达到 90% 时，删除最旧文件

### 示例3：主备两级存储
```yaml
storage:
  local:
    path: "/data/ssd"
    backuppath: "/data/hdd"
    diskthreshold: 80
    backupdiskthreshold: 95
```

**说明**：
- 主存储：SSD（`/data/ssd`）
- 备用存储：HDD（`/data/hdd`）
- 当 SSD 使用率达到 80% 时，迁移最旧文件到 HDD
- 当 HDD 使用率达到 95% 时，删除 HDD 中最旧文件

## 实现逻辑

### 路径选择算法（selectStoragePath）

当前实现：简单返回主存储路径

```go
func (s *LocalStorage) selectStoragePath() (string, error) {
    // 当前简单返回主存储路径
    // 后续可根据磁盘使用率动态选择主存储或备用存储
    return s.config.Path, nil
}
```

### 存储管理策略（待实现）

#### 单级存储
```
磁盘使用率 < diskthreshold → 正常写入
磁盘使用率 ≥ diskthreshold → 删除最旧文件
```

#### 主备两级存储
```
主存储使用率 < diskthreshold → 正常写入主存储
主存储使用率 ≥ diskthreshold → 迁移最旧文件到备用存储
备用存储使用率 < backupdiskthreshold → 正常接收迁移文件
备用存储使用率 ≥ backupdiskthreshold → 删除备用存储中最旧文件
```

### 与全局配置的关系

```yaml
mp4:
  autooverwritediskpercent: 90  # 全局阈值（兜底）
  onpub:
    record:
      ^mp4/.+:
        storage:
          local:
            path: "/data/ssd"
            diskthreshold: 80  # 优先使用这个阈值
```

**优先级规则**：
1. 如果 `storage.local.diskthreshold` 配置了（> 0），使用它
2. 如果 `storage.local.diskthreshold` 未配置（= 0），使用 `mp4.autooverwritediskpercent`

## 当前实现状态

### ✅ 已实现
- [x] 配置解析（支持字符串、对象格式）
- [x] 配置验证（路径必填、磁盘使用率范围检查）
- [x] 兼容旧配置（字符串路径）
- [x] 结构体定义（LocalStorageConfig，主备两级）
- [x] 路径选择逻辑（当前返回主存储路径）
- [x] 单元测试（配置解析、路径选择、配置验证）

### 🚧 待实现
- [ ] 磁盘使用率检查（使用 `syscall.Statfs` 或 `github.com/shirou/gopsutil/v4/disk`）
- [ ] 文件迁移逻辑（主存储 → 备用存储）
- [ ] 文件删除逻辑（达到阈值时删除最旧文件）
- [ ] 与全局阈值的集成（优先级判断）
- [ ] 存储管理任务（定期检查和执行）
- [ ] 存储切换日志（记录迁移和删除操作）

## 测试

运行单元测试：
```bash
cd /Volumes/extend/go/src/m7s/monibucav5/pkg/storage
go test -v -run TestParseLocalStorageConfig
go test -v -run TestLocalStorageConfigValidate
```

## 注意事项

1. **路径格式**：建议使用绝对路径，相对路径会被转换为绝对路径
2. **优先级**：数字越小优先级越高（1 > 2 > 3）
3. **磁盘使用率**：0 表示不检查，1-100 表示阈值百分比
4. **兼容性**：旧配置（字符串）仍然完全兼容
5. **降级路径**：`record.filepath` 始终作为最后的降级选项
