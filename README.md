<p align="center">
  <a href="https://m7s.live">
    <img src="logo.png" height="96">
  </a>
</p>
<p align="center">
 <a href="https://monibuca.com/doc">中文文档</a>
</p>
# Core code base and plug-in code base

[https://github.com/Monibuca](https://github.com/Monibuca)

# Introduction

## What is Monibuca (m7s)?

Monibuca (pronounced: analog not card, m7s is its abbreviation, similar to k8s) is an open source streaming server development framework developed in Go. It is based on go1.19+, in addition to no other dependencies built, and provides a set of plug-in secondary development model to help you efficiently develop streaming media servers, you can directly use the official plug-in, or develop your own plug-in to extend any function, so Monibuca is a framework that can support any streaming protocol!

Monibuca consists of three parts: engine, plugins, and instance project.

The engine provides a common streaming data cache and forwarding mechanism, and does not care how the protocol is implemented
The plugins offer all the other features and can be extended indefinitely
An instance project is a project project that introduces the engine and plugins and starts the engine, and can be written entirely by yourself

## Plug-in framework

Monibuca aims to build a general streaming media development ecosystem, so since the v1 version, it has considered the decoupling of services and stream forwarding, so as to design a set of plug-in mechanisms that can be arbitrarily extended. Depending on your needs, you can flexibly introduce different types of plugins:

- Provide streaming media protocol packaging/unpacking, such as RTMP plug-ins, RTSP plug-ins, etc
- Provides log persistence processing - logrotate plugin
- Provide recording function - record plugin
- Provide rich debugging functions - debug plugin
- Provide HTTP callback capability - hook plugin
If you are an experienced developer, then the best way is to carry out secondary development on the basis of existing plugins, and provide reusable plugins to more people to enrich the ecosystem. If you're a beginner in streaming, the best way to do this is to use existing plugins to cobble together the features you need and ask experienced developers for help.

# Key features
## Engine aspect
- Provides a plug-in mechanism to manage plug-in startup, configuration resolution, event distribution, etc. in a unified manner
- Provide forwarding in H264, H265, AAC, G711 format
- Provide reusable AVCC format, RTP format, AnnexB format, ADTS format and other pre-encapsulation mechanisms
- Provides a multi-track mechanism, supports large and small streams, and encrypts stream expansion
- Provide DataTrack mechanism, which can be used to implement functions such as room text chat
- Provide timestamp synchronization mechanism and speed limit mechanism
- Provides an RTP packet reorder mechanism
- Provide subscriber frame chasing and skipping mechanism (first screen second on)
- Provides the infrastructure for publish-subscribe push and pull out
- Provides underlying architecture support for authentication mechanisms
- Provides a memory reuse mechanism
- Provides a mechanism for publishers to disconnect and reconnect
- Provides an on-demand flow pulling mechanism
- Provides a common mechanism for HTTP service ports
- Provides an automatic registration mechanism for HTTP API interfaces
- Provides HTTP interface middleware mechanism
- Provides structured logs
- Provides flow information statistics and output
- Provides an event bus mechanism that broadcasts events to all plug-ins
- Provides a configuration hot update mechanism

## Plug-in aspect
- Provide RTMP protocol push-pull stream, external push-pull stream (RTMPS supported)
- Provides RTSP push and pull streams and external push and pull streams
- Provides HTTP-FLV protocol to pull streams, pull external streams, and read local FLV files
- Provides streaming of the WebSocket protocol
- Provides HLS protocol to pull streams and pull outflows
- Provides push-pull streams for the WebRTC protocol
- Provides GB28181 protocol push and dump playback analysis capabilities
- Provide support for the Onif protocol
- Provides streaming of WebTransport protocol
- Provides FMP4 protocol for pulling streams
- Provides edge server functionality to implement cascading streaming
- Provide video recording function, support FLV, MP4, HLS, RAW formats
- Provides log persistence by day, hour, minute, second, size, and number of files
- Provide a screenshot function
- Provides HTTP callback function
- Preview features available (integrated with Jessibuca Pro)
- Room function available (video conferencing possible)
- Provide the function of docking with Prometheus

Third-party plugins and paid plugins provide additional functionality and are not listed here.

Inspired by:
- [mp4ff](https://github.com/edgeware/mp4ff) mp4 file format library [@edgeware](https://github.com/edgeware)
- [gosip](https://github.com/ghettovoice/gosip) go sip library [@ghettovoice](https://github.com/ghettovoice)
- [webrtc](https://github.com/pion/webrtc) go library and whole [@pion](https://github.com/pion) team
- [gortsplib](https://github.com/bluenviron/gortsplib) rtsp library [@aler9](https://github.com/aler9)

## Remote console

- Provides multi-instance management
- Provide flow details display
- Provides visual editing of configurations
- Provides visual display of logs
- Provide visual management of plugins
- Provides GB device management
- Provides an interface for dynamically adding remote push-pull flows
- Provide WebRTC background wall function
- Provide multiplayer video demonstrations

# Origin of the name
The word Monibuca is derived from (Monica), and in order to solve the naming problem, three names are used to represent server, player, and streamer. Since Monica, Jessica, and Rebecca all have `卡` words, which is not good for the live broadcast (ca - `卡` means block in Chinese), it was changed to Monibuca, Jessibuca(https://jessibuca.com), and Rebebuca(https://rebebuca.com). (bu-`不` means not)

# Install
- The compiled binary executable files (i.e. green software) of each platform are officially provided, so it can run without installing any other software.
- If you need to compile and start the project yourself, you need to install go1.19 or above.

The official download link of the latest version is provided:
- [Linux](https://download.m7s.live/bin/m7s_linux_arm64.tar.gz)
- [Linux-arm64](https://download.m7s.live/bin/m7s_linux_arm64.tar.gz)
- [Mac](https://download.m7s.live/bin/m7s_darwin_arm64.tar.gz)
- [Mac-arm64](https://download.m7s.live/bin/m7s_darwin_arm64.tar.gz)
- [Windows](https://download.m7s.live/bin/m7s_windows_amd64.tar.gz)

Don't forget to fix the rights chmod +x m7s_xxx_xxx on Linux and Mac.
# Run

## Executable files run directly

- Linux, for example, downloaded to `/opt/m7s_linux_x86`, then `$ cd /opt ' and then `$ ./m7s_linux_x86`
- Similar to Linux and Mac, you may need to modify the executable permissions of the file or double-click to run
- Windows, double-click m7s directly_windows_x86.exe can be started

## Docker
```bash
docker run -id -p 1935:1935 -p 8080:8080 -p 8443:8443 -p 554:554 -p 58200:58200 -p 5060:5060/udp -p 8000:8000/udp -p 9000:9000 langhuihui/monibuca:latest
```

## Self-compiled startup project
1. `git clone https://github.com/langhuihui/monibuca`
2. `cd monibuca`
3. `go run .`

## Self-created startup project

You can watch the video tutorial:

- [从零启动 m7s V4](https://www.bilibili.com/video/BV1iq4y147N4/)

- [m7s v4 视频教程——插件引入](https://www.bilibili.com/video/BV1sP4y1g7BF/)

![公众号](https://m7s.live/images/m7s/footer/wx-mp.jpg)