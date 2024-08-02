package mp4

import (
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/plugin/mp4/pkg/box"
	"time"
)

type Recorder struct {
	*m7s.Subscriber
	*box.Movmuxer
	videoId uint32
	audioId uint32
}

func (r *Recorder) Record(recorder *m7s.Recorder) (err error) {
	r.Movmuxer, err = box.CreateMp4Muxer(recorder.File)
	if recorder.Publisher.HasAudioTrack() {
		audioTrack := recorder.Publisher.AudioTrack
		switch ctx := audioTrack.ICodecCtx.GetBase().(type) {
		case *codec.AACCtx:
			r.audioId = r.AddAudioTrack(box.MP4_CODEC_AAC, box.WithExtraData(ctx.ConfigBytes))
		case *codec.PCMACtx:
			r.audioId = r.AddAudioTrack(box.MP4_CODEC_G711A, box.WithAudioSampleRate(uint32(ctx.SampleRate)), box.WithAudioChannelCount(uint8(ctx.Channels)), box.WithAudioSampleBits(uint8(ctx.SampleSize)))
		case *codec.PCMUCtx:
			r.audioId = r.AddAudioTrack(box.MP4_CODEC_G711U, box.WithAudioSampleRate(uint32(ctx.SampleRate)), box.WithAudioChannelCount(uint8(ctx.Channels)), box.WithAudioSampleBits(uint8(ctx.SampleSize)))
		}
	}
	if recorder.Publisher.HasVideoTrack() {
		videoTrack := recorder.Publisher.VideoTrack
		switch ctx := videoTrack.ICodecCtx.GetBase().(type) {
		case *codec.H264Ctx:
			r.videoId = r.AddVideoTrack(box.MP4_CODEC_H264, box.WithExtraData(ctx.Record))
		case *codec.H265Ctx:
			r.videoId = r.AddVideoTrack(box.MP4_CODEC_H265, box.WithExtraData(ctx.Record))
		}
	}
	r.Subscriber = &recorder.Subscriber
	return m7s.PlayBlock(&recorder.Subscriber, func(audio *pkg.RawAudio) error {
		return r.WriteAudio(r.audioId, audio.ToBytes(), uint64(audio.Timestamp/time.Millisecond))
	}, func(video *pkg.H26xFrame) error {
		var nalus [][]byte
		for _, nalu := range video.Nalus {
			nalus = append(nalus, nalu.ToBytes())
		}
		return r.WriteVideo(r.videoId, nalus, uint64(video.Timestamp/time.Millisecond), uint64(video.CTS/time.Millisecond))
	})
}

func (r *Recorder) Close() {
	//defer func() {
	//	if err := recover(); err != nil {
	//		r.Error("close", "err", err)
	//	} else {
	//		r.Info("close")
	//	}
	//}()
	err := r.WriteTrailer()
	if err != nil {
		r.Error("write trailer", "err", err)
	} else {
		r.Info("write trailer")
	}
}
