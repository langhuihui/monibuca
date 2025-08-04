package rtp

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestRTPPayloadReaderDebug(t *testing.T) {
	// 创建简单的测试数据
	originalData := []byte("Hello World")

	// 生成RTP包
	packets := generateRTPPacketsForDebug(originalData, 0, 1000)

	fmt.Printf("Generated %d RTP packets\n", len(packets))
	for i, packet := range packets {
		fmt.Printf("Packet %d: Seq=%d, Payload=%s, PayloadLen=%d\n", i, packet.SequenceNumber, string(packet.Payload), len(packet.Payload))
	}

	// 将RTP包序列化到缓冲区
	var buf bytes.Buffer
	for _, packet := range packets {
		data, err := packet.Marshal()
		assert.NoError(t, err)
		fmt.Printf("Marshaled packet length: %d\n", len(data))

		// 写入RTP包长度和数据
		buf.Write([]byte{byte(len(data) >> 8), byte(len(data))})
		buf.Write(data)
	}

	fmt.Printf("Buffer size: %d\n", buf.Len())
	fmt.Printf("Original data length: %d\n", len(originalData))
	fmt.Printf("Original data: %s\n", string(originalData))

	// 使用RTPPayloadReader读取数据
	reader := NewRTPPayloadReader(NewRTPTCPReader(&buf))

	// 逐步读取数据
	allData := make([]byte, 0)
	bufSize := 3

	for len(allData) < len(originalData) {
		result := make([]byte, bufSize)
		fmt.Printf("Buffer length before read: %d\n", reader.buffer.Length)
		fmt.Printf("Buffer count before read: %d\n", reader.buffer.Count())
		n, err := reader.Read(result)
		fmt.Printf("Read returned: n=%d, err=%v\n", n, err)
		fmt.Printf("Read data: %s\n", string(result[:n]))
		fmt.Printf("Buffer length after read: %d\n", reader.buffer.Length)
		fmt.Printf("Buffer count after read: %d\n", reader.buffer.Count())

		if err != nil {
			if err == io.EOF {
				break
			}
			assert.NoError(t, err)
		}
		if n == 0 {
			break
		}
		allData = append(allData, result[:n]...)
		fmt.Printf("All data so far: %s\n", string(allData))
	}

	fmt.Printf("Final data length: %d\n", len(allData))
	fmt.Printf("Final data: %s\n", string(allData))

	// 验证数据是否匹配
	assert.Equal(t, originalData, allData)
}

func generateRTPPacketsForDebug(data []byte, ssrc uint32, initialSeq uint16) []*rtp.Packet {
	var packets []*rtp.Packet
	seq := initialSeq
	maxPayloadSize := 100

	for len(data) > 0 {
		// 确定当前包的负载大小
		payloadSize := maxPayloadSize
		if len(data) < payloadSize {
			payloadSize = len(data)
		}

		// 创建RTP包
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				Padding:        false,
				Extension:      false,
				Marker:         false,
				PayloadType:    96,
				SequenceNumber: seq,
				Timestamp:      123456,
				SSRC:           ssrc,
			},
			Payload: data[:payloadSize],
		}

		packets = append(packets, packet)

		// 更新数据和序列号
		data = data[payloadSize:]
		seq++
	}

	return packets
}
