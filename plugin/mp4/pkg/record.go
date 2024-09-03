package mp4

import (
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/fs"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/task"
	"m7s.live/m7s/v5/plugin/mp4/pkg/box"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
	"os"
	"time"
)

type WriteTrailerQueueTask struct {
	task.Work
}

var writeTrailerQueueTask WriteTrailerQueueTask

func init() {
	m7s.Servers.AddTaskLazy(&writeTrailerQueueTask)
}

func NewRecorder() m7s.IRecorder {
	return &Recorder{}
}

type Recorder struct {
	m7s.DefaultRecorder
}

type writeTrailerTask struct {
	task.Task
	muxer *box.Movmuxer
	file  *os.File
}

func (task *writeTrailerTask) Start() (err error) {
	err = task.muxer.WriteTrailer()
	if err != nil {
		task.Error("write trailer", "err", err)
		return task.file.Close()
	} else {
		task.Info("write trailer")
		var temp *os.File
		temp, err = os.CreateTemp("", "*.mp4")
		if err != nil {
			task.Error("create temp file", "err", err)
			return
		}
		err = task.muxer.ReWriteWithMoov(temp)
		if err != nil {
			task.Error("rewrite with moov", "err", err)
			return
		}
		err = task.file.Close()
		err = temp.Close()
		fs.MustCopyFile(temp.Name(), task.file.Name())
		return os.Remove(temp.Name())
	}
}

func (r *Recorder) Run() (err error) {
	recordJob := &r.RecordJob
	sub := recordJob.Subscriber
	var file *os.File
	var muxer *box.Movmuxer
	var audioId, videoId uint32
	// TODO: fragment
	if file, err = os.OpenFile(recordJob.FilePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666); err != nil {
		return
	}
	muxer, err = box.CreateMp4Muxer(file)
	if err != nil {
		return
	}
	defer writeTrailerQueueTask.AddTask(&writeTrailerTask{
		file:  file,
		muxer: muxer,
	}, r.Logger)
	var at, vt *pkg.AVTrack
	//err = muxer.WriteInitSegment(file)
	return m7s.PlayBlock(sub, func(audio *pkg.RawAudio) error {
		if at == nil {
			at = sub.AudioReader.Track
			switch ctx := at.ICodecCtx.GetBase().(type) {
			case *codec.AACCtx:
				audioId = muxer.AddAudioTrack(box.MP4_CODEC_AAC, box.WithExtraData(ctx.ConfigBytes))
			case *codec.PCMACtx:
				audioId = muxer.AddAudioTrack(box.MP4_CODEC_G711A, box.WithAudioSampleRate(uint32(ctx.SampleRate)), box.WithAudioChannelCount(uint8(ctx.Channels)), box.WithAudioSampleBits(uint8(ctx.SampleSize)))
			case *codec.PCMUCtx:
				audioId = muxer.AddAudioTrack(box.MP4_CODEC_G711U, box.WithAudioSampleRate(uint32(ctx.SampleRate)), box.WithAudioChannelCount(uint8(ctx.Channels)), box.WithAudioSampleBits(uint8(ctx.SampleSize)))
			}
		}
		return muxer.WriteSample(audioId, box.Sample{
			Data: audio.ToBytes(),
			PTS:  uint32(audio.Timestamp / time.Millisecond),
			DTS:  uint32(audio.Timestamp / time.Millisecond),
		})
	}, func(video *rtmp.RTMPVideo) error {
		offset := 5
		bytes := video.ToBytes()
		if vt == nil {
			vt = sub.VideoReader.Track
			switch ctx := vt.ICodecCtx.GetBase().(type) {
			case *codec.H264Ctx:
				videoId = muxer.AddVideoTrack(box.MP4_CODEC_H264, box.WithExtraData(ctx.Record), box.WithVideoWidth(uint32(ctx.Width())), box.WithVideoHeight(uint32(ctx.Height())))
			case *codec.H265Ctx:
				videoId = muxer.AddVideoTrack(box.MP4_CODEC_H265, box.WithExtraData(ctx.Record), box.WithVideoWidth(uint32(ctx.Width())), box.WithVideoHeight(uint32(ctx.Height())))
			}
		}
		switch ctx := vt.ICodecCtx.(type) {
		case *codec.H264Ctx:
			if bytes[1] == 0 {
				return nil
			}
		case *rtmp.H265Ctx:
			if ctx.Enhanced {
				switch bytes[1] & 0b1111 {
				case rtmp.PacketTypeCodedFrames:
					offset += 3
				case rtmp.PacketTypeSequenceStart:
					return nil
				}
			} else if bytes[1] == 0 {
				return nil
			}
		}

		return muxer.WriteSample(videoId, box.Sample{
			KeyFrame: sub.VideoReader.Value.IDR,
			Data:     bytes[offset:],
			PTS:      video.Timestamp + video.CTS,
			DTS:      video.Timestamp,
		})
	})
}
