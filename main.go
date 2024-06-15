package main

//go:generate go run gen.go $debug

/*
███    ███  ██████  ███    ██ 👑 ██████  ██    ██  ██████  █████
████  ████ ██    ██ ████   ██ ██ ██   ██ ██    ██ ██      ██   ██
██ ████ ██ ██    ██ ██ ██  ██ ██ ██████  ██    ██ ██      ███████
██  ██  ██ ██    ██ ██  ██ ██ ██ ██   ██ ██    ██ ██      ██   ██
██      ██  ██████  ██   ████ ██ ██████   ██████   ██████ ██   ██

The live stream server for Go
(c) dexter 2019-present

说明：
本项目为 monibuca 的启动工程，你也可以自己创建一个启动工程
本启动工程引入了 engine 和一些列官方插件，并且保证版本依赖关系
自己创建工程的时候，版本依赖必须参考本工程，否则容易出现依赖关系错乱
流的播放地址请查看文档：https://m7s.live/guide/qa/play.html
推拉流的配置方法看文档：https://m7s.live/guide/config.html#%E6%8F%92%E4%BB%B6%E9%85%8D%E7%BD%AE

高频问题：
1、OBS只能推送 rtmp 协议,如需推送 rtsp 需要安装插件
2、除了rtsp协议以外其他协议播放H265需要使用jessibuca播放器（preview 插件内置了jessibuca播放器）
3、浏览器不能直接播放rtmp、rtsp等基于tcp的协议，因为在js的环境中，无法直接使用tcp或者udp传数据（js没提供接口），而rtsp或rtmp的流是基于tcp或者udp， 所以纯web的方式目前是没办法直接播放rtsp或rtmp流的
4、webrtc是否可以播放h265取决于浏览器是否包含h265解码器（通常不包含）
5、webrtc不支持aac格式的音频
6、gb插件能收到设备的注册，但是没有流，可能：1、媒体端口被防火墙拦截默认是58200，2、使用公网IP需要配置sipip字段或者mediaip字段用于设备向指定IP发送流。3、配置范围端口（部分设备ssrc乱搞导致的）
7、当没有订阅者的时候如何自动停止拉流：设置publish 配置下的 delayclosetimeout 参数例如 10s，代表最后一个订阅者离开后 10s 后自动停止流
8、使用 ffmpeg 推流时请加-c:v h264 -c:a aac，否则 ffmpeg 会自动将流转换成系统不支持的格式
9、StreamPath 必须形如 live/test 。不能只有一级，或者斜杠开头，如/live 是错误的。
10、如果遇到直接退出（崩溃）查看一下fatal.log。
*/

import (
	"context"
	"flag"
	"fmt"
	"os"

	"m7s.live/engine/v4"
	"m7s.live/engine/v4/util"

	_ "m7s.live/plugin/debug/v4"
	_ "m7s.live/plugin/fmp4/v4"
	_ "m7s.live/plugin/gb28181/v4"
	_ "m7s.live/plugin/hdl/v4"
	_ "m7s.live/plugin/hls/v4"
	_ "m7s.live/plugin/hook/v4"
	_ "m7s.live/plugin/jessica/v4"
	_ "m7s.live/plugin/logrotate/v4"
	_ "m7s.live/plugin/monitor/v4"
	_ "m7s.live/plugin/preview/v4"
	_ "m7s.live/plugin/record/v4"
	_ "m7s.live/plugin/room/v4"
	_ "m7s.live/plugin/rtmp/v4"
	_ "m7s.live/plugin/rtsp/v4"
	_ "m7s.live/plugin/snap/v4"
	_ "m7s.live/plugin/webrtc/v4"
	_ "m7s.live/plugin/webtransport/v4"
)

var (
	version = "dev"
)

func main() {
	fmt.Println("start github.com/langhuihui/monibuca version:", version)
	confPathFromEnv := os.Getenv("BUCA_CONFIG_FILE")
	if confPathFromEnv == "" {
		confPathFromEnv = "config.yaml" // 如果环境变量未设置，默认使用此路径
	}
	fmt.Println("=== config file: ", confPathFromEnv)
	conf := flag.String("c", confPathFromEnv, "config file")
	flag.Parse()
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), "version", version))
	go util.WaitTerm(cancel)
	engine.Run(ctx, *conf)
}
