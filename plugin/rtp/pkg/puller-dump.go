package rtp

import (
	"time"

	m7s "m7s.live/v5"
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
	var receiver PSReceiver
	receiver.Publisher = pub
	receiver.StreamMode = StreamModeManual
	receiver.OnStart(func() {
		go func() {
			var t uint16
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
				select {
				case receiver.RTPMouth <- payload:
				case <-pub.Done():
					return
				}
			}
		}()
	})
	return p.RunTask(&receiver)
}
