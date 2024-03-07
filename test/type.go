package test

import (
	"context"
	"encoding/base64"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	. "m7s.live/engine/v4"
	"m7s.live/engine/v4/config"
	"m7s.live/engine/v4/track"
	"m7s.live/engine/v4/util"
)

var conf UnitTestConfig
var unitTestPlugin = InstallPlugin(&conf)
var spsppsbase64 = "AAABJ2QANKwTGqAoALWgHixLcAAAAAEozgVywA=="
var spspps, _ = base64.RawStdEncoding.DecodeString(spsppsbase64)
var EngineChan = make(chan int, 10)
var WaitEngine sync.WaitGroup

func UseEngine() {
	EngineChan <- 1
	WaitEngine.Wait()
}
func FreeEngine() {
	EngineChan <- -1
}
func init() {
	WaitEngine.Add(1)
	go func() {
		var use = 0
		bg := context.Background()
		var ctx context.Context
		var cancel context.CancelFunc
		for {
			select {
			case delta := <-EngineChan:
				use += delta
				switch use {
				case 1:
					ctx, cancel = context.WithTimeout(bg, time.Second*20)
					go Run(ctx, "config.yaml")
				case 0:
					cancel()
					WaitEngine.Add(1)
				}
			}
		}
	}()
}

type UnitTestConfig struct {
	config.Subscribe
	config.Publish
}

func (t *UnitTestConfig) OnEvent(event any) {
	switch event.(type) {
	case FirstConfig:
		WaitEngine.Done()
	}
}

type UnitTestPublisher struct {
	tb testing.TB
	Publisher
}

type UnitTestSubsciber struct {
	tb testing.TB
	Subscriber
}

func (s *UnitTestSubsciber) OnEvent(event any) {
	switch v := event.(type) {
	case AudioFrame:
	case VideoFrame:
		b := v.AUList.ToBytes()
		seq := (uint16(b[1]) << 8) | uint16(b[2])
		// s.Trace("sequence", zap.Uint32("sequence", v.Sequence), zap.Uint16("seq", seq), zap.Int("len", len(b)))
		if v.Sequence != uint32(seq) {
			s.tb.Fatal("sequence error", v.Sequence, seq)
		}
	default:
		s.Subscriber.OnEvent(event)
	}
}

func (pub *UnitTestPublisher) OnEvent(event any) {
	switch event.(type) {
	case IPublisher:
		pub.VideoTrack = track.NewH264(pub)
		pub.VideoTrack.WriteAnnexB(0, 0, spspps)
		pub.AudioTrack = track.NewAAC(pub)
		go pub.WriteAudio()
		go pub.WriteVideo()
	}
}
func (pub *UnitTestPublisher) WriteAudio() {
	for i := uint32(0); pub.Err() == nil; i++ {
		time.Sleep(40 * time.Millisecond)
		elapse := time.Since(pub.StartTime)
		pts := uint32(elapse.Milliseconds() * 90)
		pub.AudioTrack.WriteADTS(pts, util.Buffer([]byte{0xFF, 0xE1, 0x20, 0x00, 0x29, 0xA7, 0xF0, byte(i >> 8), byte(i), 0}))
	}
}
func (pub *UnitTestPublisher) WriteVideo() {
	for i := uint32(0); pub.Err() == nil; i++ {
		time.Sleep(40 * time.Millisecond)
		elapse := time.Since(pub.StartTime)
		pts := uint32(elapse.Milliseconds() * 90)
		var naluType byte = 0x61
		if elapse%8 == 0 {
			naluType = 0x65
		}
		data := []byte{naluType, byte(i >> 8), byte(i)}
		pub.Trace("data", zap.Uint32("i", i))
		pub.VideoTrack.WriteNalu(pts, pts, data)
	}
}
