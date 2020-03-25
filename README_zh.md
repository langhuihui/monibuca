# 主页

[https://monibuca.com](https://monibuca.com)

# 中文文档

[http://docs.monibuca.com](http://docs.monibuca.com)

# 核心代码库和插件代码库

[https://github.com/Monibuca](https://github.com/Monibuca)


# 本项目为开箱即用的实例demo

1. go get github.com/langhuihui/monibuca
2. linux下执行：$GOPATH/bin/monibuca  windows下执行：%GOPATH%/bin/monibuca
3. 启动后，浏览器打开8081端口查看后台界面
4. ffmpeg或者OBS推流到1935端口
5. 后台界面上提供直播预览、录制flv、rtsp拉流转发、日志跟踪等功能

# Monibuca简介
[Monibuca](https://monibuca.com) 是一个开源的流媒体服务器开发框架，适用于快速定制化开发流媒体服务器，可以对接CDN厂商，作为回源服务器，也可以自己搭建集群部署环境。 丰富的内置插件提供了流媒体服务器的常见功能，例如rtmp server、http-flv、视频录制、QoS等。除此以外还内置了后台web界面，方便观察服务器运行的状态。 也可以自己开发后台管理界面，通过api方式获取服务器的运行信息。 Monibuca 提供了可供定制化开发的插件机制，可以任意扩展其功能。

⚡高性能
 
针对流媒体服务器独特的性质进行的优化，充分利用Golang的goroutine的性质对大量的连接的读写进行合理的分配计算资源，以及尽可能的减少内存Copy操作。使用对象池减少Golang的GC时间。
 
🔧可扩展
 
流媒体服务器的个性化定制变的更简单，基于Golang语言，开发效率更高，独创的插件机制，可以方便用户定制个性化的功能组合，更高效率的利用服务器资源。[插件市场](https://plugins.monibuca.com)
 
📈可视化
 
功能强大的仪表盘可以直观的看到服务器运行的状态、消耗的资源、以及其他统计信息。用户可以利用控制台对服务器进行配置和控制。

# 交流微信群

![交流微信群](https://monibuca.com/wechat.png?t=3.25)

# Q&A

## Q：流媒体服务器项目有很多，为什么要重复发明轮子？
A: Monibuca不同于其他流媒体服务器的地方是，针对二次开发为目的。多数流媒体服务器是通用型，完成特定任务的，对于二次开发并不友好。Monibuca开创了插件机制，可以自由组合不同的协议或者功能，定制化特定需求的流媒体服务器。

## Q：Monibuca为何采用Golang为开发语言？
A：因为Golang语言相比其他语言可读性更强，代码简单易懂，更利于二次开发；另外Golang的goroutine特别适合开发高速系统。

## Q：Monibuca是否使用Cgo或者其他语言依赖库？
A：没有。Monibuca是纯Go语言开发，不依赖任何其他第三方库比如FFmpeg，方便二次开发。对部署更友好，仅仅需要Golang运行环境即可。

## Q：Monibuca对环境有什么要求？直播流可以在微信里播放吗？
A：Monibuca是基于Golang开发，支持跨平台部署。Monibuca可以用Jessibuca播放器在微信、手机浏览器里面播放视频。也可以通过其他SDK播放RTMP流、其他协议的流。只需要相应的插件支持即可。

## Q: Monibuca的名称有什么特殊含义吗？
A: 这个单词来源于Monica（莫妮卡）是个人名，在项目里面也存在这个文件夹。没有特别含义，为了解决起名的难题，使用了三个名称分别是Monica、Jessica、Rebecca用来代表服务器、播放器、推流器。由于莫妮卡、杰西卡、瑞贝卡，都带卡字，对直播来说寓意不好，所以改为模拟不卡（Monibuca）、解析不卡（Jessibuca）、累呗不卡（Rebebuca）。其中推流器Rebebuca目前尚为公布，是改造了的OBS，可用于推流H265
