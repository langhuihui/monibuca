package mp4

import (
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/plugin/mp4/pkg/box"
	"os"
	"time"
)

func RecordMP4(ctx *m7s.RecordContext) (err error) {
	var file *os.File
	var muxer *box.Movmuxer
	var audioId, videoId uint32
	// TODO: fragment
	if file, err = os.OpenFile(ctx.FilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666); err != nil {
		return
	}
	defer func() {
		err = muxer.WriteTrailer()
		if err != nil {
			ctx.Error("write trailer", "err", err)
		} else {
			ctx.Info("write trailer")
		}
		err = file.Close()
	}()
	muxer, err = box.CreateMp4Muxer(file)
	ar, vr := ctx.Subscriber.AudioReader, ctx.Subscriber.VideoReader
	if ar != nil {
		audioTrack := ar.Track
		switch ctx := audioTrack.ICodecCtx.GetBase().(type) {
		case *codec.AACCtx:
			audioId = muxer.AddAudioTrack(box.MP4_CODEC_AAC, box.WithExtraData(ctx.ConfigBytes))
		case *codec.PCMACtx:
			audioId = muxer.AddAudioTrack(box.MP4_CODEC_G711A, box.WithAudioSampleRate(uint32(ctx.SampleRate)), box.WithAudioChannelCount(uint8(ctx.Channels)), box.WithAudioSampleBits(uint8(ctx.SampleSize)))
		case *codec.PCMUCtx:
			audioId = muxer.AddAudioTrack(box.MP4_CODEC_G711U, box.WithAudioSampleRate(uint32(ctx.SampleRate)), box.WithAudioChannelCount(uint8(ctx.Channels)), box.WithAudioSampleBits(uint8(ctx.SampleSize)))
		}
	}
	if vr != nil {
		videoTrack := vr.Track
		switch ctx := videoTrack.ICodecCtx.GetBase().(type) {
		case *codec.H264Ctx:
			videoId = muxer.AddVideoTrack(box.MP4_CODEC_H264, box.WithExtraData(ctx.Record))
		case *codec.H265Ctx:
			videoId = muxer.AddVideoTrack(box.MP4_CODEC_H265, box.WithExtraData(ctx.Record))
		}
	}
	return m7s.PlayBlock(ctx.Subscriber, func(audio *pkg.RawAudio) error {
		return muxer.WriteAudio(audioId, audio.ToBytes(), uint64(audio.Timestamp/time.Millisecond))
	}, func(video *pkg.H26xFrame) error {
		var nalus [][]byte
		for _, nalu := range video.Nalus {
			nalus = append(nalus, nalu.ToBytes())
		}
		return muxer.WriteVideo(videoId, nalus, uint64(video.Timestamp/time.Millisecond), uint64(video.CTS/time.Millisecond))
	})
}
