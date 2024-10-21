package rtmp

import (
	"errors"
	"net"
	"runtime"
	"sync/atomic"

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
	tmpBuf                        util.Buffer //用来接收/发送小数据，复用内存
	chunkHeaderBuf                util.Buffer
	mediaDataPool                 util.RecyclableMemory
	writing                       atomic.Bool // false 可写，true 不可写
	Receivers                     map[uint32]*m7s.Publisher
}

func NewNetConnection(conn net.Conn) (ret *NetConnection) {
	ret = &NetConnection{
		Conn:            conn,
		BufReader:       util.NewBufReader(conn),
		WriteChunkSize:  RTMP_DEFAULT_CHUNK_SIZE,
		ReadChunkSize:   RTMP_DEFAULT_CHUNK_SIZE,
		incommingChunks: make(map[uint32]*Chunk),
		bandwidth:       RTMP_MAX_CHUNK_SIZE << 3,
		tmpBuf:          make(util.Buffer, 4),
		chunkHeaderBuf:  make(util.Buffer, 0, 20),
		Receivers:       make(map[uint32]*m7s.Publisher),
	}
	ret.mediaDataPool.SetAllocator(util.NewScalableMemoryAllocator(1 << util.MinPowerOf2))
	return
}

func (nc *NetConnection) Init(conn net.Conn) {
	nc.Conn = conn
	nc.BufReader = util.NewBufReader(conn)
	nc.bandwidth = RTMP_MAX_CHUNK_SIZE << 3
	nc.ReadChunkSize = RTMP_DEFAULT_CHUNK_SIZE
	nc.WriteChunkSize = RTMP_DEFAULT_CHUNK_SIZE
	nc.incommingChunks = make(map[uint32]*Chunk)
	nc.tmpBuf = make(util.Buffer, 4)
	nc.chunkHeaderBuf = make(util.Buffer, 0, 20)
	nc.mediaDataPool.SetAllocator(util.NewScalableMemoryAllocator(1 << util.MinPowerOf2))
	nc.Receivers = make(map[uint32]*m7s.Publisher)
}

func (nc *NetConnection) Dispose() {
	nc.Conn.Close()
	nc.BufReader.Recycle()
	nc.mediaDataPool.Recycle()
}

func (nc *NetConnection) SendStreamID(eventType uint16, streamID uint32) (err error) {
	return nc.SendMessage(RTMP_MSG_USER_CONTROL, &StreamIDMessage{UserControlMessage{EventType: eventType}, streamID})
}
func (nc *NetConnection) SendUserControl(eventType uint16) error {
	return nc.SendMessage(RTMP_MSG_USER_CONTROL, &UserControlMessage{EventType: eventType})
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
	var bufSize = 0
	if unRead := msgLen - chunk.bufLen; unRead < nc.ReadChunkSize {
		bufSize = unRead
	} else {
		bufSize = nc.ReadChunkSize
	}
	nc.readSeqNum += uint32(bufSize)
	if chunk.bufLen == 0 {
		chunk.AVData.RecyclableMemory = util.RecyclableMemory{}
		chunk.AVData.SetAllocator(nc.mediaDataPool.GetAllocator())
		chunk.AVData.NextN(msgLen)
	}
	buffer := chunk.AVData.Buffers[0]
	err = nc.ReadRange(bufSize, func(buf []byte) {
		copy(buffer[chunk.bufLen:], buf)
		chunk.bufLen += len(buf)
	})
	if err != nil {
		return nil, err
	}
	if chunk.bufLen == msgLen {
		msg = chunk
		switch chunk.MessageTypeID {
		case RTMP_MSG_AUDIO, RTMP_MSG_VIDEO:
			msg.AVData.Timestamp = chunk.ChunkHeader.ExtendTimestamp
		default:
			chunk.AVData.Recycle()
			err = GetRtmpMessage(msg, buffer)
		}
		msg.bufLen = 0
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
	if chunkType == 3 {
		// 3个字节的时间戳
	} else {
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
		switch chunkType {
		case 0:
			h.ExtendTimestamp = h.Timestamp
		case 1, 2:
			h.ExtendTimestamp += (h.Timestamp - 0xffffff)
		}
	} else {
		switch chunkType {
		case 0:
			h.ExtendTimestamp = h.Timestamp
		case 1, 2:
			h.ExtendTimestamp += h.Timestamp
		}
	}

	return nil
}

func (nc *NetConnection) RecvMessage() (msg *Chunk, err error) {
	if nc.readSeqNum >= nc.bandwidth {
		nc.totalRead += nc.readSeqNum
		nc.readSeqNum = 0
		err = nc.SendMessage(RTMP_MSG_ACK, Uint32Message(nc.totalRead))
	}
	for msg == nil && err == nil {
		if msg, err = nc.readChunk(); msg != nil && err == nil {
			switch msg.MessageTypeID {
			case RTMP_MSG_CHUNK_SIZE:
				nc.ReadChunkSize = int(msg.MsgData.(Uint32Message))
				nc.Info("msg read chunk size", "readChunkSize", nc.ReadChunkSize)
			case RTMP_MSG_ABORT:
				delete(nc.incommingChunks, uint32(msg.MsgData.(Uint32Message)))
			case RTMP_MSG_ACK, RTMP_MSG_EDGE:
			case RTMP_MSG_USER_CONTROL:
				if _, ok := msg.MsgData.(*PingRequestMessage); ok {
					nc.SendUserControl(RTMP_USER_PING_RESPONSE)
				}
			case RTMP_MSG_ACK_SIZE:
				nc.bandwidth = uint32(msg.MsgData.(Uint32Message))
			case RTMP_MSG_BANDWIDTH:
				nc.bandwidth = msg.MsgData.(*SetPeerBandwidthMessage).AcknowledgementWindowsize
			case RTMP_MSG_AMF0_COMMAND:
				return msg, err
			case RTMP_MSG_AUDIO:
				if r, ok := nc.Receivers[msg.MessageStreamID]; ok && r.PubAudio {
					err = r.WriteAudio(msg.AVData.WrapAudio())
				} else {
					msg.AVData.Recycle()
					//if r.PubAudio {
					//	nc.Warn("ReceiveAudio", "MessageStreamID", msg.MessageStreamID)
					//}
				}
			case RTMP_MSG_VIDEO:
				if r, ok := nc.Receivers[msg.MessageStreamID]; ok && r.PubVideo {
					err = r.WriteVideo(msg.AVData.WrapVideo())
				} else {
					msg.AVData.Recycle()
					//if r.PubVideo {
					//	nc.Warn("ReceiveVideo", "MessageStreamID", msg.MessageStreamID)
					//}
				}
			}
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
		err = nc.SendStreamID(RTMP_USER_PING_REQUEST, 0)
	}
	for !nc.writing.CompareAndSwap(false, true) {
		runtime.Gosched()
	}
	defer nc.writing.Store(false)
	nc.tmpBuf.Reset()
	amf := AMF{nc.tmpBuf}
	if nc.ObjectEncoding == 0 {
		msg.Encode(&amf)
	} else {
		amf := AMF3{AMF: amf}
		msg.Encode(&amf)
	}
	nc.tmpBuf = amf.Buffer
	head := newChunkHeader(t)
	head.MessageLength = uint32(nc.tmpBuf.Len())
	if sid, ok := msg.(HaveStreamID); ok {
		head.MessageStreamID = sid.GetStreamID()
	}
	return nc.sendChunk(net.Buffers{nc.tmpBuf}, head, RTMP_CHUNK_HEAD_12)
}

func (nc *NetConnection) sendChunk(data net.Buffers, head *ChunkHeader, headType byte) (err error) {
	nc.chunkHeaderBuf.Reset()
	head.WriteTo(headType, &nc.chunkHeaderBuf)
	chunks := net.Buffers{nc.chunkHeaderBuf}
	var chunk3 util.Buffer = nc.chunkHeaderBuf[nc.chunkHeaderBuf.Len():20]
	head.WriteTo(RTMP_CHUNK_HEAD_1, &chunk3)
	r := util.NewReadableBuffersFromBytes(data...)
	for {
		r.RangeN(nc.WriteChunkSize, func(buf []byte) {
			chunks = append(chunks, buf)
		})
		if r.Length <= 0 {
			break
		}
		// 如果在音视频数据太大,一次发送不完,那么这里进行分割(data + Chunk Basic Header(1))
		chunks = append(chunks, chunk3)
	}
	var nw int64
	nw, err = chunks.WriteTo(nc.Conn)
	nc.writeSeqNum += uint32(nw)
	return err
}
