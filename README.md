
<h2 align="center">
<img src="https://monibuca.com/img/logo.b5357057.png"></h2>

# Introduction

🧩 Monibuca is a Modularized, Extensible framework for building Streaming Server. 
- Customize the server by combining function plug-ins. 
- It's easy to develop plug-ins to implement business logic. 
- Reduce enterprise development cost and improve development efficiency

# Quick start

## Go has not been installed
```
bash <(curl -s -S -L https://monibuca.com/demo.sh) 
```
## Go is already installed

1. go get github.com/langhuihui/monibuca
2. $GOPATH/bin/monibuca
3. open your browser http://localhost:8081
4. use ffmpeg or OBS to push video streaming to rtmp://localhost/live/user1

# Advanced

1. go get github.com/Monibuca/monica
2. $GOPATH/bin/monica
3. open your browser http://localhost:8000
4. follow the guide to create your project

# Ecosystem

go to 
[https://plugins.monibuca.com](https://plugins.monibuca.com).
to submit your own plugin

| Project | Description  |
|---------| -------------|
|[rtmpplugin]|rtmp protocol support.push rtmp stream to monibuca.play stream from monibuca.
|[rtspplugin]|rtsp protocol support.pull rtsp stream to monibuca
|[hlsplugin]|pull hls stream to monibuca
|[tsplugin]|used by hlsplugin. read ts file to publish
|[hdlplugin]|http-flv protocol support. pull http-flv stream from monibuca
|[gatewayplugin]|a console and dashboard to display information and status of monibuca ,also can display UI of other plugins 
|[recordplugin]|record multimedia stream to flv files
|[clusterplugin]|cascade transmission of multimedia by cluster network
|[jessicaplugin]|play multimedia stream through websocket protocol

[rtmpplugin]: https://github.com/Monibuca/rtmpplugin
[rtspplugin]: https://github.com/Monibuca/rtspplugin
[hlsplugin]:https://github.com/Monibuca/hlspplugin
[tsplugin]:https://github.com/Monibuca/tspplugin
[hdlplugin]:https://github.com/Monibuca/hdlplugin
[gatewayplugin]:https://github.com/Monibuca/gatewayplugin
[recordplugin]:https://github.com/Monibuca/recordplugin
[clusterplugin]:https://github.com/Monibuca/clusterplugin
[jessicaplugin]:https://github.com/Monibuca/jessicaplugin

# Documentation

[http://docs.monibuca.com/en](http://docs.monibuca.com/en).

中文文档：
[http://docs.monibuca.com](http://docs.monibuca.com).

# Contact

wechat group:

![wechat](https://monibuca.com/wechat.png?t=4.30)

# Q&A

## Q: There are so many streaming server projects in the world，why need to create Monibuca?

A: Monibuca is different from other streaming servers,that it was created for facilitate secondary development.

## Q: Why use golang?

A: Golang is a greate programming language. It is very suited to build streaming server since streaming server is a kind of IO intensive system. Goroutine is good at doing these jobs. Another important reason of using Golang is that people read the source code or doing secondary development easier.

## Q: What does "Monibuca" mean?

A: No special meaning. Just from monica —— a girl name. 
