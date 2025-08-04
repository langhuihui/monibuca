package rtmp

import (
	"encoding/binary"
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
	binary.BigEndian.PutUint32(buf.GetBuffer().Malloc(4), uint32(msg))
}

// Protocol control message 4, User Control Messages.
// User Control messages SHOULD use message stream ID 0 (known as the control stream) and, when sent over RTMP Chunk Stream,
// be sent on chunk stream ID 2. User Control messages are effective at the point they are received in the stream; their timestamps are ignored.

// Event Type (16 bits) : The first 2 bytes of the message data are used to identify the Event type. Event type is followed by Event data.
// Event Data
type UserControlMessage uint16

// Protocol control message 6, Set Peer Bandwidth Message.
// The client or the server sends this message to limit the output bandwidth of its peer.

// AcknowledgementWindowsize (4 bytes)
// LimitType : The Limit Type is one of the following values: 0 - Hard, 1 - Soft, 2- Dynamic.
type SetPeerBandwidthMessage struct {
	AcknowledgementWindowsize uint32 // 4 bytes
	LimitType                 byte
}

func (msg *SetPeerBandwidthMessage) Encode(buf IAMF) {
	buf.GetBuffer().WriteUint32(msg.AcknowledgementWindowsize)
	buf.GetBuffer().WriteByte(msg.LimitType)
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

func (msg StreamIDMessage) Encode(buf IAMF) {
	msg.UserControlMessage.Encode(buf)
	binary.BigEndian.PutUint32(buf.GetBuffer().Malloc(4), msg.StreamID)
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
	msg.UserControlMessage.Encode(buf)
	buffer := buf.GetBuffer().Malloc(8)
	binary.BigEndian.PutUint32(buffer, msg.StreamID)
	binary.BigEndian.PutUint32(buffer[4:], msg.Millisecond)
}

// PingRequest (=6)
// The server sends this event to test whether the client is reachable. Event data is a 4-byte
// timestamp, representing the local server time when the server dispatched the command.
// The client responds with PingResponse on receiving MsgPingRequest.
type PingRequestMessage struct {
	UserControlMessage
	Timestamp uint32
}

func (msg PingRequestMessage) Encode(buf IAMF) {
	msg.UserControlMessage.Encode(buf)
	binary.BigEndian.PutUint32(buf.GetBuffer().Malloc(4), msg.Timestamp)
}

func (msg UserControlMessage) Encode(buf IAMF) {
	buf.GetBuffer().WriteUint16(uint16(msg))
}
