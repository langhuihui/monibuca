package mp4

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"m7s.live/pro"
	"m7s.live/pro/pkg"
	"m7s.live/pro/pkg/codec"
	"m7s.live/pro/pkg/task"
	"m7s.live/pro/plugin/mp4/pkg/box"
	rtmp "m7s.live/pro/plugin/rtmp/pkg"
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
		_, err = task.muxer.File.Seek(0, io.SeekStart)
		_, err = temp.Seek(0, io.SeekStart)
		_, err = io.Copy(task.muxer.File, temp)
		err = task.muxer.File.Close()
		err = temp.Close()
		return os.Remove(temp.Name())
	}
}

func init() {
	m7s.Servers.AddTask(&writeTrailerQueueTask)
}

func NewRecorder() m7s.IRecorder {
	return &Recorder{}
}

type Recorder struct {
	m7s.DefaultRecorder
	muxer  *FileMuxer
	stream m7s.RecordStream
}

func (r *Recorder) writeTailer(end time.Time) {
	r.stream.EndTime = end
	if r.RecordJob.Plugin.DB != nil {
		r.RecordJob.Plugin.DB.Save(&r.stream)
	}
	writeTrailerQueueTask.AddTask(&writeTrailerTask{
		muxer: r.muxer,
	}, r.Logger)
}

var CustomFileName = func(job *m7s.RecordJob) string {
	if job.Fragment == 0 {
		return fmt.Sprintf("%s.mp4", job.FilePath)
	}
	return filepath.Join(job.FilePath, fmt.Sprintf("%d.mp4", time.Now().Unix()))
}

func (r *Recorder) createStream(start time.Time) (err error) {
	recordJob := &r.RecordJob
	sub := recordJob.Subscriber
	var file *os.File
	r.stream = m7s.RecordStream{
		StartTime:  start,
		StreamPath: sub.StreamPath,
		FilePath:   CustomFileName(&r.RecordJob),
	}
	dir := filepath.Dir(r.stream.FilePath)
	if err = os.MkdirAll(dir, 0755); err != nil {
		return
	}
	if file, err = os.Create(r.stream.FilePath); err != nil {
		return
	}
	r.muxer, err = NewFileMuxer(file)
	if sub.Publisher.HasAudioTrack() {
		r.stream.AudioCodec = sub.Publisher.AudioTrack.ICodecCtx.FourCC().String()
	}
	if sub.Publisher.HasVideoTrack() {
		r.stream.VideoCodec = sub.Publisher.VideoTrack.ICodecCtx.FourCC().String()
	}
	if recordJob.Plugin.DB != nil {
		recordJob.Plugin.DB.Save(&r.stream)
	}
	return
}

func (r *Recorder) Start() (err error) {
	if r.RecordJob.Plugin.DB != nil {
		err = r.RecordJob.Plugin.DB.AutoMigrate(&r.stream)
		if err != nil {
			return
		}
	}
	return r.DefaultRecorder.Start()
}

func (r *Recorder) Dispose() {
	if r.muxer != nil {
		r.writeTailer(time.Now())
	}
}

func (r *Recorder) Run() (err error) {
	recordJob := &r.RecordJob
	sub := recordJob.Subscriber
	var audioTrack, videoTrack *Track
	err = r.createStream(time.Now())
	if err != nil {
		return
	}
	var at, vt *pkg.AVTrack

	checkFragment := func(absTime uint32) (err error) {
		if duration := int64(absTime); time.Duration(duration)*time.Millisecond >= recordJob.Fragment {
			now := time.Now()
			r.writeTailer(now)
			err = r.createStream(now)
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
