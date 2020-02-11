package cluster

import (
	"bufio"
	"encoding/binary"
	"io"
	"log"
	"net"
	"strings"

	. "github.com/langhuihui/monibuca/monica"
	"github.com/langhuihui/monibuca/monica/avformat"
	"github.com/langhuihui/monibuca/monica/pool"
)

type Receiver struct {
	InputStream
	io.Reader
	*bufio.Writer
}

func (p *Receiver) Auth(authSub *OutputStream) {
	p.WriteByte(MSG_AUTH)
	p.WriteString(authSub.ID + "," + authSub.Sign)
	p.WriteByte(0)
	p.Flush()
}

func (p *Receiver) readAVPacket(avType byte) (av *avformat.AVPacket, err error) {
	buf := pool.GetSlice(4)
	defer pool.RecycleSlice(buf)
	_, err = io.ReadFull(p, buf)
	if err != nil {
		println(err.Error())
		return
	}
	av = avformat.NewAVPacket(avType)
	av.Timestamp = binary.BigEndian.Uint32(buf)
	_, err = io.ReadFull(p, buf)
	if MayBeError(err) {
		return
	}
	av.Payload = pool.GetSlice(int(binary.BigEndian.Uint32(buf)))
	_, err = io.ReadFull(p, av.Payload)
	MayBeError(err)
	return
}

func PullUpStream(streamPath string) {
	addr, err := net.ResolveTCPAddr("tcp", config.Master)
	if MayBeError(err) {
		return
	}
	conn, err := net.DialTCP("tcp", nil, addr)
	if MayBeError(err) {
		return
	}
	brw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	p := &Receiver{
		Reader: brw.Reader,
		Writer: brw.Writer,
	}
	if p.Publish(streamPath, p) {
		p.WriteByte(MSG_SUBSCRIBE)
		p.WriteString(streamPath)
		p.WriteByte(0)
		p.Flush()
		for _, v := range p.Subscribers {
			p.Auth(v)
		}
	} else {
		return
	}
	defer p.Cancel()
	for cmd, err := brw.ReadByte(); !MayBeError(err); cmd, err = brw.ReadByte() {
		switch cmd {
		case MSG_AUDIO:
			if audio, err := p.readAVPacket(avformat.FLV_TAG_TYPE_AUDIO); err == nil {
				p.PushAudio(audio)
			}
		case MSG_VIDEO:
			if video, err := p.readAVPacket(avformat.FLV_TAG_TYPE_VIDEO); err == nil && len(video.Payload) > 2 {
				tmp := video.Payload[0]         // 第一个字节保存着视频的相关信息.
				video.VideoFrameType = tmp >> 4 // 帧类型 4Bit, H264一般为1或者2
				p.PushVideo(video)
			}
		case MSG_AUTH:
			cmd, err = brw.ReadByte()
			if MayBeError(err) {
				return
			}
			bytes, err := brw.ReadBytes(0)
			if MayBeError(err) {
				return
			}
			subId := strings.Split(string(bytes[0:len(bytes)-1]), ",")[0]
			if v, ok := p.Subscribers[subId]; ok {
				if cmd != 1 {
					v.Cancel()
				}
			}
		default:
			log.Printf("unknown cmd:%v", cmd)
		}
	}
}
