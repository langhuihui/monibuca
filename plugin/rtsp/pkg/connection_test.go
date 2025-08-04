package rtsp

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/util"
)

func parseRTSPDump(filename string) ([]Packet, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var dump struct {
		Packets []struct {
			Packet    int     `yaml:"packet"`
			Peer      int     `yaml:"peer"`
			Index     int     `yaml:"index"`
			Timestamp float64 `yaml:"timestamp"`
			Data      string  `yaml:"data"`
		} `yaml:"packets"`
	}

	err = yaml.Unmarshal(data, &dump)
	if err != nil {
		return nil, err
	}

	packets := make([]Packet, 0, len(dump.Packets))
	for _, p := range dump.Packets {
		packets = append(packets, Packet{
			Index:     p.Index,
			Peer:      p.Peer,
			Timestamp: p.Timestamp,
			Data:      []byte(p.Data),
		})
	}

	return packets, nil
}

type RTSPMockConn struct {
	packets      []Packet
	currentIndex int
	peer         int
	readDeadline time.Time
	closed       bool
	localAddr    net.Addr
	remoteAddr   net.Addr
}

type Packet struct {
	Index     int
	Timestamp float64
	Peer      int
	Data      []byte
}

func NewRTSPMockConn(dumpFile string, peer int) (*RTSPMockConn, error) {
	// Parse YAML dump file and extract packets
	packets, err := parseRTSPDump(dumpFile)
	if err != nil {
		return nil, err
	}

	return &RTSPMockConn{
		packets:      packets,
		currentIndex: 0,
		peer:         peer,
		localAddr:    &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8554},
		remoteAddr:   &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 49152},
	}, nil
}

// Read implements net.Conn interface
func (c *RTSPMockConn) Read(b []byte) (n int, err error) {
	if c.closed {
		return 0, io.EOF
	}

	if c.currentIndex >= len(c.packets) {
		return 0, io.EOF
	}

	// Check read deadline
	if !c.readDeadline.IsZero() && time.Now().After(c.readDeadline) {
		return 0, os.ErrDeadlineExceeded
	}

	// Find next packet for this peer
	for c.currentIndex < len(c.packets) && c.packets[c.currentIndex].Peer != c.peer {
		c.currentIndex++
	}

	if c.currentIndex >= len(c.packets) {
		return 0, io.EOF
	}

	packet := c.packets[c.currentIndex]

	n = copy(b, packet.Data)
	if n == len(packet.Data) {
		c.currentIndex++
	} else {
		packet.Data = packet.Data[n:]
	}

	return n, nil
}

// Write implements net.Conn interface - just discard data
func (c *RTSPMockConn) Write(b []byte) (n int, err error) {
	if c.closed {
		return 0, io.ErrClosedPipe
	}
	return len(b), nil
}

// Close implements net.Conn interface
func (c *RTSPMockConn) Close() error {
	c.closed = true
	return nil
}

// LocalAddr implements net.Conn interface
func (c *RTSPMockConn) LocalAddr() net.Addr {
	return c.localAddr
}

// RemoteAddr implements net.Conn interface
func (c *RTSPMockConn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

// SetDeadline implements net.Conn interface
func (c *RTSPMockConn) SetDeadline(t time.Time) error {
	c.readDeadline = t
	return nil
}

// SetReadDeadline implements net.Conn interface
func (c *RTSPMockConn) SetReadDeadline(t time.Time) error {
	c.readDeadline = t
	return nil
}

// SetWriteDeadline implements net.Conn interface
func (c *RTSPMockConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func TestNetConnection_Pull(t *testing.T) {
	conn, err := NewRTSPMockConn("/Users/dexter/project/v5/monibuca/example/default/dump/rtsp", 1)
	if err != nil {
		t.Fatal(err)
	}
	client := NewPuller(config.Pull{
		URL: "rtsp://127.0.0.1:8554/dump/test",
	}).(*Client)
	client.NetConnection = &NetConnection{Conn: conn}
	client.BufReader = util.NewBufReader(conn)
	client.URL, _ = url.Parse("rtsp://127.0.0.1:8554/dump/test")
	client.MemoryAllocator = util.NewScalableMemoryAllocator(1 << 12)
	client.Logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	client.Context, client.CancelCauseFunc = context.WithCancelCause(context.Background())
	client.Run()
}
