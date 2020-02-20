# 内置插件介绍
内置插件为Monibuca提供了许多基础功能，当然你完全可以不采用内置插件，而改用自己开发的插件，也丝毫不会影响您使用Monibuca。

## 网关插件
::: tip 源码位置
该插件位于plugins/gateway下
:::

该插件是为web控制台界面提供api，用来采集服务器的信息。

### 配置
目前仅有的配置是监听的端口号

```toml
[Plugins.GateWay]
ListenAddr = ":80"
```
如果80端口有其他用途，可以换成别的端口，比如有nginx反向代理。

## 日志分割插件
::: tip 源码位置
该插件源码位于plugins/logrotate下
:::

### 配置
```toml
[Plugins.LogRotate]
Path = "log"
Size = 0
Days = 1
```
其中Path代表生成日志的目录
Size代表按大小分割，单位是字节，如果为0，则按时间分割
Days代表按时间分割，单位是天，即24小时

## Jessica插件
::: tip 源码位置
该插件源码位于plugins/jessica下
:::

该插件为基于WebSocket协议传输音视频的订阅者，音视频数据以裸数据的形式进行传输，我们需要Jessibuca播放器来进行播放
Jessibua播放器已内置于源码中，该播放器通过js解码H264/H265并用canvas进行渲染，可以运行在几乎所有的终端浏览器上面。
在Monibuca的Web界面中预览功能就是使用的Jessibuca播放器。
### 配置
目前仅有的配置是监听的端口号
```toml
[Plugins.Jessica]
ListenAddr = ":8080"
```
### Flv格式支持
Jessica以及Jessibuca也支持采用WebSocket中传输Flv格式的方式进行通讯，目前有部分CDN厂商已经支持这种方式进行传输。
>私有协议以及Flv格式的判断是通过URL后缀是否带有.flv来进行判断

## Rtmp插件
> 该插件源码位于plugins/rtmp下

实现了基本的rtmp传输协议，包括接收来自OBS、ffmpeg等软件的推流，以及来在Flash Player播放器的拉流。

### 配置
目前仅有的配置是监听的端口号
```toml
[Plugins.RTMP]
ListenAddr = ":1935"
```

## RecordFlv插件
> 该插件源码位于plugins/record下

实现了录制Flv文件的功能，并且支持再次使用录制好的Flv文件作为发布者进行发布。在Monibuca的web界面的控制台中提供了对房间进行录制的操作按钮，以及列出所有已经录制的文件的界面。

### 配置
配置中的Path 表示要保存的Flv文件的根路径，可以使用相对路径或者绝对路径
```toml
[Plugins.RecordFlv]
Path="./resource"
```

## Http-Flv插件
> 该插件位于plugins/HDL下

实现了http-flv格式的拉流功能，方便对接CDN厂商

### 配置
目前仅有的配置是监听的端口号
```toml
[Plugins.HDL]
ListenAddr = ":2020"
```

## Cluster插件
> 该插件源码位于plugins/cluster下

实现了基本的集群功能，里面包含一对发布者和订阅者，分别在主从服务器中启用，进行连接。
起基本原理就是，在主服务器启动端口监听，从服务器收到播放请求时，如果从服务器没有对应的发布者，则向主服务器发起请求，主服务器收到来自从服务器的请求时，将该请求作为一个订阅者。从服务器则把tcp连接作为发布者，实现视频流的传递过程。

### 配置

主服务器的配置是ListenAddr，用来监听从服务器的请求。
从服务器的配置是Master,表示主服务器的地址。
当然服务器可以既是主也是从，即充当中转站。

```toml
[Plugins.Cluster]
Master = "localhost:2019"
ListenAddr = ":2019"
```

## HLS插件
> 该插件源码位于plugins/HLS下

该插件的作用是请求M3u8文件进行解码，最终将TS视频流转码成裸的视频流进行发布。
注意：该插件目前并没有实现生成HLS的功能。


## 校验插件
> 该插件位于plugins/auth下

该插件提供了基本的验证功能，其原理是
订阅流提供一个签名，签名只可以使用一次，把签名进行AES CBC 解密，如果得到的解密字符串的前面部分就是和Key相同则通过验证。

### 配置
Key代表用来加密的Key
```toml
[Plugins.Auth]
Key="www.monibuca.com"
```