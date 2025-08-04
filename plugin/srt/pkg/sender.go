package srt

import (
	srt "github.com/datarhei/gosrt"
	"m7s.live/v5"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/format"
	mpegts "m7s.live/v5/pkg/format/ts"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	hls "m7s.live/v5/plugin/hls/pkg"
)

type Sender struct {
	task.Task
	hls.TsInMemory
	srt.Conn
	Subscriber *m7s.Subscriber
}

func (s *Sender) Start() error {
	var audioCodec, videoCodec codec.FourCC
	if s.Subscriber.Publisher.HasAudioTrack() {
		audioCodec = s.Subscriber.Publisher.AudioTrack.FourCC()
	}
	if s.Subscriber.Publisher.HasVideoTrack() {
		videoCodec = s.Subscriber.Publisher.VideoTrack.FourCC()
	}
	s.SetAllocator(util.NewScalableMemoryAllocator(1 << util.MinPowerOf2))
	s.Using(s.GetAllocator(), s.Subscriber)
	s.OnStop(s.Conn.Close)
	s.WritePMTPacket(audioCodec, videoCodec)
	s.Write(mpegts.DefaultPATPacket)
	s.Write(s.PMT)
	return nil
}

func (s *Sender) Run() error {
	pesAudio, pesVideo := mpegts.CreatePESWriters()
	return m7s.PlayBlock(s.Subscriber, func(audio *format.Mpeg2Audio) (err error) {
		pesAudio.Pts = uint64(s.Subscriber.AudioReader.AbsTime) * 90
		err = pesAudio.WritePESPacket(audio.Memory, &s.RecyclableMemory)
		if err == nil {
			s.RecyclableMemory.WriteTo(s)
			s.RecyclableMemory.Recycle()
		}
		return
	}, func(video *mpegts.VideoFrame) (err error) {
		vr := s.Subscriber.VideoReader
		pesVideo.IsKeyFrame = video.IDR
		pesVideo.Pts = uint64(vr.AbsTime+video.GetCTS32()) * 90
		pesVideo.Dts = uint64(vr.AbsTime) * 90
		err = pesVideo.WritePESPacket(video.Memory, &s.RecyclableMemory)
		if err == nil {
			s.RecyclableMemory.WriteTo(s)
			s.RecyclableMemory.Recycle()
		}
		return
	})
}
