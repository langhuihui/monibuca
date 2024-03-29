package demo

import (
	"time"

	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/util"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
)

type AnnexB struct {
	PTS time.Duration
	DTS time.Duration
	util.RecyclableMemory
}

// DecodeConfig implements pkg.IAVFrame.
func (a *AnnexB) DecodeConfig(*pkg.AVTrack) error {
	panic("unimplemented")
}

// FromRaw implements pkg.IAVFrame.
func (a *AnnexB) FromRaw(t *pkg.AVTrack, raw any) error {
	var nalus = raw.(pkg.Nalus)
	a.PTS = nalus.PTS
	a.DTS = nalus.DTS

	return nil
}

// GetTimestamp implements pkg.IAVFrame.
func (a *AnnexB) GetTimestamp() time.Duration {
	return a.DTS / 90
}

// IsIDR implements pkg.IAVFrame.
func (a *AnnexB) IsIDR() bool {
	return false
}

// ToRaw implements pkg.IAVFrame.
func (a *AnnexB) ToRaw(*pkg.AVTrack) (any, error) {
	return a.Data, nil
}

type DemoPlugin struct {
	m7s.Plugin
}

func (p *DemoPlugin) OnInit() {
	publisher, err := p.Publish("live/demo")
	if err == nil {
		var annexB AnnexB
		publisher.WriteVideo(&annexB)
	}
}

func (p *DemoPlugin) OnPublish(publisher *m7s.Publisher) {
	subscriber, err := p.Subscribe(publisher.StreamPath)
	if err == nil {
		go subscriber.Handle(nil, func(v *rtmp.RTMPVideo) {

		})
	}
}

var _ = m7s.InstallPlugin[*DemoPlugin]()
