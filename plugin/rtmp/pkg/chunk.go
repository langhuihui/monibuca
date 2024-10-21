package rtmp

import (
	"encoding/binary"

	"m7s.live/v5/pkg/util"
)

// RTMP协议中基本的数据单元称为消息(Message).
// 当RTMP协议在互联网中传输数据的时候,消息会被拆分成更小的单元,称为消息块(Chunk).
// 在网络上传输数据时,消息需要被拆分成较小的数据块,才适合在相应的网络环境上传输.

// 理论上Type 0, 1, 2的Chunk都可以使用Extended Timestamp来传递时间
// Type 3由于严禁携带Extened Timestamp字段.但实际上只有Type 0才需要带此字段.
// 这是因为,对Type 1, 2来说,其时间为一个差值,一般肯定小于0x00FFFFF

// 对于除Audio,Video以外的基它Message,其时间字段都可以是置为0的，似乎没有被用到.
// 只有在发送视频和音频数据时,才需要特别的考虑TimeStamp字段.基本依据是,要以HandShake时为起始点0来计算时间.
// 一般来说,建立一个相对时间,把一个视频帧的TimeStamp特意的在当前时间的基础上延迟3秒,则可以达到缓存的效果

const (
	RTMP_CHUNK_HEAD_12 = 0 << 6 // Chunk Basic Header = (Chunk Type << 6) | Chunk Stream ID.
	RTMP_CHUNK_HEAD_8  = 1 << 6
	RTMP_CHUNK_HEAD_4  = 2 << 6
	RTMP_CHUNK_HEAD_1  = 3 << 6
)

type Chunk struct {
	ChunkHeader
	AVData  RTMPData
	MsgData RtmpMessage
	bufLen  int
}

type ChunkHeader struct {
	ChunkStreamID   uint32 `json:""`
	Timestamp       uint32 `json:""` // 3 byte
	MessageLength   uint32 `json:""` // 3 byte
	MessageTypeID   byte   `json:""` // 1 byte
	MessageStreamID uint32 `json:""` // 4 byte
	// Extended Timestamp (0 or 4 bytes): This field is present in certain
	// circumstances depending on the encoded timestamp or timestamp
	// delta field in the Chunk Message header. See Section 5.3.1.3 for
	// more information
	ExtendTimestamp uint32 `json:",omitempty"` // 标识该字段的数据可忽略
}

func (c *ChunkHeader) SetTimestamp(timestamp uint32) {
	if timestamp >= 0xFFFFFF {
		c.ExtendTimestamp = timestamp
		c.Timestamp = 0xFFFFFF
	} else {
		c.ExtendTimestamp = 0
		c.Timestamp = timestamp
	}
}

// ChunkBasicHeader会决定ChunkMessgaeHeader,ChunkMessgaeHeader有4种(0,3,7,11 Bytes),因此可能有4种头.

// 1  -> ChunkBasicHeader(1) + ChunkMessageHeader(0)
// 4  -> ChunkBasicHeader(1) + ChunkMessageHeader(3)
// 8  -> ChunkBasicHeader(1) + ChunkMessageHeader(7)
// 12 -> ChunkBasicHeader(1) + ChunkMessageHeader(11)

func (h *ChunkHeader) WriteTo(t byte, b *util.Buffer) {
	b.Reset()
	csid := byte(h.ChunkStreamID)
	b.WriteByte(t + csid)

	if t < RTMP_CHUNK_HEAD_1 {
		b.WriteUint24(h.Timestamp)
		if t < RTMP_CHUNK_HEAD_4 {
			b.WriteUint24(h.MessageLength)
			b.WriteByte(h.MessageTypeID)
			if t < RTMP_CHUNK_HEAD_8 {
				binary.LittleEndian.PutUint32(b.Malloc(4), h.MessageStreamID)
			}
		}
	}
	if h.Timestamp == 0xffffff {
		b.WriteUint32(h.ExtendTimestamp)
	}
}

type (
	ChunkHeader8  ChunkHeader
	ChunkHeader12 ChunkHeader
	ChunkHeader1  ChunkHeader
	IChunkHeader  interface {
		WriteTo(*util.Buffer)
	}
)

func (h *ChunkHeader8) WriteTo(b *util.Buffer) {
	(*ChunkHeader)(h).WriteTo(RTMP_CHUNK_HEAD_8, b)
}

func (h *ChunkHeader12) WriteTo(b *util.Buffer) {
	(*ChunkHeader)(h).WriteTo(RTMP_CHUNK_HEAD_12, b)
}

func (h *ChunkHeader1) WriteTo(b *util.Buffer) {
	(*ChunkHeader)(h).WriteTo(RTMP_CHUNK_HEAD_1, b)
}
