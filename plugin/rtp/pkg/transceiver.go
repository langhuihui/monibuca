package rtp

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/pion/rtp"
	mpegps "m7s.live/v5/pkg/format/ps"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
)

var ErrRTPReceiveLost = errors.New("rtp receive lost")

// 数据流传输模式（UDP:udp传输、TCP-ACTIVE：tcp主动模式、TCP-PASSIVE：tcp被动模式、MANUAL：手动模式）
type StreamMode string

const (
	StreamModeUDP        StreamMode = "UDP"
	StreamModeTCPActive  StreamMode = "TCP-ACTIVE"
	StreamModeTCPPassive StreamMode = "TCP-PASSIVE"
	StreamModeManual     StreamMode = "MANUAL"
)

type ChanReader chan []byte

func (r ChanReader) Read(buf []byte) (n int, err error) {
	b, ok := <-r
	if !ok {
		return 0, io.EOF
	}
	copy(buf, b)
	return len(b), nil
}

type RTPChanReader chan []byte

func (r RTPChanReader) Read(packet *rtp.Packet) (err error) {
	b, ok := <-r
	if !ok {
		return io.EOF
	}
	return packet.Unmarshal(b)
}

func (r RTPChanReader) Close() error {
	close(r)
	return nil
}

type Receiver struct {
	task.Task
	*util.BufReader
	ListenAddr string
	net.Listener
	StreamMode StreamMode
	SSRC       uint32 // RTP SSRC
	RTPMouth   chan []byte
}

type PSReceiver struct {
	Receiver
	mpegps.MpegPsDemuxer
}

func (p *PSReceiver) Start() error {
	err := p.Receiver.Start()
	if err == nil {
		p.Using(p.Publisher)
	}
	return err
}

func (p *PSReceiver) Run() error {
	p.MpegPsDemuxer.Allocator = util.NewScalableMemoryAllocator(1 << util.MinPowerOf2)
	p.Using(p.MpegPsDemuxer.Allocator)
	return p.MpegPsDemuxer.Feed(p.BufReader)
}

func (p *Receiver) Start() (err error) {
	var rtpReader *RTPPayloadReader
	switch p.StreamMode {
	case StreamModeTCPActive:
		// TCP主动模式不需要监听，直接返回
		p.Info("TCP-ACTIVE mode, no need to listen")
		addr := p.ListenAddr
		if !strings.Contains(addr, ":") {
			addr = ":" + addr
		}
		if strings.HasPrefix(addr, ":") {
			p.Error("invalid address, missing IP", "addr", addr)
			return fmt.Errorf("invalid address %s, missing IP", addr)
		}
		p.Info("TCP-ACTIVE mode, connecting to device", "addr", addr)
		var conn net.Conn
		conn, err = net.Dial("tcp", addr)
		if err != nil {
			p.Error("connect to device failed", "err", err)
			return err
		}
		p.OnStop(conn.Close)
		rtpReader = NewRTPPayloadReader(NewRTPTCPReader(conn))
		p.BufReader = util.NewBufReader(rtpReader)
	case StreamModeTCPPassive:
		var conn net.Conn
		if p.SSRC == 0 {
			p.Info("start new listener", "addr", p.ListenAddr)
			p.Listener, err = net.Listen("tcp4", p.ListenAddr)
			if err != nil {
				p.Error("start listen", "err", err)
				return errors.New("start listen,err" + err.Error())
			}
			p.OnStop(p.Listener.Close)
			conn, err = p.Accept()
		} else {
			//TODO: 公用监听端口
		}
		if err != nil {
			p.Error("accept", "err", err)
			return err
		}
		p.OnStop(conn.Close)
		rtpReader = NewRTPPayloadReader(NewRTPTCPReader(conn))
		p.BufReader = util.NewBufReader(rtpReader)
	case StreamModeUDP:
		var udpAddr *net.UDPAddr
		udpAddr, err = net.ResolveUDPAddr("udp4", p.ListenAddr)
		if err != nil {
			return
		}
		var conn net.Conn
		conn, err = net.ListenUDP("udp4", udpAddr)
		if err != nil {
			return
		}
		rtpReader = NewRTPPayloadReader(NewRTPUDPReader(conn))
		p.BufReader = util.NewBufReader(rtpReader)
	case StreamModeManual:
		p.RTPMouth = make(chan []byte)
		rtpReader = NewRTPPayloadReader((RTPChanReader)(p.RTPMouth))
		p.BufReader = util.NewBufReader(rtpReader)
	}
	p.Using(rtpReader, p.BufReader)
	return
}
