package rtmp

import (
	"errors"
	"net"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
)

const (
	SEND_CHUNK_SIZE_MESSAGE         = "Send Chunk Size Message"
	SEND_ACK_MESSAGE                = "Send Acknowledgement Message"
	SEND_ACK_WINDOW_SIZE_MESSAGE    = "Send Window Acknowledgement Size Message"
	SEND_SET_PEER_BANDWIDTH_MESSAGE = "Send Set Peer Bandwidth Message"

	SEND_STREAM_BEGIN_MESSAGE       = "Send Stream Begin Message"
	SEND_SET_BUFFER_LENGTH_MESSAGE  = "Send Set Buffer Lengh Message"
	SEND_STREAM_IS_RECORDED_MESSAGE = "Send Stream Is Recorded Message"

	SEND_PING_REQUEST_MESSAGE  = "Send Ping Request Message"
	SEND_PING_RESPONSE_MESSAGE = "Send Ping Response Message"

	SEND_CONNECT_MESSAGE          = "Send Connect Message"
	SEND_CONNECT_RESPONSE_MESSAGE = "Send Connect Response Message"

	SEND_CREATE_STREAM_MESSAGE = "Send Create Stream Message"

	SEND_PLAY_MESSAGE          = "Send Play Message"
	SEND_PLAY_RESPONSE_MESSAGE = "Send Play Response Message"

	SEND_PUBLISH_RESPONSE_MESSAGE = "Send Publish Response Message"
	SEND_PUBLISH_START_MESSAGE    = "Send Publish Start Message"

	SEND_UNPUBLISH_RESPONSE_MESSAGE = "Send Unpublish Response Message"

	SEND_AUDIO_MESSAGE      = "Send Audio Message"
	SEND_FULL_AUDIO_MESSAGE = "Send Full Audio Message"
	SEND_VIDEO_MESSAGE      = "Send Video Message"
	SEND_FULL_VDIEO_MESSAGE = "Send Full Video Message"
)

type Writers = map[uint32]*struct {
	m7s.PublishWriter[*AudioFrame, *VideoFrame]
	*m7s.Publisher
}

type NetConnection struct {
	task.Job
	*util.BufReader
	net.Conn
	bandwidth                     uint32
	readSeqNum, writeSeqNum       uint32 // 当前读的字节
	totalRead, totalWrite         uint32 // 总共读写了多少字节
	ReadChunkSize, WriteChunkSize int
	incommingChunks               map[uint32]*Chunk
	ObjectEncoding                float64
	AppName                       string
	tmpBuf                        AMF //用来接收/发送小数据，复用内存
	chunkHeaderBuf                util.Buffer
	mediaDataPool                 *util.ScalableMemoryAllocator
	writing                       atomic.Bool // false 可写，true 不可写
	Writers                       Writers
	sendBuffers                   net.Buffers
}

func NewNetConnection(conn net.Conn) (ret *NetConnection) {
	ret = &NetConnection{}
	ret.Init(conn)
	return
}

func (nc *NetConnection) Init(conn net.Conn) {
	nc.Conn = conn
	nc.BufReader = util.NewBufReader(conn)
	nc.bandwidth = RTMP_MAX_CHUNK_SIZE << 3
	nc.ReadChunkSize = RTMP_DEFAULT_CHUNK_SIZE
	nc.WriteChunkSize = RTMP_DEFAULT_CHUNK_SIZE
	nc.incommingChunks = make(map[uint32]*Chunk)
	nc.tmpBuf = make(AMF, 4)
	nc.chunkHeaderBuf = make(util.Buffer, 0, 20)
	nc.mediaDataPool = util.NewScalableMemoryAllocator(1 << util.MinPowerOf2)
	nc.sendBuffers = make(net.Buffers, 0, 50)
	nc.Writers = make(Writers)
}

func (nc *NetConnection) Dispose() {
	nc.Conn.Close()
	nc.BufReader.Recycle()
	nc.mediaDataPool.Recycle()
}

func (nc *NetConnection) SendStreamID(eventType uint16, streamID uint32) (err error) {
	return nc.SendMessage(RTMP_MSG_USER_CONTROL, StreamIDMessage{UserControlMessage(eventType), streamID})
}

func (nc *NetConnection) SendUserControl(eventType uint16) error {
	return nc.SendMessage(RTMP_MSG_USER_CONTROL, UserControlMessage(eventType))
}

func (nc *NetConnection) SendPingRequest() error {
	return nc.SendMessage(RTMP_MSG_USER_CONTROL, PingRequestMessage{UserControlMessage(RTMP_USER_PING_REQUEST), uint32(time.Now().Unix())})
}

func (nc *NetConnection) ResponseCreateStream(tid uint64, streamID uint32) error {
	m := &ResponseCreateStreamMessage{}
	m.CommandName = Response_Result
	m.TransactionId = tid
	m.StreamId = streamID
	return nc.SendMessage(RTMP_MSG_AMF0_COMMAND, m)
}

// func (conn *NetConnection) SendCommand(message string, args any) error {
// 	switch message {
// 	// case SEND_SET_BUFFER_LENGTH_MESSAGE:
// 	// 	if args != nil {
// 	// 		return errors.New(SEND_SET_BUFFER_LENGTH_MESSAGE + ", The parameter is nil")
// 	// 	}
// 	// 	m := new(SetBufferMessage)
// 	// 	m.EventType = RTMP_USER_SET_BUFFLEN
// 	// 	m.Millisecond = 100
// 	// 	m.StreamID = conn.streamID
// 	// 	return conn.writeMessage(RTMP_MSG_USER_CONTROL, m)
// 	}
// 	return errors.New("send message no exist")
// }

func (nc *NetConnection) readChunk() (msg *Chunk, err error) {
	nc.SetReadDeadline(time.Now().Add(time.Second * 5)) // 设置读取超时时间为5秒
	head, err := nc.ReadByte()
	if err != nil {
		return nil, err
	}
	nc.readSeqNum++
	ChunkStreamID := uint32(head & 0x3f) // 0011 1111
	ChunkType := head >> 6               // 1100 0000
	// 如果块流ID为0,1的话,就需要计算.
	ChunkStreamID, err = nc.readChunkStreamID(ChunkStreamID)
	if err != nil {
		return nil, errors.New("get chunk stream id error :" + err.Error())
	}
	//println("ChunkStreamID:", ChunkStreamID, "ChunkType:", ChunkType)
	chunk, ok := nc.incommingChunks[ChunkStreamID]

	if ChunkType != 3 && ok && chunk.bufLen > 0 {
		// 如果块类型不为3,那么这个rtmp的body应该为空.
		return nil, errors.New("incompleteRtmpBody error")
	}
	if !ok {
		chunk = &Chunk{}
		nc.incommingChunks[ChunkStreamID] = chunk
	}

	if err = nc.readChunkType(&chunk.ChunkHeader, ChunkType); err != nil {
		return nil, errors.New("get chunk type error :" + err.Error())
	}

	msgLen := int(chunk.MessageLength)
	if msgLen == 0 {
		return nil, nil
	}
	var bufSize int
	if unRead := msgLen - chunk.bufLen; unRead < nc.ReadChunkSize {
		bufSize = unRead
	} else {
		bufSize = nc.ReadChunkSize
	}

	nc.readSeqNum += uint32(bufSize)
	if chunk.bufLen == 0 {
		switch chunk.MessageTypeID {
		case RTMP_MSG_AUDIO:
			if writer, ok := nc.Writers[chunk.MessageStreamID]; ok {
				if writer.PubAudio {
					if writer.PublishAudioWriter == nil {
						writer.PublishAudioWriter = m7s.NewPublishAudioWriter[*AudioFrame](writer.Publisher, nc.mediaDataPool)
					}
					chunk.buf = writer.AudioFrame.NextN(msgLen)
					break
				}
			}
			chunk.buf = nc.mediaDataPool.Malloc(msgLen)
		case RTMP_MSG_VIDEO:
			if writer, ok := nc.Writers[chunk.MessageStreamID]; ok {
				if writer.PubVideo {
					if writer.PublishVideoWriter == nil {
						writer.PublishVideoWriter = m7s.NewPublishVideoWriter[*VideoFrame](writer.Publisher, nc.mediaDataPool)
					}
					chunk.buf = writer.VideoFrame.NextN(msgLen)
					break
				}
			}
			chunk.buf = nc.mediaDataPool.Malloc(msgLen)
		default:
			chunk.buf = nc.mediaDataPool.Malloc(msgLen)
		}
		var delta = chunk.Timestamp
		if delta > 0xffffff {
			delta -= 0xffffff
		}
		if ChunkType == 0 {
			chunk.ExtendTimestamp = chunk.Timestamp
		} else {
			chunk.ExtendTimestamp += delta
		}
	}
	if chunk.buf == nil {
		nc.Skip(bufSize)
	} else if err = nc.ReadNto(bufSize, chunk.buf[chunk.bufLen:]); err != nil {
		return nil, err
	}
	chunk.bufLen += bufSize
	if chunk.bufLen == msgLen {
		switch chunk.MessageTypeID {
		case RTMP_MSG_AUDIO:
			if writer, ok := nc.Writers[chunk.MessageStreamID]; ok && writer.Publisher.PubAudio {
				if writer.PubAudio {
					writer.AudioFrame.SetTS32(chunk.ChunkHeader.ExtendTimestamp)
					err = writer.NextAudio()
					break
				}
			}
			nc.mediaDataPool.Free(chunk.buf)
		case RTMP_MSG_VIDEO:
			if writer, ok := nc.Writers[chunk.MessageStreamID]; ok && writer.Publisher.PubVideo {
				if writer.PubVideo {
					writer.VideoFrame.SetTS32(chunk.ChunkHeader.ExtendTimestamp)
					err = writer.NextVideo()
					break
				}
			}
			nc.mediaDataPool.Free(chunk.buf)
		default:
			nc.mediaDataPool.Free(chunk.buf)
		}
		chunk.bufLen = 0
		return chunk, err
	}
	return
}

func (nc *NetConnection) readChunkStreamID(csid uint32) (chunkStreamID uint32, err error) {
	chunkStreamID = csid

	switch csid {
	case 0:
		{
			u8, err := nc.ReadByte()
			nc.readSeqNum++
			if err != nil {
				return 0, err
			}

			chunkStreamID = 64 + uint32(u8)
		}
	case 1:
		{
			u16_0, err1 := nc.ReadByte()
			if err1 != nil {
				return 0, err1
			}
			u16_1, err1 := nc.ReadByte()
			if err1 != nil {
				return 0, err1
			}
			nc.readSeqNum += 2
			chunkStreamID = 64 + uint32(u16_0) + (uint32(u16_1) << 8)
		}
	}

	return chunkStreamID, nil
}

func (nc *NetConnection) readChunkType(h *ChunkHeader, chunkType byte) (err error) {
	if chunkType != 3 {
		// Timestamp 3 bytes
		if h.Timestamp, err = nc.ReadBE32(3); err != nil {
			return err
		}

		if chunkType != 2 {
			if h.MessageLength, err = nc.ReadBE32(3); err != nil {
				return err
			}
			// Message Type ID 1 bytes
			if h.MessageTypeID, err = nc.ReadByte(); err != nil {
				return err
			}
			nc.readSeqNum++
			if chunkType == 0 {
				// Message Stream ID 4bytes
				if h.MessageStreamID, err = nc.ReadLE32(4); err != nil { // 读取Message Stream ID
					return err
				}
			}
		}
	}
	// ExtendTimestamp 4 bytes
	if h.Timestamp >= 0xffffff { // 对于type 0的chunk,绝对时间戳在这里表示,如果时间戳值大于等于0xffffff(16777215),该值必须是0xffffff,且时间戳扩展字段必须发送,其他情况没有要求
		if h.Timestamp, err = nc.ReadBE32(4); err != nil {
			return err
		}
	}
	return nil
}

func (nc *NetConnection) RecvMessage() (cmd Commander, err error) {
	if nc.readSeqNum >= nc.bandwidth {
		nc.totalRead += nc.readSeqNum
		nc.readSeqNum = 0
		err = nc.SendMessage(RTMP_MSG_ACK, Uint32Message(nc.totalRead))
	}
	var msg *Chunk
	for err == nil {
		if msg, err = nc.readChunk(); msg != nil && err == nil {
			// 统一的消息解析和处理逻辑
			var body util.Buffer
			if msg.buf != nil {
				body = msg.buf
			}

			switch msg.MessageTypeID {
			case RTMP_MSG_CHUNK_SIZE:
				if body.Len() < 4 {
					err = errors.New("chunk.Body < 4")
					continue
				}
				nc.ReadChunkSize = int(body.ReadUint32())
				nc.Info("msg read chunk size", "readChunkSize", nc.ReadChunkSize)
			case RTMP_MSG_ABORT:
				if body.Len() < 4 {
					err = errors.New("chunk.Body < 4")
					continue
				}
				delete(nc.incommingChunks, body.ReadUint32())
			case RTMP_MSG_ACK:
				// if body.Len() >= 4 {
				// 	msg.MsgData = Uint32Message(body.ReadUint32())
				// }
			case RTMP_MSG_ACK_SIZE:
				if body.Len() < 4 {
					err = errors.New("chunk.Body < 4")
					continue
				}
				nc.bandwidth = body.ReadUint32()
			case RTMP_MSG_USER_CONTROL: // RTMP消息类型ID=4, 用户控制消息.客户端或服务端发送本消息通知对方用户的控制事件.
				if body.Len() < 2 {
					err = errors.New("UserControlMessage.Body < 2")
					continue
				}
				switch body.ReadUint16() {
				case RTMP_USER_STREAM_BEGIN: // 服务端向客户端发送本事件通知对方一个流开始起作用可以用于通讯.在默认情况下,服务端在成功地从客户端接收连接命令之后发送本事件,事件ID为0.事件数据是表示开始起作用的流的ID.
					// m := &StreamIDMessage{
					// 	UserControlMessage: base,
					// 	StreamID:           0,
					// }
					// if len(base.EventData) >= 4 {
					// 	//服务端在成功地从客户端接收连接命令之后发送本事件,事件ID为0.事件数据是表示开始起作用的流的ID.
					// 	m.StreamID = body.ReadUint32()
					// }
					// msg.MsgData = m
				case RTMP_USER_STREAM_EOF, RTMP_USER_STREAM_DRY, RTMP_USER_STREAM_IS_RECORDED: // 服务端向客户端发送本事件通知客户端,数据回放完成.果没有发行额外的命令,就不再发送数据.客户端丢弃从流中接收的消息.4字节的事件数据表示,回放结束的流的ID.
					// msg.MsgData = &StreamIDMessage{
					// 	UserControlMessage: base,
					// 	StreamID:           body.ReadUint32(),
					// }
				case RTMP_USER_SET_BUFFLEN: // 客户端向服务端发送本事件,告知对方自己存储一个流的数据的缓存的长度(毫秒单位).当服务端开始处理一个流得时候发送本事件.事件数据的头四个字节表示流ID,后4个字节表示缓存长度(毫秒单位).
					// msg.MsgData = &SetBufferMessage{
					// 	StreamIDMessage: StreamIDMessage{
					// 		UserControlMessage: base,
					// 		StreamID:           body.ReadUint32(),
					// 	},
					// 	Millisecond: body.ReadUint32(),
					// }
				case RTMP_USER_PING_REQUEST: // 服务端通过本事件测试客户端是否可达.事件数据是4个字节的事件戳.代表服务调用本命令的本地时间.客户端在接收到kMsgPingRequest之后返回kMsgPingResponse事件
					// msg.MsgData = &PingRequestMessage{
					// 	UserControlMessage: base,
					// 	Timestamp:          body.ReadUint32(),
					// }
					nc.SendUserControl(RTMP_USER_PING_RESPONSE)
				case RTMP_USER_PING_RESPONSE, RTMP_USER_EMPTY: // 客户端向服务端发送本消息响应ping请求.事件数据是接kMsgPingRequest请求的时间.
					// msg.MsgData = &base
				}
			case RTMP_MSG_BANDWIDTH: // RTMP消息类型ID=6, 置对等端带宽.客户端或服务端发送本消息更新对等端的输出带宽.
				if body.Len() < 4 {
					err = errors.New("chunk.Body < 4")
					continue
				}
				// m := &SetPeerBandwidthMessage{
				// 	AcknowledgementWindowsize: body.ReadUint32(),
				// }
				// if body.Len() > 0 {
				// 	m.LimitType = body[0]
				// }
				// msg.MsgData = m
				// 处理带宽消息
				nc.bandwidth = body.ReadUint32()
			case RTMP_MSG_EDGE: // RTMP消息类型ID=7, 用于边缘服务与源服务器.
				// 不需要特殊处理
			case RTMP_MSG_AMF3_METADATA: // RTMP消息类型ID=15, 数据消息.用AMF3编码.
			case RTMP_MSG_AMF3_SHARED: // RTMP消息类型ID=16, 共享对象消息.用AMF3编码.
			case RTMP_MSG_AMF3_COMMAND: // RTMP消息类型ID=17, 命令消息.用AMF3编码.
				nc.decodeCommandAMF0(msg, body[1:])
			case RTMP_MSG_AMF0_METADATA: // RTMP消息类型ID=18, 数据消息.用AMF0编码.
			case RTMP_MSG_AMF0_SHARED: // RTMP消息类型ID=19, 共享对象消息.用AMF0编码.
			case RTMP_MSG_AMF0_COMMAND: // RTMP消息类型ID=20, 命令消息.用AMF0编码.
				nc.decodeCommandAMF0(msg, body) // 解析具体的命令消息
				// 处理AMF0命令消息
				return msg.MsgData.(Commander), err
			case RTMP_MSG_AGGREGATE:
			default:
			}
		}
		if nc.IsStopped() {
			err = nc.StopReason()
		}
	}
	return
}
func (nc *NetConnection) SendMessage(t byte, msg RtmpMessage) (err error) {
	if nc == nil {
		return errors.New("connection is nil")
	}
	if nc.writeSeqNum > nc.bandwidth {
		nc.totalWrite += nc.writeSeqNum
		nc.writeSeqNum = 0
		err = nc.SendMessage(RTMP_MSG_ACK, Uint32Message(nc.totalWrite))
		err = nc.SendPingRequest()
	}
	for !nc.writing.CompareAndSwap(false, true) {
		runtime.Gosched()
	}
	defer nc.writing.Store(false)
	nc.tmpBuf.GetBuffer().Reset()
	if nc.ObjectEncoding == 0 {
		msg.Encode(&nc.tmpBuf)
	} else {
		amf3 := AMF3{AMF: nc.tmpBuf}
		msg.Encode(&amf3)
		nc.tmpBuf = amf3.AMF
	}
	head := newChunkHeader(t)
	head.MessageLength = uint32(len(nc.tmpBuf))
	if sid, ok := msg.(HaveStreamID); ok {
		head.MessageStreamID = sid.GetStreamID()
	}
	nc.SetWriteDeadline(time.Now().Add(time.Second * 5)) // 设置写入超时时间为5秒
	return nc.sendChunk(util.NewMemory(nc.tmpBuf), head, RTMP_CHUNK_HEAD_12)
}

func (nc *NetConnection) sendChunk(mem util.Memory, head *ChunkHeader, headType byte) (err error) {
	head.WriteTo(headType, &nc.chunkHeaderBuf)
	defer func(reuse net.Buffers) {
		nc.sendBuffers = reuse
	}(nc.sendBuffers[:0])
	nc.sendBuffers = append(nc.sendBuffers, nc.chunkHeaderBuf)
	var chunk3 util.Buffer = nc.chunkHeaderBuf[nc.chunkHeaderBuf.Len():20]
	head.WriteTo(RTMP_CHUNK_HEAD_1, &chunk3)
	r := mem.NewReader()
	for {
		r.RangeN(nc.WriteChunkSize, func(buf []byte) {
			nc.sendBuffers = append(nc.sendBuffers, buf)
		})
		if r.Length <= 0 {
			break
		}
		// 如果在音视频数据太大,一次发送不完,那么这里进行分割(data + Chunk Basic Header(1))
		nc.sendBuffers = append(nc.sendBuffers, chunk3)
	}
	var nw int64
	nw, err = nc.sendBuffers.WriteTo(nc.Conn)
	nc.writeSeqNum += uint32(nw)
	return err
}

func (nc *NetConnection) GetMediaDataPool() *util.ScalableMemoryAllocator {
	return nc.mediaDataPool
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

func (nc *NetConnection) decodeCommandAMF0(chunk *Chunk, body []byte) {
	amf := AMF(body)             // rtmp_amf.go, amf 是 bytes类型, 将rtmp body(payload)放到bytes.Buffer(amf)中去.
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
