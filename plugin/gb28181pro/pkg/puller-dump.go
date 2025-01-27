package gb28181

import (
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg/util"
)

type DumpPuller struct {
	m7s.HTTPFilePuller
}

func (p *DumpPuller) Start() (err error) {
	p.PullJob.PublishConfig.PubType = m7s.PublishTypeReplay
	return p.HTTPFilePuller.Start()
}

func (p *DumpPuller) Run() (err error) {
	pub := p.PullJob.Publisher
	puber := NewPSPublisher(pub)
	puber.Receiver.Logger = p.Logger
	go puber.Demux()
	var t uint16
	defer close(puber.Receiver.FeedChan)
	for l := make([]byte, 6); pub.State != m7s.PublisherStateDisposed; time.Sleep(time.Millisecond * time.Duration(t)) {
		_, err = p.Read(l)
		if err != nil {
			return
		}
		payloadLen := util.ReadBE[int](l[:4])
		payload := make([]byte, payloadLen)
		t = util.ReadBE[uint16](l[4:])
		_, err = p.Read(payload)
		if err != nil {
			return
		}
		if err = puber.Receiver.ReadRTP(payload); err != nil {
			p.Error("replayPS", "err", err)
		}
		if pub.IsStopped() {
			return pub.StopReason()
		}
	}
	return
}
