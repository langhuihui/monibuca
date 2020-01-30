# 插件开发
## 插件的定义
所谓的插件，没有什么固定的规则，只需要完成`安装`操作即可。插件可以实现任意的功能扩展，最常见的是实现某种传输协议用来推流或者拉流

## 插件的安装
下面是内置插件jessica的源码，代表了典型的插件安装
```go
package jessica

import (
	. "github.com/langhuihui/monibuca/monica"
	"log"
	"net/http"
)

var config = new(ListenerConfig)

func init() {
	InstallPlugin(&PluginConfig{
		Name:   "Jessica",
		Type:   PLUGIN_SUBSCRIBER,
		Config: config,
		Run:    run,
	})
}
func run() {
	log.Printf("server Jessica start at %s", config.ListenAddr)
	log.Fatal(http.ListenAndServe(config.ListenAddr, http.HandlerFunc(WsHandler)))
}
```
当主程序读取配置文件完成解析后，会调用各个插件的Run函数，上面代码中执行了一个http的端口监听

## 开发订阅者插件
所谓订阅者就是用来从流媒体服务器接收音视频流的程序，例如RTMP协议执行play命令后、http-flv请求响应程序、websocket响应程序。内置插件中录制flv程序也是一个特殊的订阅者。
下面是http-flv插件的源码，供参考
```go
package HDL

import (
	. "github.com/langhuihui/monibuca/monica"
	"github.com/langhuihui/monibuca/monica/avformat"
	"github.com/langhuihui/monibuca/monica/pool"
	"log"
	"net/http"
	"strings"
)

var config = new(ListenerConfig)

func init() {
	InstallPlugin(&PluginConfig{
		Name:   "HDL",
		Type:   PLUGIN_SUBSCRIBER,
		Config: config,
		Run:    run,
	})
}

func run() {
	log.Printf("HDL start at %s", config.ListenAddr)
	log.Fatal(http.ListenAndServe(config.ListenAddr, http.HandlerFunc(HDLHandler)))
}

func HDLHandler(w http.ResponseWriter, r *http.Request) {
	sign := r.URL.Query().Get("sign")
	if err := AuthHooks.Trigger(sign); err != nil {
		w.WriteHeader(403)
		return
	}
	stringPath := strings.TrimLeft(r.RequestURI, "/")
	if strings.HasSuffix(stringPath, ".flv") {
		stringPath = strings.TrimRight(stringPath, ".flv")
	}
	if _, ok := AllRoom.Load(stringPath); ok {
		//atomic.AddInt32(&hdlId, 1)
		w.Header().Set("Transfer-Encoding", "chunked")
		w.Header().Set("Content-Type", "video/x-flv")
		w.Write(avformat.FLVHeader)
		p := OutputStream{
			Sign: sign,
			SendHandler: func(packet *pool.SendPacket) error {
				return avformat.WriteFLVTag(w, packet)
			},
			SubscriberInfo: SubscriberInfo{
				ID: r.RemoteAddr, Type: "FLV",
			},
		}
		p.Play(stringPath)
	} else {
		w.WriteHeader(404)
	}
}
```
其中，核心逻辑就是创建OutputStream对象，每一个订阅者需要提供SendHandler函数，用来接收来自发布者广播出来的音视频数据。
最后调用该对象的Play函数进行播放。请注意：Play函数会阻塞当前goroutine。

## 开发发布者插件

所谓发布者，就是提供音视频数据的程序，例如接收来自OBS、ffmpeg的推流的程序。内置插件中，集群功能里面有一个特殊的发布者，它接收来自源服务器的音视频数据，然后在本服务器中广播音视频。
以此为例，我们需要提供一个结构体定义来表示特定的发布者：
```go
type Receiver struct {
	InputStream
	io.Reader
	*bufio.Writer
}
```
其中InputStream 是固定的，必须包含，且必须以组合继承的方式定义。其余的成员则是任意的。
发布者的发布动作需要特定条件的触发，例如在集群插件中，当本服务器有订阅者订阅了某个流，而该流并没有发布者的时候就会触发向源服务器拉流的函数：
```go
func PullUpStream(streamPath string) {
	addr, err := net.ResolveTCPAddr("tcp", config.Master)
	if MayBeError(err) {
		return
	}
	conn, err := net.DialTCP("tcp", nil, addr)
	if MayBeError(err) {
		return
	}
	brw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	p := &Receiver{
		Reader: conn,
		Writer: brw.Writer,
	}
	if p.Publish(streamPath, p) {
		p.WriteByte(MSG_SUBSCRIBE)
		p.WriteString(streamPath)
		p.WriteByte(0)
		p.Flush()
		for _, v := range p.Subscribers {
			p.Auth(v)
		}
	} else {
		return
	}
	defer p.Cancel()
	for {
		cmd, err := brw.ReadByte()
		if MayBeError(err) {
			return
		}
		switch cmd {
		case MSG_AUDIO:
			if audio, err := p.readAVPacket(avformat.FLV_TAG_TYPE_AUDIO); err == nil {
				p.PushAudio(audio)
			}
		case MSG_VIDEO:
			if video, err := p.readAVPacket(avformat.FLV_TAG_TYPE_VIDEO); err == nil && len(video.Payload) > 2 {
				tmp := video.Payload[0]         // 第一个字节保存着视频的相关信息.
				video.VideoFrameType = tmp >> 4 // 帧类型 4Bit, H264一般为1或者2
				p.PushVideo(video)
			}
		case MSG_AUTH:
			cmd, err = brw.ReadByte()
			if MayBeError(err) {
				return
			}
			bytes, err := brw.ReadBytes(0)
			if MayBeError(err) {
				return
			}
			subId := strings.Split(string(bytes[0:len(bytes)-1]), ",")[0]
			if v, ok := p.Subscribers[subId]; ok {
				if cmd != 1 {
					v.Cancel()
				}
			}
		}
	}
}

```
正在该函数中会向源服务器建立tcp连接，然后发送特定命令表示需要拉流，当我们接收到源服务器的数据的时候，就调用PushVideo和PushAudio函数来广播音视频。

核心逻辑是调用InputStream的Publish以及PushVideo、PushAudio函数

## 开发钩子插件

钩子插件就是在服务器的关键逻辑处插入的函数调用，方便扩展服务器的功能，比如对连接进行验证，或者触发一些特殊的发布者。
目前提供的钩子包括
- 当发布者开始发布时 `OnPublishHooks.AddHook(onPublish)`
例如：
```go
func onPublish(r *Room) {
	for _, v := range r.Subscribers {
		if err := CheckSign(v.Sign); err != nil {
			v.Cancel()
		}
	}
}
```
此时可以访问房间里面的订阅者，对其进行验证。
- 当有订阅者订阅了某个流时，`OnSubscribeHooks.AddHook(onSubscribe)`
例如：
```go
func onSubscribe(s *OutputStream) {
	if s.Publisher == nil {
		go PullUpStream(s.StreamPath)
	}
}

```
拉取源服务器的流

