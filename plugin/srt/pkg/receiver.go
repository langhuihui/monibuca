package srt

import (
	"bytes"

	srt "github.com/datarhei/gosrt"
	mpegts "m7s.live/v5/pkg/format/ts"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
)

type Receiver struct {
	task.Task
	mpegts.MpegTsStream
	srt.Conn
}

func (r *Receiver) Start() error {
	r.Allocator = util.NewScalableMemoryAllocator(1 << util.MinPowerOf2)
	r.Using(r.Allocator, r.Publisher)
	r.OnStop(r.Conn.Close)
	return nil
}

func (r *Receiver) Run() error {
	for !r.IsStopped() {
		packet, err := r.ReadPacket()
		if err != nil {
			return err
		}
		err = r.Feed(bytes.NewReader(packet.Data()))
		if err != nil {
			return err
		}
	}
	return r.StopReason()
}
