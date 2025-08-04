package rtp

import (
	"fmt"
	"testing"

	"github.com/pion/rtp"
)

func TestForwardConfig(t *testing.T) {
	config := &ForwardConfig{
		Source: ConnectionConfig{
			IP:   "127.0.0.1",
			Port: 8080,
			Mode: StreamModeUDP,
			SSRC: 12345,
		},
		Target: ConnectionConfig{
			IP:   "127.0.0.1",
			Port: 8081,
			Mode: StreamModeTCPActive,
			SSRC: 67890,
		},
		Relay: false,
	}

	if config.Source.IP != "127.0.0.1" {
		t.Errorf("Expected source IP 127.0.0.1, got %s", config.Source.IP)
	}

	if config.Source.Port != 8080 {
		t.Errorf("Expected source port 8080, got %d", config.Source.Port)
	}

	if config.Source.Mode != StreamModeUDP {
		t.Errorf("Expected source mode UDP, got %s", config.Source.Mode)
	}

	if config.Source.SSRC != 12345 {
		t.Errorf("Expected source SSRC 12345, got %d", config.Source.SSRC)
	}

	if config.Target.IP != "127.0.0.1" {
		t.Errorf("Expected target IP 127.0.0.1, got %s", config.Target.IP)
	}

	if config.Target.Port != 8081 {
		t.Errorf("Expected target port 8081, got %d", config.Target.Port)
	}

	if config.Target.Mode != StreamModeTCPActive {
		t.Errorf("Expected target mode TCP-ACTIVE, got %s", config.Target.Mode)
	}

	if config.Target.SSRC != 67890 {
		t.Errorf("Expected target SSRC 67890, got %d", config.Target.SSRC)
	}

	if config.Relay {
		t.Error("Expected relay to be false")
	}
}

func TestNewForwarder(t *testing.T) {
	config := &ForwardConfig{
		Source: ConnectionConfig{
			IP:   "127.0.0.1",
			Port: 8080,
			Mode: StreamModeUDP,
			SSRC: 12345,
		},
		Target: ConnectionConfig{
			IP:   "127.0.0.1",
			Port: 8081,
			Mode: StreamModeTCPActive,
			SSRC: 67890,
		},
		Relay: false,
	}

	forwarder := NewForwarder(config)

	if forwarder.config != config {
		t.Error("Expected forwarder config to match input config")
	}

	if forwarder.source != nil {
		t.Error("Expected source connection to be nil initially")
	}

	if forwarder.target != nil {
		t.Error("Expected target connection to be nil initially")
	}
}

func TestConnectionConfig(t *testing.T) {
	config := ConnectionConfig{
		IP:   "192.168.1.100",
		Port: 9000,
		Mode: StreamModeTCPPassive,
		SSRC: 54321,
	}

	if config.IP != "192.168.1.100" {
		t.Errorf("Expected IP 192.168.1.100, got %s", config.IP)
	}

	if config.Port != 9000 {
		t.Errorf("Expected port 9000, got %d", config.Port)
	}

	if config.Mode != StreamModeTCPPassive {
		t.Errorf("Expected mode TCP-PASSIVE, got %s", config.Mode)
	}

	if config.SSRC != 54321 {
		t.Errorf("Expected SSRC 54321, got %d", config.SSRC)
	}
}

func TestRTPWriter(t *testing.T) {
	// 创建一个模拟的writer
	mockWriter := &mockWriter{}
	writer := NewRTPWriter(mockWriter, StreamModeUDP)

	if writer == nil {
		t.Error("Expected RTPWriter to be created")
	}

	// 测试UDP模式的WriteRaw
	data := []byte{1, 2, 3, 4}
	err := writer.WriteRaw(data)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if len(mockWriter.data) != 1 {
		t.Errorf("Expected 1 write, got %d", len(mockWriter.data))
	}

	if len(mockWriter.data[0]) != 4 {
		t.Errorf("Expected 4 bytes written, got %d", len(mockWriter.data[0]))
	}
}

// mockWriter 用于测试的模拟writer
type mockWriter struct {
	data [][]byte
}

func (w *mockWriter) Write(data []byte) (int, error) {
	w.data = append(w.data, append([]byte{}, data...))
	return len(data), nil
}

func TestRelayProcessor(t *testing.T) {
	// 创建模拟的reader和writer
	mockReader := &mockReader{data: [][]byte{{1, 2, 3}, {4, 5, 6}}}
	mockWriter := &mockWriter{}

	processor := NewRelayProcessor(mockReader, mockWriter, StreamModeUDP, StreamModeTCPActive)

	if processor.reader != mockReader {
		t.Error("Expected reader to match input")
	}

	if processor.writer != mockWriter {
		t.Error("Expected writer to match input")
	}

	if processor.sourceMode != StreamModeUDP {
		t.Errorf("Expected source mode UDP, got %s", processor.sourceMode)
	}

	if processor.targetMode != StreamModeTCPActive {
		t.Errorf("Expected target mode TCP-ACTIVE, got %s", processor.targetMode)
	}
}

// mockReader 用于测试的模拟reader
type mockReader struct {
	data [][]byte
	pos  int
}

func (r *mockReader) Read(buf []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, nil // EOF
	}

	data := r.data[r.pos]
	r.pos++

	copy(buf, data)
	return len(data), nil
}

func TestConnectionTypes(t *testing.T) {
	// 测试ConnectionConfig
	config := ConnectionConfig{
		IP:   "127.0.0.1",
		Port: 8080,
		Mode: StreamModeUDP,
		SSRC: 12345,
	}

	if config.Mode != StreamModeUDP {
		t.Errorf("Expected mode UDP, got %s", config.Mode)
	}

	if config.SSRC != 12345 {
		t.Errorf("Expected SSRC 12345, got %d", config.SSRC)
	}
}

func TestConnectionDirection(t *testing.T) {
	// 测试连接方向的概念
	config := &ForwardConfig{
		Source: ConnectionConfig{
			IP:   "127.0.0.1",
			Port: 8080,
			Mode: StreamModeUDP,
			SSRC: 12345,
		},
		Target: ConnectionConfig{
			IP:   "127.0.0.1",
			Port: 8081,
			Mode: StreamModeTCPActive,
			SSRC: 67890,
		},
		Relay: false,
	}

	forwarder := NewForwarder(config)

	// 验证配置正确性
	if forwarder.config.Source.SSRC != 12345 {
		t.Errorf("Expected source SSRC 12345, got %d", forwarder.config.Source.SSRC)
	}

	if forwarder.config.Target.SSRC != 67890 {
		t.Errorf("Expected target SSRC 67890, got %d", forwarder.config.Target.SSRC)
	}

	// 验证连接类型
	if forwarder.source != nil {
		t.Error("Expected source connection to be nil initially")
	}

	if forwarder.target != nil {
		t.Error("Expected target connection to be nil initially")
	}
}

func TestBufferReuse(t *testing.T) {
	// 测试RTPWriter的buffer复用
	writer := NewRTPWriter(&mockWriter{}, StreamModeUDP)

	// 多次写入应该复用同一个buffer
	for i := 0; i < 10; i++ {
		packet := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				SequenceNumber: uint16(i),
				Timestamp:      uint32(i * 1000),
				SSRC:           uint32(i),
			},
			Payload: []byte(fmt.Sprintf("test packet %d", i)),
		}

		err := writer.WritePacket(packet)
		if err != nil {
			t.Errorf("WritePacket failed: %v", err)
		}
	}

	// 测试RelayProcessor的buffer复用
	processor := NewRelayProcessor(&mockReader{data: [][]byte{{1, 2, 3}, {4, 5, 6}}}, &mockWriter{}, StreamModeUDP, StreamModeTCPActive)

	// 验证buffer字段存在
	if processor.buffer == nil {
		t.Error("Expected buffer to be initialized")
	}

	if len(processor.buffer) != 1460 {
		t.Errorf("Expected buffer size 1460, got %d", len(processor.buffer))
	}

	if processor.header == nil {
		t.Error("Expected header to be initialized")
	}

	if len(processor.header) != 2 {
		t.Errorf("Expected header size 2, got %d", len(processor.header))
	}
}

func TestRTPProcessorBufferReuse(t *testing.T) {
	// 测试RTPProcessor的buffer复用
	config := &ForwardConfig{
		Source: ConnectionConfig{
			IP:   "127.0.0.1",
			Port: 8080,
			Mode: StreamModeUDP,
			SSRC: 12345,
		},
		Target: ConnectionConfig{
			IP:   "127.0.0.1",
			Port: 8081,
			Mode: StreamModeTCPActive,
			SSRC: 67890,
		},
		Relay: false,
	}

	processor := NewRTPProcessor(nil, nil, config)

	// 验证sendBuffer字段存在
	if processor.sendBuffer == nil {
		t.Error("Expected sendBuffer to be initialized")
	}
}
