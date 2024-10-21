package flv

import (
	"bufio"
	"io"
	"net"

	"m7s.live/v5/pkg/util"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
)

const (
	// FLV Tag Type
	FLV_TAG_TYPE_AUDIO  = 0x08
	FLV_TAG_TYPE_VIDEO  = 0x09
	FLV_TAG_TYPE_SCRIPT = 0x12
)

var FLVHead = []byte{'F', 'L', 'V', 0x01, 0x05, 0, 0, 0, 9, 0, 0, 0, 0}

type FlvWriter struct {
	io.Writer
	buf [15]byte
}

func NewFlvWriter(w io.Writer) *FlvWriter {
	return &FlvWriter{Writer: w}
}

func (w *FlvWriter) WriteHeader(hasAudio, hasVideo bool) (err error) {
	var flags byte
	if hasAudio {
		flags |= 0x04
	}
	if hasVideo {
		flags |= 0x01
	}
	_, err = w.Write([]byte{'F', 'L', 'V', 0x01, flags, 0, 0, 0, 9, 0, 0, 0, 0})
	return
}

func (w *FlvWriter) WriteTag(t byte, ts, dataSize uint32, payload ...[]byte) (err error) {
	WriteFLVTagHead(t, ts, dataSize, w.buf[:])
	var buffers net.Buffers = append(append(net.Buffers{w.buf[:11]}, payload...), util.PutBE(w.buf[11:], dataSize+11))
	_, err = buffers.WriteTo(w)
	return
}

//func AVCC2FLV(t byte, ts uint32, avcc ...[]byte) (flv net.Buffers) {
//	b := util.Buffer(make([]byte, 0, 15))
//	b.WriteByte(t)
//	dataSize := util.SizeOfBuffers(avcc)
//	b.WriteUint24(uint32(dataSize))
//	b.WriteUint24(ts)
//	b.WriteByte(byte(ts >> 24))
//	b.WriteUint24(0)
//	return append(append(append(flv, b), avcc...), util.PutBE(b.Malloc(4), dataSize+11))
//}

func PutFlvTimestamp(header []byte, timestamp uint32) {
	header[4] = byte(timestamp >> 16)
	header[5] = byte(timestamp >> 8)
	header[6] = byte(timestamp)
	header[7] = byte(timestamp >> 24)
}

func WriteFLVTagHead(t uint8, ts, dataSize uint32, b []byte) {
	b[0] = t
	b[1], b[2], b[3] = byte(dataSize>>16), byte(dataSize>>8), byte(dataSize)
	PutFlvTimestamp(b, ts)
}

//func WriteFLVTag(w io.Writer, t byte, timestamp uint32, payload ...[]byte) (n int64, err error) {
//	buffers := AVCC2FLV(t, timestamp, payload...)
//	return buffers.WriteTo(w)
//}

func ReadMetaData(reader io.Reader) (metaData rtmp.EcmaArray, err error) {
	r := bufio.NewReader(reader)
	_, err = r.Discard(13)
	tagHead := make(util.Buffer, 11)
	_, err = io.ReadFull(r, tagHead)
	if err != nil {
		return
	}
	tmp := tagHead
	t := tmp.ReadByte()
	dataLen := tmp.ReadUint24()
	_, err = r.Discard(4)
	if t == FLV_TAG_TYPE_SCRIPT {
		data := make([]byte, dataLen+4)
		_, err = io.ReadFull(reader, data)
		amf := &rtmp.AMF{
			Buffer: util.Buffer(data[1+2+len("onMetaData") : len(data)-4]),
		}
		var obj any
		obj, err = amf.Unmarshal()
		metaData = obj.(rtmp.EcmaArray)
	}
	return
}
