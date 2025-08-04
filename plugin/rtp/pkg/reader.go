package rtp

import (
	"errors"
	"fmt"
	"io"

	"github.com/pion/rtp"
	"m7s.live/v5/pkg/util"
)

type IRTPReader interface {
	Read(packet *rtp.Packet) (err error)
}

type RTPUDPReader struct {
	io.Reader
	buf [MTUSize]byte
}

func NewRTPUDPReader(r io.Reader) *RTPUDPReader {
	return &RTPUDPReader{Reader: r}
}

func (r *RTPUDPReader) Read(packet *rtp.Packet) (err error) {
	n, err := r.Reader.Read(r.buf[:])
	if err != nil {
		return err
	}
	return packet.Unmarshal(r.buf[:n])
}

type RTPTCPReader struct {
	*util.BufReader
	buffer util.Buffer
}

func NewRTPTCPReader(r io.Reader) *RTPTCPReader {
	return &RTPTCPReader{BufReader: util.NewBufReader(r)}
}

func (r *RTPTCPReader) Read(packet *rtp.Packet) (err error) {
	var rtplen uint32
	var b0, b1 byte
	rtplen, err = r.ReadBE32(2)
	if err != nil {
		return
	}
	var mem util.Memory
	mem, err = r.ReadBytes(int(rtplen))
	if err != nil {
		return
	}
	mr := mem.NewReader()
	mr.ReadByteTo(&b0, &b1)
	if b0>>6 != 2 || b0&0x0f > 15 || b1&0x7f > 127 {
		// TODO:
		panic(fmt.Errorf("invalid rtp packet: %x", r.buffer[:2]))
	} else {
		r.buffer.Relloc(int(rtplen))
		mem.CopyTo(r.buffer)
		err = packet.Unmarshal(r.buffer)
	}
	return
}

type RTPPayloadReader struct {
	IRTPReader
	rtp.Packet
	SSRC   uint32 // RTP SSRC
	buffer util.MemoryReader
}

// func NewTCPRTPPayloadReaderForFeed() *RTPPayloadReader {
// 	r := &RTPPayloadReader{}
// 	r.IRTPReader = &RTPTCPReader{
// 		BufReader: util.NewBufReaderChan(10),
// 	}
// 	r.buffer.Memory = &util.Memory{}
// 	return r
// }

func NewRTPPayloadReader(t IRTPReader) *RTPPayloadReader {
	r := &RTPPayloadReader{}
	r.IRTPReader = t
	r.buffer.Memory = &util.Memory{}
	return r
}

func (r *RTPPayloadReader) Read(buf []byte) (n int, err error) {
	// 如果缓冲区中有数据，先读取缓冲区中的数据
	if r.buffer.Length > 0 {
		n, _ = r.buffer.Read(buf)
		return n, nil
	}

	// 读取新的RTP包
	for {
		lastSeq := r.SequenceNumber
		err = r.IRTPReader.Read(&r.Packet)
		if err != nil {
			err = errors.Join(err, fmt.Errorf("failed to read RTP packet"))
			return
		}

		// 检查SSRC是否匹配
		if r.SSRC != 0 && r.SSRC != r.Packet.SSRC {
			// SSRC不匹配，继续读取下一个包
			continue
		}

		// 检查序列号是否连续
		if lastSeq == 0 || r.SequenceNumber == lastSeq+1 {
			// 序列号连续，处理当前包的数据
			if lbuf, lpayload := len(buf), len(r.Payload); lbuf >= lpayload {
				// 缓冲区足够大，可以容纳整个负载
				copy(buf, r.Payload)
				n += lpayload

				// 如果缓冲区还有剩余空间，继续读取下一个包
				if lbuf > lpayload {
					var nextn int
					nextn, err = r.Read(buf[lpayload:])
					if err != nil && err != io.EOF {
						return n, err
					}
					n += nextn
				}
				return
			} else {
				// 缓冲区不够大，只复制部分数据，将剩余数据放入缓冲区
				n += lbuf
				copy(buf, r.Payload[:lbuf])
				r.buffer.PushOne(r.Payload[lbuf:])
				r.buffer.Length = lpayload - lbuf
				return
			}
		}
	}
}
