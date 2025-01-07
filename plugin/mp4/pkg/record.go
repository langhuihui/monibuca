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
	muxer *FileMuxer
}

func (task *writeTrailerTask) Start() (err error) {
	err = task.muxer.WriteTrailer()
	if err != nil {
		task.Error("write trailer", "err", err)
		if errClose := task.muxer.File.Close(); errClose != nil {
			return errClose
		}
	}
	return
}

func (task *writeTrailerTask) Run() (err error) {
	task.Info("write trailer")
	var temp *os.File
	temp, err = os.CreateTemp("", "*.mp4")
	if err != nil {
		task.Error("create temp file", "err", err)
		return
	}
	defer os.Remove(temp.Name())
	err = task.muxer.ReWriteWithMoov(temp)
	if err != nil {
		task.Error("rewrite with moov", "err", err)
		return
	}
	if _, err = task.muxer.File.Seek(0, io.SeekStart); err != nil {
		task.Error("seek file", "err", err)
		return
	}
	if _, err = temp.Seek(0, io.SeekStart); err != nil {
		task.Error("seek temp file", "err", err)
		return
	}
	if _, err = io.Copy(task.muxer.File, temp); err != nil {
		task.Error("copy file", "err", err)
		return
	}
	if err = task.muxer.File.Close(); err != nil {
		task.Error("close file", "err", err)
		return
	}
	if err = temp.Close(); err != nil {
		task.Error("close temp file", "err", err)
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
	}
	t.DB.Where(&queryRecord).Find(&eventRecordStreams, "stream_path=?", t.streamPath) //搜索事件录像，且为重要事件（无法自动删除）
	if len(eventRecordStreams) > 0 {
		for _, recordStream := range eventRecordStreams {
			var unimportantEventRecordStreams []m7s.RecordStream
			queryRecord.EventLevel = m7s.EventLevelLow
			query := `(start_time BETWEEN ? AND ?)
							OR (end_time BETWEEN ? AND ?) 
							OR (? BETWEEN start_time AND end_time) 
							OR (? BETWEEN start_time AND end_time) AND stream_path=? `
			t.DB.Where(&queryRecord).Where(query, recordStream.StartTime, recordStream.EndTime, recordStream.StartTime, recordStream.EndTime, recordStream.StartTime, recordStream.EndTime, recordStream.StreamPath).Find(&unimportantEventRecordStreams)
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
		writeTrailerQueueTask.AddTask(&eventRecordCheck{
			DB:         r.RecordJob.Plugin.DB,
			streamPath: r.stream.StreamPath,
		})
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
		if sub.VideoReader == nil {
			if recordJob.AfterDuration != 0 {
				err := checkEventRecordStop(sub.VideoReader.AbsTime)
				if err != nil {
					return err
				}
			}
			if recordJob.Fragment != 0 {
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
		if sub.VideoReader.Value.IDR {
			if recordJob.AfterDuration != 0 {
				err := checkEventRecordStop(sub.VideoReader.AbsTime)
				if err != nil {
					return err
				}
			}
			if recordJob.Fragment != 0 {
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
		return r.muxer.WriteSample(videoTrack, box.Sample{
			KeyFrame: sub.VideoReader.Value.IDR,
			Data:     bytes[offset:],
			PTS:      uint64(sub.VideoReader.AbsTime) + uint64(video.CTS),
			DTS:      uint64(sub.VideoReader.AbsTime),
		})
	})
}
