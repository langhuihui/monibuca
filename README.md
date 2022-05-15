# 主页

[https://m7s.live](https://m7s.live)

# 中文文档

[https://m7s.live/guide/introduction.html](https://m7s.live/guide/introduction.html)

# 文章

[重新定义流媒体服务器](https://www.infoq.cn/article/uiPl8dIuQmhipKb3q3Tz)

# 核心代码库和插件代码库

[https://github.com/Monibuca](https://github.com/Monibuca)

# 介绍

## 什么是Monibuca（m7s)？

Monibuca(发音：模拟不卡，m7s是其缩写，类似k8s) 是一个开源的Go语言开发的流媒体服务器开发框架。
它基于go1.18+，此外并无任何其他依赖构建，并提供了一套插件式的二次开发模型，帮助你高效地开发流媒体服务器，你既可以直接使用官方提供的插件，也可以自己开发插件扩展任意的功能，所以Monibuca是可以支持**任意**流媒体协议的框架！


> 流媒体服务器是一种用于分发流媒体的服务器端软件，可用于直播、监控、会议等需要实时观看音视频的场景。流媒体服务器区别于传统Web服务器对于实时性要求极高，需要使用各种传输协议，而Web服务器则主要以http/https协议为主。

Monibuca由三部分组成：引擎、插件、实例工程。
- 引擎提供一套通用的流媒体数据缓存以及转发的机制，本身不关心协议如何实现
- 插件提供其他所有的功能，并可以无限扩展
- 实例工程是引入引擎和插件并启动引擎的项目工程，可以完全自己编写

## 插件式框架

Monibuca旨在构建一个通用的流媒体开发生态，所以从v1版本开始就考虑到业务和流转发的解耦，从而设计了一套可供任意扩展的插件机制。根据你的需求场景，可以灵活引入不同类型的插件：
- 提供流媒体协议打包/解包，例如rtmp插件、rtsp插件等
- 提供日志持久化的处理——logrotate插件
- 提供录像功能——record插件
- 提供丰富的调试功能——debug插件
- 提供http回调能力——http插件

如果你是有经验的开发者，那么最佳的方式是在现有的插件基础上进行二次开发，并可向更多的人提供可重用的插件丰富生态。
如果你是流媒体的初学者，那么最佳的方式是利用现有的插件拼凑出你需要的功能，并向有经验的开发者寻求帮助。


## 名称的由来
Monibuca这个单词来源于 `Monica` （莫妮卡），为了解决起名的难题，使用了三个名称分别是 `Monica` 、 `Jessica` 、`Rebecca` 用来代表服务器、播放器、推流器。由于莫妮卡、杰西卡、瑞贝卡，都带卡字，对直播来说寓意不好，所以改为莫妮不卡（`Monibuca`）、杰西不卡[Jessibuca](https://jessibuca.com)、瑞贝不卡[Rebebuca](https://rebebuca.com)。

## 安装
- 官方提供已编译好的各个平台的二进制可执行文件（即绿色软件），所以无需安装任何其他软件即可运行。
- 如果需要自己编译启动工程，则需要安装go1.18以上版本。

:::tip 配置go环境
- go可以在https://golang.google.cn/dl 中下载到
- 国内需要执行go env -w GOPROXY=https://goproxy.cn 来下载到被屏蔽的第三方库
:::

官方提供了最新版本的下载链接：
- [Linux](https://m7s.live/bin/m7s_linux_x86)
- [Linux-arm64](https://m7s.live/bin/m7s_linux_arm64)
- [Mac](https://m7s.live/bin/m7s_darwin_x86)
- [Mac-arm64](https://m7s.live/bin/m7s_darwin_arm64)
- [Windows](https://m7s.live/bin/m7s_windows_x86)

## 运行

### 可执行文件直接运行

- Linux 例如下载到了/opt/m7s_linux_x86,则 `cd /opt` 然后 `./m7s_linux_x86`
- Mac 和Linux类似，需要注意的时候可能需要修改文件的可执行权限，也可以双击运行
- Windows，直接双击m7s_windows_x86.exe即可启动

:::tip 运行多实例
由于实例会监听http端口，所以如果需要运行多实例，就需要为每个实例指定不同的http端口，因此需要启动时指定配置文件，例如./m7s_linux_x86 -c config.yaml
:::

### 自行编译启动工程
1. `git clone https://github.com/langhuihui/monibuca`
2. `cd monibuca`
3. `go run main.go`

### 自行创建启动工程

可以观看视频教程：

- [从零启动 m7s V4](https://www.bilibili.com/video/BV1iq4y147N4/)

- [m7s v4 视频教程——插件引入](https://www.bilibili.com/video/BV1sP4y1g7BF/)