package rtmp

import (
	"encoding/binary"
	"errors"
	"strings"

	"m7s.live/v5/pkg/util"
)

// https://zhuanlan.zhihu.com/p/196743129
const (
	/* RTMP Message ID*/

	// Protocal Control Messgae(1-7)

	// Chunk
	RTMP_MSG_CHUNK_SIZE = 1
	RTMP_MSG_ABORT      = 2

	// RTMP
	RTMP_MSG_ACK           = 3
	RTMP_MSG_USER_CONTROL  = 4
	RTMP_MSG_ACK_SIZE      = 5
	RTMP_MSG_BANDWIDTH     = 6
	RTMP_MSG_EDGE          = 7
	RTMP_MSG_AUDIO         = 8
	RTMP_MSG_VIDEO         = 9
	RTMP_MSG_AMF3_METADATA = 15
	RTMP_MSG_AMF3_SHARED   = 16
	RTMP_MSG_AMF3_COMMAND  = 17

	RTMP_MSG_AMF0_METADATA = 18
	RTMP_MSG_AMF0_SHARED   = 19
	RTMP_MSG_AMF0_COMMAND  = 20

	RTMP_MSG_AGGREGATE = 22

	RTMP_DEFAULT_CHUNK_SIZE = 128
	RTMP_MAX_CHUNK_SIZE     = 65536
	RTMP_MAX_CHUNK_HEADER   = 18

	// User Control Event
	RTMP_USER_STREAM_BEGIN       = 0
	RTMP_USER_STREAM_EOF         = 1
	RTMP_USER_STREAM_DRY         = 2
	RTMP_USER_SET_BUFFLEN        = 3
	RTMP_USER_STREAM_IS_RECORDED = 4
	RTMP_USER_PING_REQUEST       = 6
	RTMP_USER_PING_RESPONSE      = 7
	RTMP_USER_EMPTY              = 31

	// StreamID == (ChannelID-4)/5+1
	// ChannelID == Chunk Stream ID
	// StreamID == Message Stream ID
	// Chunk Stream ID == 0, 第二个byte + 64
	// Chunk Stream ID == 1, (第三个byte) * 256 + 第二个byte + 64
	// Chunk Stream ID == 2.
	// 2 < Chunk Stream ID < 64(2的6次方)
	RTMP_CSID_CONTROL = 0x02
	RTMP_CSID_COMMAND = 0x03
	RTMP_CSID_AUDIO   = 0x06
	RTMP_CSID_DATA    = 0x05
	RTMP_CSID_VIDEO   = 0x05
)

func newChunkHeader(messageType byte) *ChunkHeader {
	head := new(ChunkHeader)
	head.ChunkStreamID = RTMP_CSID_CONTROL
	if messageType == RTMP_MSG_AMF0_COMMAND {
		head.ChunkStreamID = RTMP_CSID_COMMAND
	}
	head.MessageTypeID = messageType
	return head
}

func (h ChunkHeader) Clone() *ChunkHeader {
	return &h
}

type RtmpMessage interface {
	Encode(IAMF)
}
type HaveStreamID interface {
	GetStreamID() uint32
}

func GetRtmpMessage(chunk *Chunk, body util.Buffer) error {
	switch chunk.MessageTypeID {
	case RTMP_MSG_CHUNK_SIZE, RTMP_MSG_ABORT, RTMP_MSG_ACK, RTMP_MSG_ACK_SIZE:
		if body.Len() < 4 {
			return errors.New("chunk.Body < 4")
		}
		chunk.MsgData = Uint32Message(body.ReadUint32())
	case RTMP_MSG_USER_CONTROL: // RTMP消息类型ID=4, 用户控制消息.客户端或服务端发送本消息通知对方用户的控制事件.
		{
			if body.Len() < 2 {
				return errors.New("UserControlMessage.Body < 2")
			}
			base := UserControlMessage{
				EventType: body.ReadUint16(),
				EventData: body,
			}
			switch base.EventType {
			case RTMP_USER_STREAM_BEGIN: // 服务端向客户端发送本事件通知对方一个流开始起作用可以用于通讯.在默认情况下,服务端在成功地从客户端接收连接命令之后发送本事件,事件ID为0.事件数据是表示开始起作用的流的ID.
				m := &StreamIDMessage{
					UserControlMessage: base,
					StreamID:           0,
				}
				if len(base.EventData) >= 4 {
					//服务端在成功地从客户端接收连接命令之后发送本事件,事件ID为0.事件数据是表示开始起作用的流的ID.
					m.StreamID = body.ReadUint32()
				}
				chunk.MsgData = m
			case RTMP_USER_STREAM_EOF, RTMP_USER_STREAM_DRY, RTMP_USER_STREAM_IS_RECORDED: // 服务端向客户端发送本事件通知客户端,数据回放完成.果没有发行额外的命令,就不再发送数据.客户端丢弃从流中接收的消息.4字节的事件数据表示,回放结束的流的ID.
				chunk.MsgData = &StreamIDMessage{
					UserControlMessage: base,
					StreamID:           body.ReadUint32(),
				}
			case RTMP_USER_SET_BUFFLEN: // 客户端向服务端发送本事件,告知对方自己存储一个流的数据的缓存的长度(毫秒单位).当服务端开始处理一个流得时候发送本事件.事件数据的头四个字节表示流ID,后4个字节表示缓存长度(毫秒单位).
				chunk.MsgData = &SetBufferMessage{
					StreamIDMessage: StreamIDMessage{
						UserControlMessage: base,
						StreamID:           body.ReadUint32(),
					},
					Millisecond: body.ReadUint32(),
				}
			case RTMP_USER_PING_REQUEST: // 服务端通过本事件测试客户端是否可达.事件数据是4个字节的事件戳.代表服务调用本命令的本地时间.客户端在接收到kMsgPingRequest之后返回kMsgPingResponse事件
				chunk.MsgData = &PingRequestMessage{
					UserControlMessage: base,
					Timestamp:          body.ReadUint32(),
				}
			case RTMP_USER_PING_RESPONSE, RTMP_USER_EMPTY: // 客户端向服务端发送本消息响应ping请求.事件数据是接kMsgPingRequest请求的时间.
				chunk.MsgData = &base
			default:
				chunk.MsgData = &base
			}
		}
	case RTMP_MSG_BANDWIDTH: // RTMP消息类型ID=6, 置对等端带宽.客户端或服务端发送本消息更新对等端的输出带宽.
		if body.Len() < 4 {
			return errors.New("chunk.Body < 4")
		}
		m := &SetPeerBandwidthMessage{
			AcknowledgementWindowsize: body.ReadUint32(),
		}
		if body.Len() > 0 {
			m.LimitType = body[0]
		}
		chunk.MsgData = m
	case RTMP_MSG_EDGE: // RTMP消息类型ID=7, 用于边缘服务与源服务器.
	case RTMP_MSG_AUDIO: // RTMP消息类型ID=8, 音频数据.客户端或服务端发送本消息用于发送音频数据.
	case RTMP_MSG_VIDEO: // RTMP消息类型ID=9, 视频数据.客户端或服务端发送本消息用于发送视频数据.
	case RTMP_MSG_AMF3_METADATA: // RTMP消息类型ID=15, 数据消息.用AMF3编码.
	case RTMP_MSG_AMF3_SHARED: // RTMP消息类型ID=16, 共享对象消息.用AMF3编码.
	case RTMP_MSG_AMF3_COMMAND: // RTMP消息类型ID=17, 命令消息.用AMF3编码.
		decodeCommandAMF0(chunk, body[1:])
	case RTMP_MSG_AMF0_METADATA: // RTMP消息类型ID=18, 数据消息.用AMF0编码.
	case RTMP_MSG_AMF0_SHARED: // RTMP消息类型ID=19, 共享对象消息.用AMF0编码.
	case RTMP_MSG_AMF0_COMMAND: // RTMP消息类型ID=20, 命令消息.用AMF0编码.
		decodeCommandAMF0(chunk, body) // 解析具体的命令消息
	case RTMP_MSG_AGGREGATE:
	default:
	}
	return nil
}

// 03 00 00 00 00 01 02 14 00 00 00 00 02 00 07 63 6F 6E 6E 65 63 74 00 3F F0 00 00 00 00 00 00 08
//
// 这个函数解析的是从02(第13个字节)开始,前面12个字节是Header,后面的是Payload,即解析Payload.
//
// 解析用AMF0编码的命令消息.(Payload)
// 第一个字节(Byte)为此数据的类型.例如:string,int,bool...

// string就是字符类型,一个byte的amf类型,两个bytes的字符长度,和N个bytes的数据.
// 比如: 02 00 02 33 22,第一个byte为amf类型,其后两个bytes为长度,注意这里的00 02是大端模式,33 22是字符数据

// umber类型其实就是double,占8bytes.
// 比如: 00 00 00 00 00 00 00 00,第一个byte为amf类型,其后8bytes为double值0.0

// boolean就是布尔类型,占用1byte.
// 比如:01 00,第一个byte为amf类型,其后1byte是值,false.

// object类型要复杂点.
// 第一个byte是03表示object,其后跟的是N个(key+value).最后以00 00 09表示object结束
func decodeCommandAMF0(chunk *Chunk, body []byte) {
	amf := AMF{body}             // rtmp_amf.go, amf 是 bytes类型, 将rtmp body(payload)放到bytes.Buffer(amf)中去.
	cmd := amf.ReadShortString() // rtmp_amf.go, 将payload的bytes类型转换成string类型.
	cmdMsg := CommandMessage{
		cmd,
		uint64(amf.ReadNumber()),
	}
	switch cmd {
	case "connect", "call":
		chunk.MsgData = &CallMessage{
			cmdMsg,
			amf.ReadObject(),
			amf.ReadObject(),
		}
	case "createStream":
		amf.Unmarshal()
		chunk.MsgData = &cmdMsg
	case "play":
		amf.Unmarshal()
		m := &PlayMessage{
			CURDStreamMessage{
				cmdMsg,
				chunk.MessageStreamID,
			},
			amf.ReadShortString(),
			float64(-2),
			float64(-1),
			true,
		}
		for i := 0; i < 3; i++ {
			if v, _ := amf.Unmarshal(); v != nil {
				switch vv := v.(type) {
				case float64:
					if i == 0 {
						m.Start = vv
					} else {
						m.Duration = vv
					}
				case bool:
					m.Reset = vv
					i = 2
				}
			} else {
				break
			}
		}
		chunk.MsgData = m
	case "play2":
		amf.Unmarshal()
		chunk.MsgData = &Play2Message{
			cmdMsg,
			uint64(amf.ReadNumber()),
			amf.ReadShortString(),
			amf.ReadShortString(),
			uint64(amf.ReadNumber()),
			amf.ReadShortString(),
		}
	case "publish":
		amf.Unmarshal()
		chunk.MsgData = &PublishMessage{
			CURDStreamMessage{
				cmdMsg,
				chunk.MessageStreamID,
			},
			amf.ReadShortString(),
			amf.ReadShortString(),
		}
	case "pause":
		amf.Unmarshal()
		chunk.MsgData = &PauseMessage{
			cmdMsg,
			amf.ReadBool(),
			uint64(amf.ReadNumber()),
		}
	case "seek":
		amf.Unmarshal()
		chunk.MsgData = &SeekMessage{
			cmdMsg,
			uint64(amf.ReadNumber()),
		}
	case "deleteStream", "closeStream":
		amf.Unmarshal()
		chunk.MsgData = &CURDStreamMessage{
			cmdMsg,
			uint32(amf.ReadNumber()),
		}
	case "releaseStream":
		amf.Unmarshal()
		chunk.MsgData = &ReleaseStreamMessage{
			cmdMsg,
			amf.ReadShortString(),
		}
	case "receiveAudio", "receiveVideo":
		amf.Unmarshal()
		chunk.MsgData = &ReceiveAVMessage{
			cmdMsg,
			amf.ReadBool(),
		}
	case Response_Result, Response_Error, Response_OnStatus:
		if cmdMsg.TransactionId == 2 {
			chunk.MsgData = &ResponseCreateStreamMessage{
				cmdMsg, amf.ReadObject(), uint32(amf.ReadNumber()),
			}
			return
		}
		response := &ResponseMessage{
			cmdMsg,
			amf.ReadObject(),
			amf.ReadObject(), "",
		}
		if response.Infomation == nil && response.Properties != nil {
			response.Infomation = response.Properties
		}
		// codef := zap.String("code", response.Infomation["code"].(string))
		switch response.Infomation["level"] {
		case Level_Status:
			// RTMPPlugin.Info("_result :", codef)
		case Level_Warning:
			// RTMPPlugin.Warn("_result :", codef)
		case Level_Error:
			// RTMPPlugin.Error("_result :", codef)
		}
		if strings.HasPrefix(response.Infomation["code"].(string), "NetStream.Publish") {
			chunk.MsgData = &ResponsePublishMessage{
				cmdMsg,
				response.Properties,
				response.Infomation,
				chunk.MessageStreamID,
			}
		} else if strings.HasPrefix(response.Infomation["code"].(string), "NetStream.Play") {
			chunk.MsgData = &ResponsePlayMessage{
				cmdMsg,
				response.Infomation,
				chunk.MessageStreamID,
			}
		} else {
			chunk.MsgData = response
		}
	case "FCPublish", "FCUnpublish":
		fallthrough
	default:
		chunk.MsgData = &struct{ CommandMessage }{cmdMsg}
		// RTMPPlugin.Info("decode command amf0 ", zap.String("cmd", cmd))
	}
}

/* Command Message */
type CommandMessage struct {
	CommandName   string // 命令名. 字符串. 命令名.设置为"connect"
	TransactionId uint64 // 传输ID. 数字. 总是设为1
}
type Commander interface {
	GetCommand() *CommandMessage
}

func (cmd *CommandMessage) GetCommand() *CommandMessage {
	return cmd
}

func (msg *CommandMessage) Encode(buf IAMF) {
	buf.Marshals(msg.CommandName, msg.TransactionId, nil)
}

// Protocol control message 1.
// Set Chunk Size, is used to notify the peer of a new maximum chunk size

// chunk size (31 bits): This field holds the new maximum chunk size,in bytes, which will be used for all of the sender’s subsequent chunks until further notice
type Uint32Message uint32

func (msg Uint32Message) Encode(buf IAMF) {
	binary.BigEndian.PutUint32(buf.Malloc(4), uint32(msg))
}

// Protocol control message 4, User Control Messages.
// User Control messages SHOULD use message stream ID 0 (known as the control stream) and, when sent over RTMP Chunk Stream,
// be sent on chunk stream ID 2. User Control messages are effective at the point they are received in the stream; their timestamps are ignored.

// Event Type (16 bits) : The first 2 bytes of the message data are used to identify the Event type. Event type is followed by Event data.
// Event Data
type UserControlMessage struct {
	EventType uint16
	EventData []byte
}

// Protocol control message 6, Set Peer Bandwidth Message.
// The client or the server sends this message to limit the output bandwidth of its peer.

// AcknowledgementWindowsize (4 bytes)
// LimitType : The Limit Type is one of the following values: 0 - Hard, 1 - Soft, 2- Dynamic.
type SetPeerBandwidthMessage struct {
	AcknowledgementWindowsize uint32 // 4 bytes
	LimitType                 byte
}

func (msg *SetPeerBandwidthMessage) Encode(buf IAMF) {
	buf.WriteUint32(msg.AcknowledgementWindowsize)
	buf.WriteByte(msg.LimitType)
}

// Message 15, 18. Data Message. The client or the server sends this message to send Metadata or any
// user data to the peer. Metadata includes details about the data(audio, video etc.) like creation time, duration,
// theme and so on. These messages have been assigned message type value of 18 for AMF0 and message type value of 15 for AMF3
type MetadataMessage struct {
	Proterties map[string]interface{} `json:",omitempty"`
}

// Object 可选值:
// App 				客户端要连接到的服务应用名 												Testapp
// Flashver			Flash播放器版本.和应用文档中getversion()函数返回的字符串相同.			FMSc/1.0
// SwfUrl			发起连接的swf文件的url													file://C:/ FlvPlayer.swf
// TcUrl			服务url.有下列的格式.protocol://servername:port/appName/appInstance		rtmp://localhost::1935/testapp/instance1
// fpad				是否使用代理															true or false
// audioCodecs		指示客户端支持的音频编解码器											SUPPORT_SND_MP3
// videoCodecs		指示支持的视频编解码器													SUPPORT_VID_SORENSON
// pageUrl			SWF文件被加载的页面的Url												http:// somehost/sample.html
// objectEncoding	AMF编码方法																AMF编码方法	kAMF3

// Call Message.
// The call method of the NetConnection object runs remote procedure calls (RPC) at the receiving end.
// The called RPC name is passed as a parameter to the call command.
type CallMessage struct {
	CommandMessage
	Object   map[string]any `json:",omitempty"`
	Optional map[string]any `json:",omitempty"`
}

func (msg *CallMessage) Encode(buf IAMF) {
	buf.Marshals(msg.CommandName, msg.TransactionId, msg.Object)
	if msg.Optional != nil {
		buf.Marshals(msg.Optional)
	}
}

// func (msg *CallMessage) Encode3() []byte {
// 	var amf util.AMF
// 	amf.WriteByte(0)
// 	return amf.Marshals(msg.CommandName, msg.TransactionId, msg.Object, msg.Optional)
// }

// Create Stream Message.
// The client sends this command to the server to create a logical channel for message communication The publishing of audio,
// video, and metadata is carried out over stream channel created using the createStream command.

/*
func (msg *CreateStreamMessage) Encode3() {
	msg.Encode0()

	buf := new(bytes.Buffer)
	buf.WriteByte(0)
	buf.Write(msg.RtmpBody)
	msg.RtmpBody = buf.Bytes()
}*/

// The following commands can be sent on the NetStream by the client to the server:

// Play
// Play2
// DeleteStream
// CloseStream
// ReceiveAudio
// ReceiveVideo
// Publish
// Seek
// Pause
// Release(37)
// FCPublish

// Play Message
// The client sends this command to the server to play a stream. A playlist can also be created using this command multiple times
type PlayMessage struct {
	CURDStreamMessage
	StreamName string
	Start      float64
	Duration   float64
	Reset      bool
}

// 命令名 -> 命令名,设置为”play”
// 传输ID -> 0
// 命令对象
// 流名字 -> 要播放流的名字
// start -> 可选的参数,以秒为单位定义开始时间.默认值为 -2,表示用户首先尝试播放流名字段中定义的直播流.
// Duration -> 可选的参数,以秒为单位定义了回放的持续时间.默认值为 -1.-1 值意味着一个直播流会一直播放直到它不再可用或者一个录制流一直播放直到结束
// Reset -> 可选的布尔值或者数字定义了是否对以前的播放列表进行 flush

func (msg *PlayMessage) Encode(buf IAMF) {
	// if msg.Start > 0 {
	// 	amf.writeNumber(msg.Start)
	// }

	// if msg.Duration > 0 {
	// 	amf.writeNumber(msg.Duration)
	// }

	// amf.writeBool(msg.Reset)
	buf.Marshals(msg.CommandName, msg.TransactionId, nil, msg.StreamName, -2000)
}

/*
func (msg *PlayMessage) Encode3() {
}*/

// Play2 Message
// Unlike the play command, play2 can switch to a different bit rate stream without changing the timeline of the content played. The
// server maintains multiple files for all supported bitrates that the client can request in play2.
type Play2Message struct {
	CommandMessage
	StartTime     uint64
	OldStreamName string
	StreamName    string
	Duration      uint64
	Transition    string
}

func (msg *Play2Message) Encode0() {
}

// Delete Stream Message
// NetStream sends the deleteStream command when the NetStream object is getting destroyed
type CURDStreamMessage struct {
	CommandMessage
	StreamId uint32
}

func (msg *CURDStreamMessage) GetStreamID() uint32 {
	return msg.StreamId
}

func (msg *CURDStreamMessage) Encode0() {
}

type ReleaseStreamMessage struct {
	CommandMessage
	StreamName string
}

func (msg *ReleaseStreamMessage) Encode0() {
}

// Receive Audio Message
// NetStream sends the receiveAudio message to inform the server whether to send or not to send the audio to the client
type ReceiveAVMessage struct {
	CommandMessage
	BoolFlag bool
}

func (msg *ReceiveAVMessage) Encode0() {
}

// Publish Message
// The client sends the publish command to publish a named stream to the server. Using this name,
// any client can play this stream and receive the published audio, video, and data messages
type PublishMessage struct {
	CURDStreamMessage
	PublishingName string
	PublishingType string
}

// 命令名 -> 命令名,设置为”publish”
// 传输ID -> 0
// 命令对象
// 发布名 -> 流发布的名字
// 发布类型 -> 设置为”live”，”record”或”append”.

// “record”:流被发布,并且数据被录制到一个新的文件,文件被存储到服务端的服务应用的目录的一个子目录下.如果文件已经存在则重写文件.
// “append”:流被发布并且附加到一个文件之后.如果没有发现文件则创建一个文件.
// “live”:发布直播数据而不录制到文件

func (msg *PublishMessage) Encode(buf IAMF) {
	buf.Marshals(msg.CommandName, msg.TransactionId, nil, msg.PublishingName, msg.PublishingType)
}

// Seek Message
// The client sends the seek command to seek the offset (in milliseconds) within a media file or playlist.
type SeekMessage struct {
	CommandMessage
	Milliseconds uint64
}

func (msg *SeekMessage) Encode0() {
}

// Pause Message
// The client sends the pause command to tell the server to pause or start playing.
type PauseMessage struct {
	CommandMessage
	Pause        bool
	Milliseconds uint64
}

// 命令名 -> 命令名,设置为”pause”
// 传输ID -> 0
// 命令对象 -> null
// Pause/Unpause Flag -> true 或者 false，来指示暂停或者重新播放
// milliSeconds -> 流暂停或者重新开始所在的毫秒数.这个是客户端暂停的当前流时间.当回放已恢复时,服务器端值发送带有比这个值大的 timestamp 消息

func (msg *PauseMessage) Encode0() {
}

//
// Response Message. Server -> Response -> Client
//

// Response Connect Message
type ResponseConnectMessage struct {
	CommandMessage
	Properties map[string]any `json:",omitempty"`
	Infomation map[string]any `json:",omitempty"`
}

func (msg *ResponseConnectMessage) Encode(buf IAMF) {
	buf.Marshals(msg.CommandName, msg.TransactionId, msg.Properties, msg.Infomation)
}

/*
func (msg *ResponseConnectMessage) Encode3() {
}*/

// Response Call Message
type ResponseCallMessage struct {
	CommandMessage
	Object   map[string]any
	Response map[string]any
}

// func (msg *ResponseCallMessage) Encode0() []byte {
// 	return codec.MarshalAMFs(msg.CommandName, msg.TransactionId, msg.Object, msg.Response)
// }

// Response Create Stream Message
type ResponseCreateStreamMessage struct {
	CommandMessage
	Object   any `json:",omitempty"`
	StreamId uint32
}

func (msg *ResponseCreateStreamMessage) Encode(buf IAMF) {
	buf.Marshals(msg.CommandName, msg.TransactionId, nil, msg.StreamId)
}

/*
func (msg *ResponseCreateStreamMessage) Encode3() {
}*/

// func (msg *ResponseCreateStreamMessage) Decode0(chunk *Chunk) {
// 	amf := util.AMF{chunk.Body}
// 	msg.CommandName = amf.ReadShortString()
// 	msg.TransactionId = uint64(amf.ReadNumber())
// 	amf.Unmarshal()
// 	msg.StreamId = uint32(amf.ReadNumber())
// }

// func (msg *ResponseCreateStreamMessage) Decode3(chunk *Chunk) {
// 	chunk.Body = chunk.Body[1:]
// 	msg.Decode0(chunk)
// }

// Response Play Message
type ResponsePlayMessage struct {
	CommandMessage
	Infomation map[string]any `json:",omitempty"`
	StreamID   uint32
}

func (msg *ResponsePlayMessage) GetStreamID() uint32 {
	return msg.StreamID
}
func (msg *ResponsePlayMessage) Encode(buf IAMF) {
	buf.Marshals(msg.CommandName, msg.TransactionId, nil, msg.Infomation)
}

/*
func (msg *ResponsePlayMessage) Encode3() {
}*/

// func (msg *ResponsePlayMessage) Decode0(chunk *Chunk) {
// 	amf := util.AMF{chunk.Body}
// 	msg.CommandName = amf.ReadShortString()
// 	msg.TransactionId = uint64(amf.ReadNumber())
// 	msg.Infomation = amf.ReadObject()
// }

// func (msg *ResponsePlayMessage) Decode3(chunk *Chunk) {
// 	chunk.Body = chunk.Body[1:]
// 	msg.Decode0(chunk)
// }

// Response Publish Message
type ResponsePublishMessage struct {
	CommandMessage
	Properties map[string]any `json:",omitempty"`
	Infomation map[string]any `json:",omitempty"`
	StreamID   uint32
}

func (msg *ResponsePublishMessage) GetStreamID() uint32 {
	return msg.StreamID
}

// 命令名 -> 命令名,设置为"OnStatus"
// 传输ID -> 0
// 属性 -> null
// 信息 -> level, code, description

func (msg *ResponsePublishMessage) Encode(buf IAMF) {
	buf.Marshals(msg.CommandName, msg.TransactionId, msg.Properties, msg.Infomation)
}

/*
func (msg *ResponsePublishMessage) Encode3() {
}*/

// Response Seek Message
type ResponseSeekMessage struct {
	CommandMessage
	Description string
}

func (msg *ResponseSeekMessage) Encode0() {
}

//func (msg *ResponseSeekMessage) Encode3() {
//}

// Response Pause Message
type ResponsePauseMessage struct {
	CommandMessage
	Description string
}

// 命令名 -> 命令名,设置为"OnStatus"
// 传输ID -> 0
// 描述

func (msg *ResponsePauseMessage) Encode0() {
}

//func (msg *ResponsePauseMessage) Encode3() {
//}

// Response Message
type ResponseMessage struct {
	CommandMessage
	Properties  map[string]any `json:",omitempty"`
	Infomation  map[string]any `json:",omitempty"`
	Description string
}

// User Control Message 4.
// The client or the server sends this message to notify the peer about the user control events.
// For information about the message format, see Section 6.2.

// The following user control event types are supported:

// Stream Begin (=0)
// The server sends this event to notify the client that a stream has become functional and can be
// used for communication. By default, this event is sent on ID 0 after the application connect
// command is successfully received from the client. The event data is 4-byte and represents
// the stream ID of the stream that became functional.
type StreamIDMessage struct {
	UserControlMessage
	StreamID uint32
}

func (msg *StreamIDMessage) Encode(buffer IAMF) {
	buffer.WriteUint16(msg.EventType)
	msg.EventData = buffer.Malloc(4)
	binary.BigEndian.PutUint32(msg.EventData, msg.StreamID)
}

// SetBuffer Length (=3)
// The client sends this event to inform the server of the buffer size (in milliseconds) that is
// used to buffer any data coming over a stream. This event is sent before the server starts |
// processing the stream. The first 4 bytes of the event data represent the stream ID and the next |
// 4 bytes represent the buffer length, in  milliseconds.
type SetBufferMessage struct {
	StreamIDMessage
	Millisecond uint32
}

func (msg *SetBufferMessage) Encode(buf IAMF) {
	buf.WriteUint16(msg.EventType)
	msg.EventData = buf.Malloc(8)
	binary.BigEndian.PutUint32(msg.EventData, msg.StreamID)
	binary.BigEndian.PutUint32(msg.EventData[4:], msg.Millisecond)
}

// PingRequest (=6)
// The server sends this event to test whether the client is reachable. Event data is a 4-byte
// timestamp, representing the local server time when the server dispatched the command.
// The client responds with PingResponse on receiving MsgPingRequest.
type PingRequestMessage struct {
	UserControlMessage
	Timestamp uint32
}

func (msg *PingRequestMessage) Encode(buf IAMF) {
	buf.WriteUint16(msg.EventType)
	msg.EventData = buf.Malloc(4)
	binary.BigEndian.PutUint32(msg.EventData, msg.Timestamp)
}

func (msg *UserControlMessage) Encode(buf IAMF) {
	buf.WriteUint16(msg.EventType)
}
