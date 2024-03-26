package pkg

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"runtime"
	"sync/atomic"

	"m7s.live/m7s/v5/pkg/util"
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

type BytesPool struct {
	util.Pool[[]byte]
	ItemSize int
}

func (bp *BytesPool) Get(size int) []byte {
	if size != bp.ItemSize {
		return make([]byte, size)
	}
	ret := bp.Pool.Get()
	if ret == nil {
		return make([]byte, size)
	}
	return ret
}

func (bp *BytesPool) Put(b []byte) {
	if cap(b) != bp.ItemSize {
		bp.ItemSize = cap(b)
		bp.Clear()
	}
	bp.Pool.Put(b)
}

type NetConnection struct {
	*slog.Logger    `json:"-" yaml:"-"`
	*bufio.Reader   `json:"-" yaml:"-"`
	net.Conn        `json:"-" yaml:"-"`
	bandwidth       uint32
	readSeqNum      uint32 // 当前读的字节
	writeSeqNum     uint32 // 当前写的字节
	totalWrite      uint32 // 总共写了多少字节
	totalRead       uint32 // 总共读了多少字节
	WriteChunkSize  int
	readChunkSize   int
	incommingChunks map[uint32]*Chunk
	ObjectEncoding  float64
	AppName         string
	tmpBuf          util.Buffer //用来接收/发送小数据，复用内存
	chunkHeader     util.Buffer
	byteChunkPool   BytesPool
	byte16Pool      BytesPool
	writing         atomic.Bool // false 可写，true 不可写
}

func NewNetConnection(conn net.Conn) *NetConnection {
	return &NetConnection{
		Conn:            conn,
		Reader:          bufio.NewReader(conn),
		WriteChunkSize:  RTMP_DEFAULT_CHUNK_SIZE,
		readChunkSize:   RTMP_DEFAULT_CHUNK_SIZE,
		incommingChunks: make(map[uint32]*Chunk),
		bandwidth:       RTMP_MAX_CHUNK_SIZE << 3,
		tmpBuf:          make(util.Buffer, 4),
		chunkHeader:     make(util.Buffer, 0, 16),
	}
}
func (conn *NetConnection) ReadFull(buf []byte) (n int, err error) {
	n, err = io.ReadFull(conn.Reader, buf)
	if err == nil {
		conn.readSeqNum += uint32(n)
	}
	return
}
func (conn *NetConnection) SendStreamID(eventType uint16, streamID uint32) (err error) {
	return conn.SendMessage(RTMP_MSG_USER_CONTROL, &StreamIDMessage{UserControlMessage{EventType: eventType}, streamID})
}
func (conn *NetConnection) SendUserControl(eventType uint16) error {
	return conn.SendMessage(RTMP_MSG_USER_CONTROL, &UserControlMessage{EventType: eventType})
}

func (conn *NetConnection) ResponseCreateStream(tid uint64, streamID uint32) error {
	m := &ResponseCreateStreamMessage{}
	m.CommandName = Response_Result
	m.TransactionId = tid
	m.StreamId = streamID
	return conn.SendMessage(RTMP_MSG_AMF0_COMMAND, m)
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

func (conn *NetConnection) readChunk() (msg *Chunk, err error) {
	head, err := conn.ReadByte()
	if err != nil {
		return nil, err
	}
	conn.readSeqNum++
	ChunkStreamID := uint32(head & 0x3f) // 0011 1111
	ChunkType := head >> 6               // 1100 0000
	// 如果块流ID为0,1的话,就需要计算.
	ChunkStreamID, err = conn.readChunkStreamID(ChunkStreamID)
	if err != nil {
		return nil, errors.New("get chunk stream id error :" + err.Error())
	}
	// println("ChunkStreamID:", ChunkStreamID, "ChunkType:", ChunkType)
	chunk, ok := conn.incommingChunks[ChunkStreamID]

	if ChunkType != 3 && ok && chunk.AVData.Length > 0 {
		// 如果块类型不为3,那么这个rtmp的body应该为空.
		return nil, errors.New("incompleteRtmpBody error")
	}
	if !ok {
		chunk = &Chunk{}
		conn.incommingChunks[ChunkStreamID] = chunk
	}

	if err = conn.readChunkType(&chunk.ChunkHeader, ChunkType); err != nil {
		return nil, errors.New("get chunk type error :" + err.Error())
	}
	msgLen := int(chunk.MessageLength)

	needRead := conn.readChunkSize
	if unRead := msgLen - chunk.AVData.Length; unRead < needRead {
		needRead = unRead
	}
	mem := conn.byteChunkPool.Get(needRead)
	if n, err := conn.ReadFull(mem); err != nil {
		conn.byteChunkPool.Put(mem)
		return nil, err
	} else {
		conn.readSeqNum += uint32(n)
	}
	chunk.AVData.Data = append(chunk.AVData.Data, mem)
	if chunk.AVData.ReadFromBytes(mem); chunk.AVData.Length == msgLen {
		chunk.ChunkHeader.ExtendTimestamp += chunk.ChunkHeader.Timestamp
		msg = chunk
		switch chunk.MessageTypeID {
		case RTMP_MSG_AUDIO, RTMP_MSG_VIDEO:
			msg.AVData.Timestamp = chunk.ChunkHeader.ExtendTimestamp
		default:
			err = GetRtmpMessage(msg, msg.AVData.ToBytes())
			msg.AVData.Recycle()
		}
		conn.incommingChunks[ChunkStreamID] = &Chunk{
			ChunkHeader: chunk.ChunkHeader,
		}
	}
	return
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

func (conn *NetConnection) readChunkType(h *ChunkHeader, chunkType byte) (err error) {
	conn.tmpBuf.Reset()
	b4 := conn.tmpBuf.Malloc(4)
	b3 := b4[:3]
	if chunkType == 3 {
		// 3个字节的时间戳
	} else {
		// Timestamp 3 bytes
		if _, err = conn.ReadFull(b3); err != nil {
			return err
		}
		util.GetBE(b3, &h.Timestamp)
		if chunkType != 2 {
			if _, err = conn.ReadFull(b3); err != nil {
				return err
			}
			util.GetBE(b3, &h.MessageLength)
			// Message Type ID 1 bytes
			if h.MessageTypeID, err = conn.ReadByte(); err != nil {
				return err
			}
			conn.readSeqNum++
			if chunkType == 0 {
				// Message Stream ID 4bytes
				if _, err = conn.ReadFull(b4); err != nil { // 读取Message Stream ID
					return err
				}
				h.MessageStreamID = binary.LittleEndian.Uint32(b4)
			}
		}
	}

	// ExtendTimestamp 4 bytes
	if h.Timestamp == 0xffffff { // 对于type 0的chunk,绝对时间戳在这里表示,如果时间戳值大于等于0xffffff(16777215),该值必须是0xffffff,且时间戳扩展字段必须发送,其他情况没有要求
		if _, err = conn.ReadFull(b4); err != nil {
			return err
		}
		util.GetBE(b4, &h.Timestamp)
	}
	if chunkType == 0 {
		h.ExtendTimestamp = h.Timestamp
		h.Timestamp = 0
	}
	return nil
}

func (conn *NetConnection) RecvMessage() (msg *Chunk, err error) {
	if conn.readSeqNum >= conn.bandwidth {
		conn.totalRead += conn.readSeqNum
		conn.readSeqNum = 0
		err = conn.SendMessage(RTMP_MSG_ACK, Uint32Message(conn.totalRead))
	}
	for msg == nil && err == nil {
		if msg, err = conn.readChunk(); msg != nil && err == nil {
			switch msg.MessageTypeID {
			case RTMP_MSG_CHUNK_SIZE:
				conn.readChunkSize = int(msg.MsgData.(Uint32Message))
				// RTMPPlugin.Info("msg read chunk size", zap.Int("readChunkSize", conn.readChunkSize))
			case RTMP_MSG_ABORT:
				delete(conn.incommingChunks, uint32(msg.MsgData.(Uint32Message)))
			case RTMP_MSG_ACK, RTMP_MSG_EDGE:
			case RTMP_MSG_USER_CONTROL:
				if _, ok := msg.MsgData.(*PingRequestMessage); ok {
					conn.SendUserControl(RTMP_USER_PING_RESPONSE)
				}
			case RTMP_MSG_ACK_SIZE:
				conn.bandwidth = uint32(msg.MsgData.(Uint32Message))
			case RTMP_MSG_BANDWIDTH:
				conn.bandwidth = msg.MsgData.(*SetPeerBandwidthMessage).AcknowledgementWindowsize
			case RTMP_MSG_AMF0_COMMAND, RTMP_MSG_AUDIO, RTMP_MSG_VIDEO:
				return msg, err
			}
		}
	}
	return
}
func (conn *NetConnection) SendMessage(t byte, msg RtmpMessage) (err error) {
	if conn == nil {
		return errors.New("connection is nil")
	}
	if conn.writeSeqNum > conn.bandwidth {
		conn.totalWrite += conn.writeSeqNum
		conn.writeSeqNum = 0
		err = conn.SendMessage(RTMP_MSG_ACK, Uint32Message(conn.totalWrite))
		err = conn.SendStreamID(RTMP_USER_PING_REQUEST, 0)
	}
	for !conn.writing.CompareAndSwap(false, true) {
		runtime.Gosched()
	}
	defer conn.writing.Store(false)
	conn.tmpBuf.Reset()
	amf := AMF{conn.tmpBuf}
	if conn.ObjectEncoding == 0 {
		msg.Encode(&amf)
	} else {
		amf := AMF3{AMF: amf}
		msg.Encode(&amf)
	}
	conn.tmpBuf = amf.Buffer
	head := newChunkHeader(t)
	head.MessageLength = uint32(conn.tmpBuf.Len())
	if sid, ok := msg.(HaveStreamID); ok {
		head.MessageStreamID = sid.GetStreamID()
	}
	head.WriteTo(RTMP_CHUNK_HEAD_12, &conn.chunkHeader)
	for b := conn.tmpBuf; b.Len() > 0; {
		if b.CanReadN(conn.WriteChunkSize) {
			conn.sendChunk(b.ReadN(conn.WriteChunkSize))
		} else {
			conn.sendChunk(b)
			break
		}
	}
	return nil
}

func (conn *NetConnection) sendChunk(writeBuffer ...[]byte) error {
	if n, err := conn.Write(conn.chunkHeader); err != nil {
		return err
	} else {
		conn.writeSeqNum += uint32(n)
	}
	buf := net.Buffers(writeBuffer)
	n, err := buf.WriteTo(conn)
	conn.writeSeqNum += uint32(n)
	return err
}
