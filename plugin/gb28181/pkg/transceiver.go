package gb28181

import (
	"net"
	"os"

	"github.com/pion/rtp"
	"m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	rtp2 "m7s.live/v5/plugin/rtp/pkg"
)

const (
	StartCodePS        = 0x000001ba
	StartCodeSYS       = 0x000001bb
	StartCodeMAP       = 0x000001bc
	StartCodeVideo     = 0x000001e0
	StartCodeAudio     = 0x000001c0
	PrivateStreamCode  = 0x000001bd
	MEPGProgramEndCode = 0x000001b9
)

type PSPublisher struct {
	*m7s.Publisher
	*util.BufReader
	Receiver Receiver
}

type Receiver struct {
	task.Task
	rtp.Packet
	FeedChan   chan []byte
	psm        util.Memory
	dump       *os.File
	dumpLen    []byte
	psVideo    PSVideo
	psAudio    PSAudio
	RTPReader  *rtp2.TCP
	ListenAddr string
	listener   net.Listener
}

func NewPSPublisher(puber *m7s.Publisher) *PSPublisher {
	ret := &PSPublisher{
		Publisher: puber,
	}
	ret.Receiver.FeedChan = make(chan []byte, 10)
	ret.BufReader = util.NewBufReaderChan(ret.Receiver.FeedChan)
	ret.Receiver.psVideo.SetAllocator(ret.Allocator)
	ret.Receiver.psAudio.SetAllocator(ret.Allocator)
	return ret
}

func (p *PSPublisher) ReadPayload() (payload util.Memory, err error) {
	payloadlen, err := p.ReadBE(2)
	if err != nil {
		return
	}
	return p.ReadBytes(payloadlen)
}

func (p *PSPublisher) Demux() {
	var payload util.Memory
	defer p.Info("demux exit")
	for {
		code, err := p.ReadBE32(4)
		if err != nil {
			return
		}
		p.Trace("demux", "code", code)
		switch code {
		case StartCodePS:
			var psl byte
			if err = p.Skip(9); err != nil {
				return
			}
			psl, err = p.ReadByte()
			if err != nil {
				return
			}
			psl &= 0x07
			if err = p.Skip(int(psl)); err != nil {
				return
			}
		case StartCodeVideo:
			payload, err = p.ReadPayload()
			var annexB *pkg.AnnexB
			annexB, err = p.Receiver.psVideo.parsePESPacket(payload)
			if annexB != nil {
				err = p.WriteVideo(annexB)
			}
		case StartCodeAudio:
			payload, err = p.ReadPayload()
			var audioFrame pkg.IAVFrame
			audioFrame, err = p.Receiver.psAudio.parsePESPacket(payload)
			if audioFrame != nil {
				err = p.WriteAudio(audioFrame)
			}
		case StartCodeMAP:
			p.decProgramStreamMap()
		default:
			p.ReadPayload()
		}
	}
}

func (dec *PSPublisher) decProgramStreamMap() (err error) {
	dec.Receiver.psm, err = dec.ReadPayload()
	if err != nil {
		return err
	}
	var programStreamInfoLen, programStreamMapLen, elementaryStreamInfoLength uint32
	var streamType, elementaryStreamID byte
	reader := dec.Receiver.psm.NewReader()
	reader.Skip(2)
	programStreamInfoLen, err = reader.ReadBE(2)
	reader.Skip(int(programStreamInfoLen))
	programStreamMapLen, err = reader.ReadBE(2)
	for programStreamMapLen > 0 {
		streamType, err = reader.ReadByte()
		elementaryStreamID, err = reader.ReadByte()
		if elementaryStreamID >= 0xe0 && elementaryStreamID <= 0xef {
			dec.Receiver.psVideo.streamType = streamType
		} else if elementaryStreamID >= 0xc0 && elementaryStreamID <= 0xdf {
			dec.Receiver.psAudio.streamType = streamType
		}
		elementaryStreamInfoLength, err = reader.ReadBE(2)
		reader.Skip(int(elementaryStreamInfoLength))
		programStreamMapLen -= 4 + elementaryStreamInfoLength
	}
	return nil
}

func (p *Receiver) ReadRTP(rtp util.Buffer) (err error) {
	if err = p.Unmarshal(rtp); err != nil {
		return
	}
	copyData := make([]byte, len(p.Payload))
	copy(copyData, p.Payload)
	p.FeedChan <- copyData
	return
}

func (p *Receiver) Start() (err error) {
	p.listener, err = net.Listen("tcp", p.ListenAddr)
	if err != nil {
		p.Error("start listen", "err", err)
		return
	}
	p.Info("start listen", "addr", p.ListenAddr)
	return
}

func (p *Receiver) Dispose() {
	p.listener.Close()
	if p.RTPReader != nil {
		p.RTPReader.Close()
	}
	//close(p.FeedChan)
}

func (p *Receiver) Go() error {
	p.Info("start accept")
	conn, err := p.listener.Accept()
	if err != nil {
		p.Error("accept", "err", err)
		return err
	}
	p.RTPReader = (*rtp2.TCP)(conn.(*net.TCPConn))
	p.Info("accept", "addr", conn.RemoteAddr())
	return p.RTPReader.Read(p.ReadRTP)
}
