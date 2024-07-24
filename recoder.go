package m7s

import (
	"m7s.live/m7s/v5/pkg/config"
	"os"
)

type RecordHandler interface {
	Close()
	Record(*Recorder) error
}

type Recorder struct {
	File *os.File
	Subscriber
	config.Record
}

func (p *Recorder) GetKey() string {
	return p.File.Name()
}

func (p *Recorder) Start(handler RecordHandler) (err error) {
	defer handler.Close()
	return handler.Record(p)
}
