# Jessica
该插件为基于WebSocket协议传输音视频的订阅者，音视频数据以裸数据的形式进行传输，我们需要Jessibuca播放器来进行播放
Jessibua播放器已内置于源码中，该播放器通过js解码H264并用canvas进行渲染，可以运行在几乎所有的终端浏览器上面。
在Monibuca的Web界面中预览功能就是使用的Jessibuca播放器。
## 配置
目前仅有的配置是监听的端口号
```toml
[Plugins.Jessica]
ListenAddr = ":8080"
```
