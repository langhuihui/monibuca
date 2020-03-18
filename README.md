# Introduction

🧩 Monibuca is a Modularized, Extensible framework for building Streaming Server. 

# Quick start

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

[rtmpplugin]: https://github.com/Monibuca/rtmpplugin
[rtspplugin]: https://github.com/Monibuca/rtspplugin
[hlsplugin]:https://github.com/Monibuca/hlspplugin
[tsplugin]:https://github.com/Monibuca/tspplugin
[hdlplugin]:https://github.com/Monibuca/hdlplugin
[gatewayplugin]:https://github.com/Monibuca/gatewayplugin
[recordplugin]:https://github.com/Monibuca/recordplugin
[clusterplugin]:https://github.com/Monibuca/clusterplugin

# Documentation

To check out live examples and docs, visit [https://monibuca.com](https://monibuca.com).

# Contact

wechat group:

![wechat](https://monibuca.com/wechat.png?t=3.18)

# Q&A

## Q: There are so many streaming server projects in the world，why need to create Monibuca?

A: Monibuca is different from other streaming servers,that it was created for facilitate secondary development.

## Q: Why use golang?

A: Golang is a greate programming language. It is very suited to build streaming server since streaming server is a kind of IO intensive system.