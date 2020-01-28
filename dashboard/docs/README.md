# Monibuca快速起步
## 介绍
Monibuca 是一个开源的流媒体服务器开发框架，适用于快速定制化开发流媒体服务器，可以对接CDN厂商，作为回源服务器，也可以自己搭建集群部署环境。
丰富的内置插件提供了流媒体服务器的常见功能，例如rtmp server、http-flv、视频录制、QoS等。除此以外还内置了后台web界面，方便观察服务器运行的状态。
也可以自己开发后台管理界面，通过api方式获取服务器的运行信息。
Monibuca 提供了可供定制化开发的插件机制，可以任意扩展其功能。

## 启动
启用所有内置插件
```go
package main

import (
	. "github.com/langhuihui/monibuca/monica"
	_ "github.com/langhuihui/monibuca/plugins"
)

func main() {
	Run("config.toml")
	select {}
}
```

## 配置

要使用`Monibuca`,需要编写一个`toml`格式的配置文件，通常可以放在程序的同级目录下例如：`config.toml`(名称不是必须为`config`)

该配置文件主要是为了定制各个插件的配置，例如监听端口号等，具体还是要看各个插件的设计。

> 如果你编写了自己的插件，就必须在该配置文件中写入对自己插件的配置信息

如果注释掉部分插件的配置，那么该插件就不会启用，典型的配置如下：
```toml
[Plugins.HDL]
ListenAddr = ":2020"
[Plugins.Jessica]
ListenAddr = ":8080"
[Plugins.RTMP]
ListenAddr = ":1935"
[Plugins.GateWay]
ListenAddr = ":81"
#[Plugins.Cluster]
#Master = "localhost:2019"
#ListenAddr = ":2019"
#
#[Plugins.Auth]
#Key="www.monibuca.com"
#[Plugins.RecordFlv]
#Path="./resouce"
[Plugins.QoS]
Suffix = ["high","medium","low"]
```
具体配置的含义，可以参考每个插件的说明