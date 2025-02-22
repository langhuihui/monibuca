package mp4

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"
	m7s "m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/plugin/mp4/pkg/box"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
)

type WriteTrailerQueueTask struct {
	task.Work
}

var writeTrailerQueueTask WriteTrailerQueueTask

type writeTrailerTask struct {
	task.Task
	muxer *Muxer
	file  *os.File
}

func (task *writeTrailerTask) Start() (err error) {
	err = task.muxer.WriteTrailer(task.file)
	if err != nil {
		task.Error("write trailer", "err", err)
		if task.file != nil {
			if errClose := task.file.Close(); errClose != nil {
				return errClose
			}
		}
	}
	return
}

func (t *writeTrailerTask) Run() (err error) {
	t.Info("write trailer")
	var temp *os.File
	temp, err = os.CreateTemp("", "*.mp4")
	if err != nil {
		t.Error("create temp file", "err", err)
		return
	}
	defer os.Remove(temp.Name())
	err = t.muxer.ReWriteWithMoov(temp, t.file)
	if err != nil {
		if err == pkg.ErrSkip {
			return task.ErrTaskComplete
		}
		t.Error("rewrite with moov", "err", err)
		return
	}
	if _, err = t.file.Seek(0, io.SeekStart); err != nil {
		t.Error("seek file", "err", err)
		return
	}
	if _, err = temp.Seek(0, io.SeekStart); err != nil {
		t.Error("seek temp file", "err", err)
		return
	}
	if _, err = io.Copy(t.file, temp); err != nil {
		t.Error("copy file", "err", err)
		return
	}
	if err = t.file.Close(); err != nil {
		t.Error("close file", "err", err)
		return
	}
	if err = temp.Close(); err != nil {
		t.Error("close temp file", "err", err)
	}
	return
}

type eventRecordCheck struct {
	task.Task
	DB         *gorm.DB
	streamPath string
}

func (t *eventRecordCheck) Run() (err error) {
	var eventRecordStreams []m7s.RecordStream
	queryRecord := m7s.RecordStream{
		EventLevel: m7s.EventLevelHigh,
		Mode:       m7s.RecordModeEvent,
		Type:       "mp4",
		StreamPath: t.streamPath,
	}
	t.DB.Where(&queryRecord).Find(&eventRecordStreams) //搜索事件录像，且为重要事件（无法自动删除）
	if len(eventRecordStreams) > 0 {
		for _, recordStream := range eventRecordStreams {
			var unimportantEventRecordStreams []m7s.RecordStream
			queryRecord.EventLevel = m7s.EventLevelLow
			queryRecord.Mode = m7s.RecordModeAuto
			query := `start_time <= ? and end_time >= ?`
			t.DB.Where(&queryRecord).Where(query, recordStream.EndTime, recordStream.StartTime).Find(&unimportantEventRecordStreams)
			if len(unimportantEventRecordStreams) > 0 {
				for _, unimportantEventRecordStream := range unimportantEventRecordStreams {
					unimportantEventRecordStream.EventLevel = m7s.EventLevelHigh
					t.DB.Save(&unimportantEventRecordStream)
				}
			}
		}
	}
	return
}

func init() {
	m7s.Servers.AddTask(&writeTrailerQueueTask)
}

func NewRecorder(conf config.Record) m7s.IRecorder {
	return &Recorder{}
}

type Recorder struct {
	m7s.DefaultRecorder
	muxer  *Muxer
	file   *os.File
	stream m7s.RecordStream
}

func (r *Recorder) writeTailer(end time.Time) {
	r.stream.EndTime = end
	if r.RecordJob.Plugin.DB != nil {
		r.RecordJob.Plugin.DB.Save(&r.stream)
		writeTrailerQueueTask.AddTask(&eventRecordCheck{
			DB:         r.RecordJob.Plugin.DB,
			streamPath: r.stream.StreamPath,
		})
	}
	writeTrailerQueueTask.AddTask(&writeTrailerTask{
		muxer: r.muxer,
		file:  r.file,
	}, r.Logger)
}

var CustomFileName = func(job *m7s.RecordJob) string {
	if job.RecConf.Fragment == 0 {
		return fmt.Sprintf("%s.mp4", job.RecConf.FilePath)
	}
	return filepath.Join(job.RecConf.FilePath, fmt.Sprintf("%d.mp4", time.Now().Unix()))
}

func (r *Recorder) createStream(start time.Time) (err error) {
	recordJob := &r.RecordJob
	sub := recordJob.Subscriber
	r.stream = m7s.RecordStream{
		StartTime:      start,
		StreamPath:     sub.StreamPath,
		FilePath:       CustomFileName(&r.RecordJob),
		EventId:        recordJob.EventId,
		EventDesc:      recordJob.EventDesc,
		EventName:      recordJob.EventName,
		EventLevel:     recordJob.EventLevel,
		BeforeDuration: recordJob.BeforeDuration,
		AfterDuration:  recordJob.AfterDuration,
		Mode:           recordJob.Mode,
		Type:           "mp4",
	}
	dir := filepath.Dir(r.stream.FilePath)
	if err = os.MkdirAll(dir, 0755); err != nil {
		return
	}
	r.file, err = os.Create(r.stream.FilePath)
	if err != nil {
		return
	}
	if recordJob.RecConf.Type == "fmp4" {
		r.stream.Type = "fmp4"
		r.muxer = NewMuxer(FLAG_FRAGMENT)
	} else {
		r.muxer = NewMuxer(0)
	}
	r.muxer.WriteInitSegment(r.file)
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

func (r *Recorder) Dispose() {
	if r.muxer != nil {
		r.writeTailer(time.Now())
	}
}

func (r *Recorder) Run() (err error) {
	recordJob := &r.RecordJob
	sub := recordJob.Subscriber
	var audioTrack, videoTrack *Track
	startTime := time.Now()
	if recordJob.BeforeDuration > 0 {
		startTime = startTime.Add(-recordJob.BeforeDuration)
	}
	err = r.createStream(startTime)
	if err != nil {
		return
	}
	var at, vt *pkg.AVTrack

	checkEventRecordStop := func(absTime uint32) (err error) {
		if duration := int64(absTime); time.Duration(duration)*time.Millisecond >= recordJob.AfterDuration+recordJob.BeforeDuration {
			now := time.Now()
			r.writeTailer(now)
			r.RecordJob.Stop(task.ErrStopByUser)
		}
		return
	}

	checkFragment := func(absTime uint32) (err error) {
		if duration := int64(absTime); time.Duration(duration)*time.Millisecond >= recordJob.RecConf.Fragment {
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
		if sub.VideoReader == nil {
			if recordJob.AfterDuration != 0 {
				err := checkEventRecordStop(sub.VideoReader.AbsTime)
				if err != nil {
					return err
				}
			}
			if recordJob.RecConf.Fragment != 0 {
				err := checkFragment(sub.AudioReader.AbsTime)
				if err != nil {
					return err
				}
			}
		}
		if at == nil {
			at = sub.AudioReader.Track
			switch ctx := at.ICodecCtx.GetBase().(type) {
			case *codec.AACCtx:
				track := r.muxer.AddTrack(box.MP4_CODEC_AAC)
				audioTrack = track
				track.SampleSize = uint16(16)
				track.SampleRate = uint32(ctx.SampleRate())
				track.ChannelCount = uint8(ctx.ChannelLayout().Count())
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
		return r.muxer.WriteSample(r.file, audioTrack, box.Sample{
			Data:      audio.ToBytes(),
			Timestamp: uint32(dts),
		})
	}, func(video *rtmp.RTMPVideo) error {
		if sub.VideoReader.Value.IDR {
			if recordJob.AfterDuration != 0 {
				err := checkEventRecordStop(sub.VideoReader.AbsTime)
				if err != nil {
					return err
				}
			}
			if recordJob.RecConf.Fragment != 0 {
				err := checkFragment(sub.VideoReader.AbsTime)
				if err != nil {
					return err
				}
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
				// Check if video resolution has changed
				if uint32(ctx.Width()) != videoTrack.Width || uint32(ctx.Height()) != videoTrack.Height {
					r.Info("Video resolution changed, restarting recording",
						"old", fmt.Sprintf("%dx%d", videoTrack.Width, videoTrack.Height),
						"new", fmt.Sprintf("%dx%d", ctx.Width(), ctx.Height()))
					now := time.Now()
					r.writeTailer(now)
					err = r.createStream(now)
					if err != nil {
						return nil
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
				return nil
			}
		case *rtmp.H265Ctx:
			if ctx.Enhanced {
				switch t := bytes[0] & 0b1111; t {
				case rtmp.PacketTypeCodedFrames:
					offset += 3
				case rtmp.PacketTypeSequenceStart:
					return nil
				case rtmp.PacketTypeCodedFramesX:
				default:
					r.Warn("unknown h265 packet type", "type", t)
					return nil
				}
			} else if bytes[1] == 0 {
				return nil
			}
		}
		return r.muxer.WriteSample(r.file, videoTrack, box.Sample{
			KeyFrame:  sub.VideoReader.Value.IDR,
			Data:      bytes[offset:],
			Timestamp: uint32(sub.VideoReader.AbsTime),
			CTS:       video.CTS,
		})
	})
}
