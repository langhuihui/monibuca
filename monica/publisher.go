package monica

import (
	"log"
	"reflect"
	"time"
)

type Publisher interface {
	OnClosed()
}

type InputStream struct {
	*Room
}

func (p *InputStream) Close() {
	if p.Running() {
		p.Cancel()
	}
}
func (p *InputStream) Running() bool {
	return p.Room != nil && p.Err() == nil
}
func (p *InputStream) OnClosed() {
}
func (p *InputStream) Publish(streamPath string, publisher Publisher) bool {
	p.Room = AllRoom.Get(streamPath)
	if p.Publisher != nil {
		return false
	}
	p.Publisher = publisher
	p.Type = reflect.ValueOf(publisher).Elem().Type().Name()
	log.Printf("publish set :%s", p.Type)
	p.StartTime = time.Now()
	OnPublishHooks.Trigger(p.Room)
	return true
}
