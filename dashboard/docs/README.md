# Monibuca快速起步
## 介绍
Monibuca 是一个开源的流媒体服务器开发框架，适用于快速定制化开发流媒体服务器，可以对接CDN厂商，作为回源服务器，也可以自己搭建集群部署环境。
丰富的内置插件提供了流媒体服务器的常见功能，例如rtmp server、http-flv、视频录制、QoS等。除此以外还内置了后台web界面，方便观察服务器运行的状态。
也可以自己开发后台管理界面，通过api方式获取服务器的运行信息。
Monibuca 提供了可供定制化开发的插件机制，可以任意扩展其功能。

## 使用实例管理器启动实例

### step0 配置golang环境

将GOPATH的bin目录加入环境变量PATH中，这样可以快速启动Monibuca实例管理器

### step1 安装Monibuca
```bash
go get github.com/langhuihui/monibuca
```
安装完成后会在GOPATH的bin目录下生成monibuca可执行文件

### step2 启动monibuca实例管理器
如果GOPATH的bin目录已经加入PATH环境变量，则可以直接执行
```bash
monibuca
```
程序默认监听8000端口，你也可以带上参数指定启动的端口
```bash
monibuca -port 8001
```
### step3 创建实例
浏览器打开上面的端口地址，出现实例管理器页面，点击创建标签页，按照提示选择实例放置的目录和插件，进行创建。
完成后会在所在目录创建若干文件并运行该golang项目，如果选择了网关插件，则可以在该插件配置的端口下看到控制台页面。

## 实例目录说明

1. main.go
2. config.toml
3. restart.sh

### main.go
实例启动的主文件，初始化各类插件，然后调用配置文件启动引擎
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
可以修改该主文件，添加任意功能

### config.toml

该配置文件主要是为了定制各个插件的配置，例如监听端口号等，具体还是要看各个插件的设计。

::: tip
如果你编写了自己的插件，就必须在该配置文件中写入对自己插件的配置信息
:::

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

### restart.sh
该文件是一个用来重启实例的bash脚本，方便通过实例管理器重启，或者手工重启。