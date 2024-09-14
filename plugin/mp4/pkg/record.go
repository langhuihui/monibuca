package mp4

import (
	"fmt"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/fs"
	"os"
	"path/filepath"
	"time"

	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/task"
	"m7s.live/m7s/v5/plugin/mp4/pkg/box"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
)

type WriteTrailerQueueTask struct {
	task.Work
}

var writeTrailerQueueTask WriteTrailerQueueTask

type writeTrailerTask struct {
	task.Task
	muxer *FileMuxer
}

func (task *writeTrailerTask) Start() (err error) {
	err = task.muxer.WriteTrailer()
	if err != nil {
		task.Error("write trailer", "err", err)
		return task.muxer.File.Close()
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
		err = task.muxer.File.Close()
		err = temp.Close()
		fs.MustCopyFile(temp.Name(), task.muxer.File.Name())
		return os.Remove(temp.Name())
	}
}

func init() {
	m7s.Servers.AddTaskLazy(&writeTrailerQueueTask)
}

func NewRecorder() m7s.IRecorder {
	return &Recorder{}
}

type Recorder struct {
	m7s.DefaultRecorder
	muxer  *FileMuxer
	stream m7s.RecordStream
}

func (r *Recorder) writeTailer() {
	r.stream.EndTime = time.Now()
	r.RecordJob.Plugin.DB.Save(&r.stream)
	writeTrailerQueueTask.AddTask(&writeTrailerTask{
		muxer: r.muxer,
	}, r.Logger)
}

func (r *Recorder) createStream() (err error) {
	recordJob := &r.RecordJob
	sub := recordJob.Subscriber
	r.stream = m7s.RecordStream{
		StartTime: time.Now(),
		FilePath:  recordJob.FilePath,
	}
	if sub.Publisher.HasAudioTrack() {
		r.stream.AudioCodec = sub.Publisher.AudioTrack.ICodecCtx.FourCC().String()
	}
	if sub.Publisher.HasVideoTrack() {
		r.stream.VideoCodec = sub.Publisher.VideoTrack.ICodecCtx.FourCC().String()
	}
	recordJob.Plugin.DB.Save(&r.stream)
	var file *os.File
	if r.RecordJob.Fragment == 0 {
		if file, err = os.Create(fmt.Sprintf("%d.mp4", r.RecordJob.FilePath)); err != nil {
			return
		}
	} else {
		if file, err = os.Create(filepath.Join(r.RecordJob.FilePath, fmt.Sprintf("%d.mp4", r.stream.ID))); err != nil {
			return
		}
	}
	r.muxer, err = NewFileMuxer(file)
	return
}

func (r *Recorder) Start() (err error) {
	err = r.RecordJob.Plugin.DB.AutoMigrate(&r.stream)
	if err != nil {
		return
	}
	return r.DefaultRecorder.Start()
}

func (r *Recorder) Dispose() {
	if r.muxer != nil {
		r.writeTailer()
	}
}

func (r *Recorder) Run() (err error) {
	recordJob := &r.RecordJob
	sub := recordJob.Subscriber
	var audioTrack, videoTrack *Track
	err = r.createStream()
	if err != nil {
		return
	}
	var at, vt *pkg.AVTrack

	checkFragment := func(absTime uint32) (err error) {
		if duration := int64(absTime); time.Duration(duration)*time.Millisecond >= recordJob.Fragment {
			r.writeTailer()
			err = r.createStream()
			if err != nil {
				return
			}
			at, vt = nil, nil
			if vr := sub.VideoReader; vr != nil {
				vr.ResetAbsTime()
				//seq := vt.SequenceFrame.(*rtmp.RTMPVideo)
				//offset = int64(seq.Size + 15)
			}
			if ar := sub.AudioReader; ar != nil {
				ar.ResetAbsTime()
			}
		}
		return
	}

	return m7s.PlayBlock(sub, func(audio *pkg.RawAudio) error {
		if sub.VideoReader == nil && recordJob.Fragment != 0 {
			err := checkFragment(sub.AudioReader.AbsTime)
			if err != nil {
				return err
			}
		}
		if at == nil {
			at = sub.AudioReader.Track
			switch ctx := at.ICodecCtx.GetBase().(type) {
			case *codec.AACCtx:
				track := r.muxer.AddTrack(box.MP4_CODEC_AAC)
				audioTrack = track
				track.ExtraData = ctx.ConfigBytes
			case *codec.PCMACtx:
				track := r.muxer.AddTrack(box.MP4_CODEC_G711A)
				audioTrack = track
				track.SampleSize = uint16(ctx.SampleSize)
				track.SampleRate = uint32(ctx.SampleRate)
				track.ChannelCount = uint8(ctx.Channels)
			case *codec.PCMUCtx:
				track := r.muxer.AddTrack(box.MP4_CODEC_G711U)
				audioTrack = track
				track.SampleSize = uint16(ctx.SampleSize)
				track.SampleRate = uint32(ctx.SampleRate)
				track.ChannelCount = uint8(ctx.Channels)
			}
		}
		dts := sub.AudioReader.AbsTime
		return r.muxer.WriteSample(audioTrack, box.Sample{
			Data: audio.ToBytes(),
			PTS:  uint64(dts),
			DTS:  uint64(dts),
		})
	}, func(video *rtmp.RTMPVideo) error {
		if sub.VideoReader.Value.IDR && recordJob.Fragment != 0 {
			err := checkFragment(sub.VideoReader.AbsTime)
			if err != nil {
				return err
			}
		}
		offset := 5
		bytes := video.ToBytes()
		if vt == nil {
			vt = sub.VideoReader.Track
			switch ctx := vt.ICodecCtx.GetBase().(type) {
			case *codec.H264Ctx:
				track := r.muxer.AddTrack(box.MP4_CODEC_H264)
				videoTrack = track
				track.ExtraData = ctx.Record
				track.Width = uint32(ctx.Width())
				track.Height = uint32(ctx.Height())
			case *codec.H265Ctx:
				track := r.muxer.AddTrack(box.MP4_CODEC_H265)
				videoTrack = track
				track.ExtraData = ctx.Record
				track.Width = uint32(ctx.Width())
				track.Height = uint32(ctx.Height())
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

		return r.muxer.WriteSample(videoTrack, box.Sample{
			KeyFrame: sub.VideoReader.Value.IDR,
			Data:     bytes[offset:],
			PTS:      uint64(sub.VideoReader.AbsTime) + uint64(video.CTS),
			DTS:      uint64(sub.VideoReader.AbsTime),
		})
	})
}
