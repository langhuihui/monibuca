package avformat

import (
	"github.com/langhuihui/monibuca/monica/pool"
	"github.com/langhuihui/monibuca/monica/util"
	"io"
)

const (
	// FLV Tag Type
	FLV_TAG_TYPE_AUDIO  = 0x08
	FLV_TAG_TYPE_VIDEO  = 0x09
	FLV_TAG_TYPE_SCRIPT = 0x12
)

var (
	// 音频格式. 4 bit
	SoundFormat = map[byte]string{
		0:  "Linear PCM, platform endian",
		1:  "ADPCM",
		2:  "MP3",
		3:  "Linear PCM, little endian",
		4:  "Nellymoser 16kHz mono",
		5:  "Nellymoser 8kHz mono",
		6:  "Nellymoser",
		7:  "G.711 A-law logarithmic PCM",
		8:  "G.711 mu-law logarithmic PCM",
		9:  "reserved",
		10: "AAC",
		11: "Speex",
		14: "MP3 8Khz",
		15: "Device-specific sound"}

	// 采样频率. 2 bit
	SoundRate = map[byte]int{
		0: 5500,
		1: 11000,
		2: 22000,
		3: 44000}

	// 量化精度. 1 bit
	SoundSize = map[byte]string{
		0: "8Bit",
		1: "16Bit"}

	// 音频类型. 1bit
	SoundType = map[byte]string{
		0: "Mono",
		1: "Stereo"}

	// 视频帧类型. 4bit
	FrameType = map[byte]string{
		1: "keyframe (for AVC, a seekable frame)",
		2: "inter frame (for AVC, a non-seekable frame)",
		3: "disposable inter frame (H.263 only)",
		4: "generated keyframe (reserved for server use only)",
		5: "video info/command frame"}

	// 视频编码类型. 4bit
	CodecID = map[byte]string{
		1:  "JPEG (currently unused)",
		2:  "Sorenson H.263",
		3:  "Screen video",
		4:  "On2 VP6",
		5:  "On2 VP6 with alpha channel",
		6:  "Screen video version 2",
		7:  "AVC",
		12: "H265"}
)

var FLVHeader = []byte{0x46, 0x4c, 0x56, 0x01, 0x05, 0, 0, 0, 9, 0, 0, 0, 0}

func WriteFLVTag(w io.Writer, tag *pool.SendPacket) (err error) {
	head := pool.GetSlice(11)
	defer pool.RecycleSlice(head)
	tail := pool.GetSlice(4)
	defer pool.RecycleSlice(tail)
	head[0] = tag.Packet.Type
	dataSize := uint32(len(tag.Packet.Payload))
	util.BigEndian.PutUint32(tail, dataSize+11)
	util.BigEndian.PutUint24(head[1:], dataSize)
	util.BigEndian.PutUint24(head[4:], tag.Timestamp)
	util.BigEndian.PutUint32(head[7:], 0)
	if _, err = w.Write(head); err != nil {
		return
	}
	// Tag Data
	if _, err = w.Write(tag.Packet.Payload); err != nil {
		return
	}
	if _, err = w.Write(tail); err != nil { // PreviousTagSizeN(4)
		return
	}
	return
}
func ReadFLVTag(r io.Reader) (tag *pool.AVPacket, err error) {
	head := pool.GetSlice(11)
	defer pool.RecycleSlice(head)
	if _, err = io.ReadFull(r, head); err != nil {
		return
	}
	tag = pool.NewAVPacket(head[0])
	dataSize := util.BigEndian.Uint24(head[1:])
	tag.Timestamp = util.BigEndian.Uint24(head[4:])
	body := pool.GetSlice(int(dataSize))
	defer pool.RecycleSlice(body)
	if _, err = io.ReadFull(r, body); err == nil {
		tag.Payload = body
		t := pool.GetSlice(4)
		_, err = io.ReadFull(r, t)
		pool.RecycleSlice(t)
	}
	return
}
