
# vmlog

## 介绍
三个字段
```
_time:时间戳
_msg:正文
_stream:结构化日志需要采集的字段
```
其它字段，不会全文存储，查询的时候，需要指定字段名

## 查询

### 查询正文

直接输入即可
### 查询字段
字段名:字段值，支持嵌套
比如:
```
level:INFO
attrs.key1:value1
attrs.key1:*
log.level:in("error", "fatal")
log.level:(="error" OR ="fatal")
```

### 过滤时间
```
_time:5m 5分钟内
_time:day_range(08:00, 18:00] 8点-18点
```

### 更多过滤见[文档](https://docs.victoriametrics.com/victorialogs/logsql/)
- 前缀匹配比如 test*
- 忽略大小写 i(error)
- 序列匹配 seq("error", "open file")，词语跟随
- 正则 ~"err|warn"

extra.group字段.details.attempts:*
