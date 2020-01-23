package rtmp

import (
	"bufio"
	"errors"
	"github.com/langhuihui/monibuca/monica/pool"
	"github.com/langhuihui/monibuca/monica/util"
	"io"
	"log"
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

	SEND_CREATE_STREAM_MESSAGE          = "Send Create Stream Message"
	SEND_CREATE_STREAM_RESPONSE_MESSAGE = "Send Create Stream Response Message"

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

func newConnectResponseMessageData(objectEncoding float64) (amfobj AMFObjects) {
	amfobj = newAMFObjects()
	amfobj["fmsVer"] = "monibuca/1.0"
	amfobj["capabilities"] = 31
	amfobj["mode"] = 1
	amfobj["Author"] = "dexter"
	amfobj["level"] = Level_Status
	amfobj["code"] = NetConnection_Connect_Success
	amfobj["objectEncoding"] = uint64(objectEncoding)

	return
}

func newPublishResponseMessageData(streamid uint32, code, level string) (amfobj AMFObjects) {
	amfobj = newAMFObjects()
	amfobj["code"] = code
	amfobj["level"] = level
	amfobj["streamid"] = streamid

	return
}

func newPlayResponseMessageData(streamid uint32, code, level string) (amfobj AMFObjects) {
	amfobj = newAMFObjects()
	amfobj["code"] = code
	amfobj["level"] = level
	amfobj["streamid"] = streamid

	return
}

type NetConnection struct {
	*bufio.ReadWriter
	bandwidth          uint32
	readSeqNum         uint32 // 当前读的字节
	writeSeqNum        uint32 // 当前写的字节
	totalWrite         uint32 // 总共写了多少字节
	totalRead          uint32 // 总共读了多少字节
	writeChunkSize     int
	readChunkSize      int
	incompleteRtmpBody map[uint32][]byte       // 完整的RtmpBody,在网络上是被分成一块一块的,需要将其组装起来
	nextStreamID       func(uint32) uint32     // 下一个流ID
	streamID           uint32                  // 流ID
	rtmpHeader         map[uint32]*ChunkHeader // RtmpHeader
	objectEncoding     float64
	appName            string
}

func (conn *NetConnection) OnConnect() (err error) {
	var msg *Chunk
	if msg, err = conn.RecvMessage(); err == nil {
		defer chunkMsgPool.Put(msg)
		if connect, ok := msg.MsgData.(*CallMessage); ok {
			if connect.CommandName == "connect" {
				app := DecodeAMFObject(connect.Object, "app")                       // 客户端要连接到的服务应用名
				objectEncoding := DecodeAMFObject(connect.Object, "objectEncoding") // AMF编码方法
				if objectEncoding != nil {
					conn.objectEncoding = objectEncoding.(float64)
				}
				conn.appName = app.(string)
				log.Printf("app:%v,objectEncoding:%v", app, objectEncoding)
				err = conn.SendMessage(SEND_ACK_WINDOW_SIZE_MESSAGE, uint32(512<<10))
				err = conn.SendMessage(SEND_SET_PEER_BANDWIDTH_MESSAGE, uint32(512<<10))
				err = conn.SendMessage(SEND_STREAM_BEGIN_MESSAGE, nil)
				err = conn.SendMessage(SEND_CONNECT_RESPONSE_MESSAGE, conn.objectEncoding)
				return
			}
		}
	}
	return
}
func (conn *NetConnection) SendMessage(message string, args interface{}) error {
	switch message {
	case SEND_CHUNK_SIZE_MESSAGE:
		size, ok := args.(uint32)
		if !ok {
			return errors.New(SEND_CHUNK_SIZE_MESSAGE + ", The parameter only one(size uint32)!")
		}
		return conn.writeMessage(RTMP_MSG_CHUNK_SIZE, Uint32Message(size))
	case SEND_ACK_MESSAGE:
		num, ok := args.(uint32)
		if !ok {
			return errors.New(SEND_ACK_MESSAGE + ", The parameter only one(number uint32)!")
		}
		return conn.writeMessage(RTMP_MSG_ACK, Uint32Message(num))
	case SEND_ACK_WINDOW_SIZE_MESSAGE:
		size, ok := args.(uint32)
		if !ok {
			return errors.New(SEND_ACK_WINDOW_SIZE_MESSAGE + ", The parameter only one(size uint32)!")
		}
		return conn.writeMessage(RTMP_MSG_ACK_SIZE, Uint32Message(size))
	case SEND_SET_PEER_BANDWIDTH_MESSAGE:
		size, ok := args.(uint32)
		if !ok {
			return errors.New(SEND_SET_PEER_BANDWIDTH_MESSAGE + ", The parameter only one(size uint32)!")
		}
		return conn.writeMessage(RTMP_MSG_BANDWIDTH, &SetPeerBandwidthMessage{
			AcknowledgementWindowsize: size,
			LimitType:                 byte(2),
		})
	case SEND_STREAM_BEGIN_MESSAGE:
		if args != nil {
			return errors.New(SEND_STREAM_BEGIN_MESSAGE + ", The parameter is nil")
		}
		return conn.writeMessage(RTMP_MSG_USER_CONTROL, &StreamIDMessage{UserControlMessage{EventType: RTMP_USER_STREAM_BEGIN}, conn.streamID})
	case SEND_STREAM_IS_RECORDED_MESSAGE:
		if args != nil {
			return errors.New(SEND_STREAM_IS_RECORDED_MESSAGE + ", The parameter is nil")
		}
		return conn.writeMessage(RTMP_MSG_USER_CONTROL, &StreamIDMessage{UserControlMessage{EventType: RTMP_USER_STREAM_IS_RECORDED}, conn.streamID})

	case SEND_SET_BUFFER_LENGTH_MESSAGE:
		if args != nil {
			return errors.New(SEND_SET_BUFFER_LENGTH_MESSAGE + ", The parameter is nil")
		}
		m := new(SetBufferMessage)
		m.EventType = RTMP_USER_SET_BUFFLEN
		m.Millisecond = 100
		m.StreamID = conn.streamID
		return conn.writeMessage(RTMP_MSG_USER_CONTROL, m)
	case SEND_PING_REQUEST_MESSAGE:
		if args != nil {
			return errors.New(SEND_PING_REQUEST_MESSAGE + ", The parameter is nil")
		}
		return conn.writeMessage(RTMP_MSG_USER_CONTROL, &UserControlMessage{EventType: RTMP_USER_PING_REQUEST})
	case SEND_PING_RESPONSE_MESSAGE:
		if args != nil {
			return errors.New(SEND_PING_RESPONSE_MESSAGE + ", The parameter is nil")
		}
		return conn.writeMessage(RTMP_MSG_USER_CONTROL, &UserControlMessage{EventType: RTMP_USER_PING_RESPONSE})
	case SEND_CREATE_STREAM_MESSAGE:
		if args != nil {
			return errors.New(SEND_CREATE_STREAM_MESSAGE + ", The parameter is nil")
		}

		m := &CreateStreamMessage{}
		m.CommandName = "createStream"
		m.TransactionId = 1
		return conn.writeMessage(RTMP_MSG_AMF0_COMMAND, m)
	case SEND_CREATE_STREAM_RESPONSE_MESSAGE:
		tid, ok := args.(uint64)
		if !ok {
			return errors.New(SEND_CREATE_STREAM_RESPONSE_MESSAGE + ", The parameter only one(TransactionId uint64)!")
		}
		m := &ResponseCreateStreamMessage{}
		m.CommandName = Response_Result
		m.TransactionId = tid
		m.StreamId = conn.streamID
		return conn.writeMessage(RTMP_MSG_AMF0_COMMAND, m)
	case SEND_PLAY_MESSAGE:
		data, ok := args.(map[interface{}]interface{})
		if !ok {
			errors.New(SEND_PLAY_MESSAGE + ", The parameter is map[interface{}]interface{}")
		}
		m := new(PlayMessage)
		m.CommandName = "play"
		m.TransactionId = 1
		for i, v := range data {
			if i == "StreamName" {
				m.StreamName = v.(string)
			} else if i == "Start" {
				m.Start = v.(uint64)
			} else if i == "Duration" {
				m.Duration = v.(uint64)
			} else if i == "Rest" {
				m.Rest = v.(bool)
			}
		}
		return conn.writeMessage(RTMP_MSG_AMF0_COMMAND, m)
	case SEND_PLAY_RESPONSE_MESSAGE:
		data, ok := args.(AMFObjects)
		if !ok {
			errors.New(SEND_PLAY_RESPONSE_MESSAGE + ", The parameter is AMFObjects(map[string]interface{})")
		}

		obj := newAMFObjects()
		var streamID uint32

		for i, v := range data {
			switch i {
			case "code", "level":
				obj[i] = v
			case "streamid":
				if t, ok := v.(uint32); ok {
					streamID = t
				}
			}
		}

		obj["clientid"] = 1

		m := new(ResponsePlayMessage)
		m.CommandName = Response_OnStatus
		m.TransactionId = 0
		m.Object = obj
		m.StreamID = streamID
		return conn.writeMessage(RTMP_MSG_AMF0_COMMAND, m)
	case SEND_CONNECT_RESPONSE_MESSAGE:
		data := newConnectResponseMessageData(args.(float64))
		//if !ok {
		//	errors.New(SEND_CONNECT_RESPONSE_MESSAGE + ", The parameter is AMFObjects(map[string]interface{})")
		//}

		//pro := newAMFObjects()
		info := newAMFObjects()

		//for i, v := range data {
		//	switch i {
		//	case "fmsVer", "capabilities", "mode", "Author", "level", "code", "objectEncoding":
		//		pro[i] = v
		//	}
		//}
		m := new(ResponseConnectMessage)
		m.CommandName = Response_Result
		m.TransactionId = 1
		m.Properties = data
		m.Infomation = info
		return conn.writeMessage(RTMP_MSG_AMF0_COMMAND, m)
	case SEND_CONNECT_MESSAGE:
		data, ok := args.(AMFObjects)
		if !ok {
			errors.New(SEND_CONNECT_MESSAGE + ", The parameter is AMFObjects(map[string]interface{})")
		}

		obj := newAMFObjects()
		info := newAMFObjects()

		for i, v := range data {
			switch i {
			case "videoFunction", "objectEncoding", "fpad", "flashVer", "capabilities", "pageUrl", "swfUrl", "tcUrl", "videoCodecs", "app", "audioCodecs":
				obj[i] = v
			}
		}

		m := new(CallMessage)
		m.CommandName = "connect"
		m.TransactionId = 1
		m.Object = obj
		m.Optional = info
		return conn.writeMessage(RTMP_MSG_AMF0_COMMAND, m)
	case SEND_PUBLISH_RESPONSE_MESSAGE, SEND_PUBLISH_START_MESSAGE:
		data, ok := args.(AMFObjects)
		if !ok {
			errors.New(SEND_CONNECT_MESSAGE + "or" + SEND_PUBLISH_START_MESSAGE + ", The parameter is AMFObjects(map[string]interface{})")
		}

		info := newAMFObjects()
		var streamID uint32

		for i, v := range data {
			switch i {
			case "code", "level":
				info[i] = v
			case "streamid":
				if t, ok := v.(uint32); ok {
					streamID = t
				}
			}
		}

		info["clientid"] = 1

		m := new(ResponsePublishMessage)
		m.CommandName = Response_OnStatus
		m.TransactionId = 0
		m.Infomation = info
		m.StreamID = streamID
		return conn.writeMessage(RTMP_MSG_AMF0_COMMAND, m)
	case SEND_UNPUBLISH_RESPONSE_MESSAGE:
	case SEND_FULL_AUDIO_MESSAGE:
		audio, ok := args.(*pool.SendPacket)
		if !ok {
			errors.New(message + ", The parameter is AVPacket")
		}

		return conn.sendAVMessage(audio, true, true)
	case SEND_AUDIO_MESSAGE:
		audio, ok := args.(*pool.SendPacket)
		if !ok {
			errors.New(message + ", The parameter is AVPacket")
		}

		return conn.sendAVMessage(audio, true, false)
	case SEND_FULL_VDIEO_MESSAGE:
		video, ok := args.(*pool.SendPacket)
		if !ok {
			errors.New(message + ", The parameter is AVPacket")
		}

		return conn.sendAVMessage(video, false, true)
	case SEND_VIDEO_MESSAGE:
		{
			video, ok := args.(*pool.SendPacket)
			if !ok {
				errors.New(message + ", The parameter is AVPacket")
			}

			return conn.sendAVMessage(video, false, false)
		}
	}

	return errors.New("send message no exist")
}

// 当发送音视频数据的时候,当块类型为12的时候,Chunk Message Header有一个字段TimeStamp,指明一个时间
// 当块类型为4,8的时候,Chunk Message Header有一个字段TimeStamp Delta,记录与上一个Chunk的时间差值
// 当块类型为0的时候,Chunk Message Header没有时间字段,与上一个Chunk时间值相同
func (conn *NetConnection) sendAVMessage(av *pool.SendPacket, isAudio bool, isFirst bool) error {
	if conn.writeSeqNum > conn.bandwidth {
		conn.totalWrite += conn.writeSeqNum
		conn.writeSeqNum = 0
		conn.SendMessage(SEND_ACK_MESSAGE, conn.totalWrite)
		conn.SendMessage(SEND_PING_REQUEST_MESSAGE, nil)
	}

	var err error
	var mark []byte
	var need []byte
	var head *ChunkHeader

	if isAudio {
		head = newRtmpHeader(RTMP_CSID_AUDIO, av.Timestamp, uint32(len(av.Packet.Payload)), RTMP_MSG_AUDIO, conn.streamID, 0)
	} else {
		head = newRtmpHeader(RTMP_CSID_VIDEO, av.Timestamp, uint32(len(av.Packet.Payload)), RTMP_MSG_VIDEO, conn.streamID, 0)
	}

	// 第一次是发送关键帧,需要完整的消息头(Chunk Basic Header(1) + Chunk Message Header(11) + Extended Timestamp(4)(可能会要包括))
	// 后面开始,就是直接发送音视频数据,那么直接发送,不需要完整的块(Chunk Basic Header(1) + Chunk Message Header(7))
	// 当Chunk Type为0时(即Chunk12),
	if isFirst {
		mark, need, err = encodeChunk12(head, av.Packet.Payload, conn.writeChunkSize)
	} else {
		mark, need, err = encodeChunk8(head, av.Packet.Payload, conn.writeChunkSize)
	}

	if err != nil {
		return err
	}

	_, err = conn.Write(mark)
	if err != nil {
		return err
	}

	err = conn.Flush()
	if err != nil {
		return err
	}

	conn.writeSeqNum += uint32(len(mark))

	// 如果音视频数据太大,一次发送不完,那么在这里进行分割(data + Chunk Basic Header(1))
	for need != nil && len(need) > 0 {
		mark, need, err = encodeChunk1(head, need, conn.writeChunkSize)
		if err != nil {
			return err
		}

		_, err = conn.Write(mark)
		if err != nil {
			return err
		}

		err = conn.Flush()
		if err != nil {
			return err
		}

		conn.writeSeqNum += uint32(len(mark))
	}

	return nil
}

func (conn *NetConnection) readChunk() (msg *Chunk, err error) {
	head, err := conn.ReadByte()
	conn.readSeqNum++
	if err != nil {
		return nil, err
	}

	ChunkStreamID := uint32(head & 0x3f) // 0011 1111
	ChunkType := (head & 0xc0) >> 6      // 1100 0000

	// 如果块流ID为0,1的话,就需要计算.
	ChunkStreamID, err = conn.readChunkStreamID(ChunkStreamID)
	if err != nil {
		return nil, errors.New("get chunk stream id error :" + err.Error())
	}

	h, ok := conn.rtmpHeader[ChunkStreamID]
	if !ok {
		h = new(ChunkHeader)
		h.ChunkStreamID = ChunkStreamID
		h.ChunkType = ChunkType
		conn.rtmpHeader[ChunkStreamID] = h
	}
	currentBody, ok := conn.incompleteRtmpBody[ChunkStreamID]
	if ChunkType != 3 && ok {
		// 如果块类型不为3,那么这个rtmp的body应该为空.
		return nil, errors.New("incompleteRtmpBody error")
	}

	chunkHead, err := conn.readChunkType(h, ChunkType)
	if err != nil {
		return nil, errors.New("get chunk type error :" + err.Error())
	}
	msgLen := int(chunkHead.MessageLength)
	if !ok {
		currentBody = (pool.GetSlice(msgLen))[:0]
		conn.incompleteRtmpBody[ChunkStreamID] = currentBody
	}

	markRead := len(currentBody)
	needRead := conn.readChunkSize
	unRead := msgLen - markRead
	if unRead < needRead {
		needRead = unRead
	}
	if n, err := io.ReadFull(conn, currentBody[markRead:needRead+markRead]); err != nil {
		return nil, err
	} else {
		markRead += n
		conn.readSeqNum += uint32(n)
	}
	currentBody = currentBody[:markRead]
	conn.incompleteRtmpBody[ChunkStreamID] = currentBody

	// 如果读完了一个完整的块,那么就返回这个消息,没读完继续递归读块.
	if markRead == msgLen {

		msg := chunkMsgPool.Get().(*Chunk)
		msg.Body = currentBody
		msg.ChunkHeader = chunkHead.Clone()
		GetRtmpMessage(msg)
		delete(conn.incompleteRtmpBody, ChunkStreamID)
		return msg, nil
	}

	return conn.readChunk()
}

func (conn *NetConnection) readChunkStreamID(csid uint32) (chunkStreamID uint32, err error) {
	chunkStreamID = csid

	switch csid {
	case 0:
		{
			u8, err := conn.ReadByte()
			conn.readSeqNum++
			if err != nil {
				return 0, err
			}

			chunkStreamID = 64 + uint32(u8)
		}
	case 1:
		{
			u16_0, err1 := conn.ReadByte()
			if err1 != nil {
				return 0, err1
			}
			u16_1, err1 := conn.ReadByte()
			if err1 != nil {
				return 0, err1
			}
			conn.readSeqNum += 2
			chunkStreamID = 64 + uint32(u16_0) + (uint32(u16_1) << 8)
		}
	}

	return chunkStreamID, nil
}

func (conn *NetConnection) readChunkType(h *ChunkHeader, chunkType byte) (head *ChunkHeader, err error) {
	switch chunkType {
	case 0:
		{
			// Timestamp 3 bytes
			b := pool.GetSlice(3)
			if _, err := io.ReadFull(conn, b); err != nil {
				return nil, err
			}
			conn.readSeqNum += 3
			h.Timestamp = util.BigEndian.Uint24(b) //type = 0的时间戳为绝对时间,其他的都为相对时间

			// Message Length 3 bytes
			if _, err = io.ReadFull(conn, b); err != nil { // 读取Message Length,这里的长度指的是一条信令或者一帧视频数据或音频数据的长度,而不是Chunk data的长度.
				return nil, err
			}
			conn.readSeqNum += 3
			h.MessageLength = util.BigEndian.Uint24(b)
			pool.RecycleSlice(b)
			// Message Type ID 1 bytes
			v, err := conn.ReadByte() // 读取Message Type ID
			if err != nil {
				return nil, err
			}
			conn.readSeqNum++
			h.MessageTypeID = v

			// Message Stream ID 4bytes
			bb := pool.GetSlice(4)
			if _, err = io.ReadFull(conn, bb); err != nil { // 读取Message Stream ID
				return nil, err
			}
			conn.readSeqNum += 4
			h.MessageStreamID = util.LittleEndian.Uint32(bb)

			// ExtendTimestamp 4 bytes
			if h.Timestamp == 0xffffff { // 对于type 0的chunk,绝对时间戳在这里表示,如果时间戳值大于等于0xffffff(16777215),该值必须是0xffffff,且时间戳扩展字段必须发送,其他情况没有要求
				if _, err = io.ReadFull(conn, bb); err != nil {
					return nil, err
				}
				conn.readSeqNum += 4
				h.ExtendTimestamp = util.BigEndian.Uint32(bb)
			}
			pool.RecycleSlice(bb)
		}
	case 1:
		{
			// Timestamp 3 bytes
			b := pool.GetSlice(3)
			if _, err = io.ReadFull(conn, b); err != nil {
				return nil, err
			}
			conn.readSeqNum += 3
			h.ChunkType = chunkType
			h.Timestamp = util.BigEndian.Uint24(b)

			// Message Length 3 bytes
			if _, err = io.ReadFull(conn, b); err != nil {
				return nil, err
			}
			conn.readSeqNum += 3
			h.MessageLength = util.BigEndian.Uint24(b)
			pool.RecycleSlice(b)
			// Message Type ID 1 bytes
			v, err := conn.ReadByte()
			if err != nil {
				return nil, err
			}
			conn.readSeqNum++
			h.MessageTypeID = v

			// ExtendTimestamp 4 bytes
			if h.Timestamp == 0xffffff {
				bb := pool.GetSlice(4)
				if _, err := io.ReadFull(conn, bb); err != nil {
					return nil, err
				}
				conn.readSeqNum += 4
				h.ExtendTimestamp = util.BigEndian.Uint32(bb)
				pool.RecycleSlice(bb)
			}
		}
	case 2:
		{
			// Timestamp 3 bytes
			b := pool.GetSlice(3)
			if _, err = io.ReadFull(conn, b); err != nil {
				return nil, err
			}
			conn.readSeqNum += 3
			h.ChunkType = chunkType
			h.Timestamp = util.BigEndian.Uint24(b)
			pool.RecycleSlice(b)
			// ExtendTimestamp 4 bytes
			if h.Timestamp == 0xffffff {
				bb := pool.GetSlice(4)
				if _, err := io.ReadFull(conn, bb); err != nil {
					return nil, err
				}
				conn.readSeqNum += 4
				h.ExtendTimestamp = util.BigEndian.Uint32(bb)
				pool.RecycleSlice(bb)
			}
		}
	case 3:
		{
			h.ChunkType = chunkType
		}
	}

	return h, nil
}

func (conn *NetConnection) RecvMessage() (msg *Chunk, err error) {
	if conn.readSeqNum >= conn.bandwidth {
		conn.totalRead += conn.readSeqNum
		conn.readSeqNum = 0
		//sendAck(conn, conn.totalRead)
		conn.SendMessage(SEND_ACK_MESSAGE, conn.totalRead)
	}

	msg, err = conn.readChunk()
	if err != nil {
		return nil, err
	}

	// 如果消息是类型是用户控制消息,那么我们就简单做一些相应的处理,
	// 然后继续读取下一个消息.如果不是用户控制消息,就将消息返回就好.
	messageType := msg.MessageTypeID
	if RTMP_MSG_CHUNK_SIZE <= messageType && messageType <= RTMP_MSG_EDGE {
		switch messageType {
		case RTMP_MSG_CHUNK_SIZE:
			m := msg.MsgData.(Uint32Message)
			conn.readChunkSize = int(m)
			return conn.RecvMessage()
		case RTMP_MSG_ABORT:
			m := msg.MsgData.(Uint32Message)
			delete(conn.incompleteRtmpBody, uint32(m))
			return conn.RecvMessage()
		case RTMP_MSG_ACK, RTMP_MSG_EDGE:
			return conn.RecvMessage()
		case RTMP_MSG_USER_CONTROL:
			if _, ok := msg.MsgData.(*PingRequestMessage); ok {
				//sendPingResponse(conn)
				conn.SendMessage(SEND_PING_RESPONSE_MESSAGE, nil)
			}
			return conn.RecvMessage()
		case RTMP_MSG_ACK_SIZE:
			m := msg.MsgData.(Uint32Message)
			conn.bandwidth = uint32(m)
			return conn.RecvMessage()
		case RTMP_MSG_BANDWIDTH:
			m := msg.MsgData.(*SetPeerBandwidthMessage)
			conn.bandwidth = m.AcknowledgementWindowsize
			return conn.RecvMessage()
		}
	}

	return msg, err
}
func (conn *NetConnection) writeMessage(t byte, msg RtmpMessage) error {
	body := msg.Encode()
	head := newChunkHeader(t)
	head.MessageLength = uint32(len(body))
	if sid, ok := msg.(HaveStreamID); ok {
		head.MessageStreamID = sid.GetStreamID()
	}
	if conn.writeSeqNum > conn.bandwidth {
		conn.totalWrite += conn.writeSeqNum
		conn.writeSeqNum = 0
		conn.SendMessage(SEND_ACK_MESSAGE, conn.totalWrite)
		conn.SendMessage(SEND_PING_REQUEST_MESSAGE, nil)
	}

	mark, need, err := encodeChunk12(head, body, conn.writeChunkSize)
	if err != nil {
		return err
	}

	_, err = conn.Write(mark)
	if err != nil {
		return err
	}

	err = conn.Flush()
	if err != nil {
		return err
	}

	conn.writeSeqNum += uint32(len(mark))

	for need != nil && len(need) > 0 {
		mark, need, err = encodeChunk1(head, need, conn.writeChunkSize)
		if err != nil {
			return err
		}

		_, err = conn.Write(mark)
		if err != nil {
			return err
		}

		err = conn.Flush()
		if err != nil {
			return err
		}

		conn.writeSeqNum += uint32(len(mark))
	}

	return nil
}
