package rtp

import (
	"bytes"
	"io"
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/assert"
)

func TestRTPPayloadReader(t *testing.T) {
	// 创建测试数据
	originalData := []byte("Hello, World! This is a test payload for RTP packets.")

	// 生成RTP包
	packets := generateRTPPackets(originalData, 0, 1000)

	// 将RTP包序列化到缓冲区
	var buf bytes.Buffer
	for _, packet := range packets {
		data, err := packet.Marshal()
		assert.NoError(t, err)

		// 写入RTP包长度和数据
		buf.Write([]byte{byte(len(data) >> 8), byte(len(data))})
		buf.Write(data)
	}

	// 使用RTPPayloadReader读取数据
	reader := NewRTPPayloadReader(NewRTPTCPReader(&buf))

	// 读取所有数据
	result := make([]byte, len(originalData))
	n, err := reader.Read(result)
	assert.NoError(t, err)
	assert.Equal(t, len(originalData), n)

	// 验证数据是否匹配
	assert.Equal(t, originalData, result)
}

func TestRTPPayloadReaderWithBuffer(t *testing.T) {
	// 创建测试数据
	originalData := []byte("This is a longer test payload that will be split across multiple RTP packets to test the buffering functionality of the RTPPayloadReader.")

	// 生成RTP包
	packets := generateRTPPackets(originalData, 0, 2000)

	// 将RTP包序列化到缓冲区
	var buf bytes.Buffer
	for _, packet := range packets {
		data, err := packet.Marshal()
		assert.NoError(t, err)

		// 写入RTP包长度和数据
		buf.Write([]byte{byte(len(data) >> 8), byte(len(data))})
		buf.Write(data)
	}

	// 使用RTPPayloadReader读取数据
	reader := NewRTPPayloadReader(NewRTPTCPReader(&buf))

	// 使用较小的缓冲区读取数据
	allData := make([]byte, 0)
	bufSize := 10

	for len(allData) < len(originalData) {
		result := make([]byte, bufSize)
		n, err := reader.Read(result)
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
	}

	// 验证数据是否匹配
	assert.Equal(t, originalData, allData)
}

func TestRTPPayloadReaderSimple(t *testing.T) {
	// 创建简单的测试数据
	originalData := []byte("Hello World")

	// 生成RTP包
	packets := generateRTPPackets(originalData, 0, 1000)

	// 将RTP包序列化到缓冲区
	var buf bytes.Buffer
	for _, packet := range packets {
		data, err := packet.Marshal()
		assert.NoError(t, err)

		// 写入RTP包长度和数据
		buf.Write([]byte{byte(len(data) >> 8), byte(len(data))})
		buf.Write(data)
	}

	// 使用RTPPayloadReader读取数据
	reader := NewRTPPayloadReader(NewRTPTCPReader(&buf))

	// 读取所有数据
	result := make([]byte, len(originalData))
	n, err := reader.Read(result)
	assert.NoError(t, err)
	assert.Equal(t, len(originalData), n)

	// 验证数据是否匹配
	assert.Equal(t, originalData, result)
}

func generateRTPPackets(data []byte, ssrc uint32, initialSeq uint16) []*rtp.Packet {
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
