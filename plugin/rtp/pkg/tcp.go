package rtp

import (
	"bufio"
	"encoding/binary"
	"io"
	"m7s.live/v5/pkg/util"
	"net"
)

type TCP net.TCPConn

func (t *TCP) Read(onRTP func(util.Buffer) error) (err error) {
	reader := bufio.NewReader((*net.TCPConn)(t))
	rtpLenBuf := make([]byte, 4)
	buffer := make(util.Buffer, 1024)
	for err == nil {
		if _, err = io.ReadFull(reader, rtpLenBuf); err != nil {
			return
		}
		rtpLen := int(binary.BigEndian.Uint16(rtpLenBuf[:2]))
		if rtpLenBuf[2]>>6 != 2 || rtpLenBuf[2]&0x0f > 15 || rtpLenBuf[3]&0x7f > 127 { //长度后面正常紧跟 rtp 头，如果不是，说明长度不对，此处并非长度，而是可能之前的 rtp 包不完整导致的，需要往前查找
			buffer.Write(rtpLenBuf)                //已读的数据先写入缓存
			for i := 12; i < buffer.Len()-2; i++ { // 缓存中 rtp 头就不用判断了，跳过 12 字节往后寻找
				if buffer[i]>>6 != 2 || buffer[i]&0x0f > 15 || buffer[i+1]&0x7f > 127 { // 一直找到 rtp 头为止
					continue
				}
				rtpLen = int(binary.BigEndian.Uint16(buffer[i-2 : i])) // rtp 头前面两个字节是长度
				if remain := buffer.Len() - i; remain < rtpLen {       // 缓存中的数据不够一个 rtp 包，继续读取
					copy(buffer, buffer[i:])
					buffer.Relloc(rtpLen)
					if _, err = io.ReadFull(reader, buffer[remain:]); err != nil {
						return
					}
					err = onRTP(buffer)
					break
				} else {
					err = onRTP(buffer.SubBuf(i, rtpLen))
					if err != nil {
						return
					}
					i += rtpLen
					if buffer.Len() > i+1 {
						i += 2
					} else if buffer.Len() > i {
						reader.UnreadByte()
						break
					} else {
						break
					}
					i--
				}
			}
		} else {
			buffer.Relloc(rtpLen)
			copy(buffer, rtpLenBuf[2:])
			if _, err = io.ReadFull(reader, buffer[2:]); err != nil {
				return
			}
			err = onRTP(buffer)
		}
	}
	return
}
