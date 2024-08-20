package gb28181

import (
	"github.com/pion/rtp"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/util"
	rtp2 "m7s.live/m7s/v5/plugin/rtp/pkg"
	"os"
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

type Receiver struct {
	*m7s.Publisher
	rtp.Packet
	*util.BufReader
	FeedChan  chan []byte
	psm       util.Memory
	dump      *os.File
	dumpLen   []byte
	psVideo   PSVideo
	psAudio   PSAudio
	RTPReader *rtp2.TCP
}

func NewReceiver(puber *m7s.Publisher) *Receiver {
	ret := &Receiver{
		Publisher: puber,
		FeedChan:  make(chan []byte),
	}
	ret.BufReader = util.NewBufReaderChan(ret.FeedChan)
	ret.psVideo.SetAllocator(ret.Allocator)
	ret.psAudio.SetAllocator(ret.Allocator)
	return ret
}

func (p *Receiver) ReadPayload() (payload util.Memory, err error) {
	payloadlen, err := p.ReadBE(2)
	if err != nil {
		return
	}
	return p.ReadBytes(payloadlen)
}

func (p *Receiver) Demux() {
	var payload util.Memory
	defer p.Info("demux exit")
	for {
		code, err := p.ReadBE32(4)
		if err != nil {
			return
		}
		p.Debug("demux", "code", code)
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
			annexB, err = p.psVideo.parsePESPacket(payload)
			if annexB != nil {
				err = p.WriteVideo(annexB)
			}
		case StartCodeAudio:
			payload, err = p.ReadPayload()
			var audioFrame pkg.IAVFrame
			audioFrame, err = p.psAudio.parsePESPacket(payload)
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

func (dec *Receiver) decProgramStreamMap() (err error) {
	dec.psm, err = dec.ReadPayload()
	if err != nil {
		return err
	}
	var programStreamInfoLen, programStreamMapLen, elementaryStreamInfoLength uint32
	var streamType, elementaryStreamID byte
	reader := dec.psm.NewReader()
	reader.Skip(2)
	programStreamInfoLen, err = reader.ReadBE(2)
	reader.Skip(int(programStreamInfoLen))
	programStreamMapLen, err = reader.ReadBE(2)
	for programStreamMapLen > 0 {
		streamType, err = reader.ReadByte()
		elementaryStreamID, err = reader.ReadByte()
		if elementaryStreamID >= 0xe0 && elementaryStreamID <= 0xef {
			dec.psVideo.streamType = streamType
		} else if elementaryStreamID >= 0xc0 && elementaryStreamID <= 0xdf {
			dec.psAudio.streamType = streamType
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
	p.FeedChan <- p.Payload
	return
}

func (p *Receiver) Dispose() {
	p.RTPReader.Close()
	//close(p.FeedChan)
}

func (p *Receiver) Go() (err error) {
	return p.RTPReader.Read(p.ReadRTP)
}
