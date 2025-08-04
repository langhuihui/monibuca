package mp4

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	m7s "m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/plugin/mp4/pkg/box"
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

const BeforeMdatData = 16 // free box + mdat box header or big mdat box header
// 将 moov 从末尾移动到前方
// 将 ftyp + free(optional) + moov + mdat 写入临时文件, 然后替换原文件
func (t *writeTrailerTask) Run() (err error) {
	t.Info("write trailer")
	var temp *os.File
	temp, err = os.CreateTemp("", "*.mp4")
	if err != nil {
		t.Error("create temp file", "err", err)
		return
	}

	defer os.Remove(temp.Name())

	_, err = t.file.Seek(0, io.SeekStart)
	if err != nil {
		t.Error("seek file", "err", err)
		return
	}
	// 复制 mdat box之前的内容
	_, err = io.CopyN(temp, t.file, int64(t.muxer.mdatOffset)-BeforeMdatData)
	if err != nil {
		return
	}
	for _, track := range t.muxer.Tracks {
		for i := range len(track.Samplelist) {
			track.Samplelist[i].Offset += int64(t.muxer.moov.Size())
		}
	}
	err = t.muxer.WriteMoov(temp)
	if err != nil {
		return
	}
	// 复制 mdat box
	_, err = io.CopyN(temp, t.file, int64(t.muxer.mdatSize)+BeforeMdatData)

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

func init() {
	m7s.Servers.AddTask(&writeTrailerQueueTask)
}

func NewRecorder(conf config.Record) m7s.IRecorder {
	return &Recorder{}
}

type Recorder struct {
	m7s.DefaultRecorder
	muxer *Muxer
	file  *os.File
}

func (r *Recorder) writeTailer(end time.Time) {
	r.WriteTail(end, &writeTrailerQueueTask)
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
	if r.RecordJob.RecConf.Type == "" {
		r.RecordJob.RecConf.Type = "mp4"
	}
	err = r.CreateStream(start, CustomFileName)
	if err != nil {
		return
	}
	r.file, err = os.Create(r.Event.FilePath)
	if err != nil {
		return
	}
	if r.Event.Type == "fmp4" {
		r.muxer = NewMuxerWithStreamPath(FLAG_FRAGMENT, r.Event.StreamPath)
	} else {
		r.muxer = NewMuxerWithStreamPath(0, r.Event.StreamPath)
	}
	return r.muxer.WriteInitSegment(r.file)
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
	var at, vt *pkg.AVTrack
	checkEventRecordStop := func(absTime uint32) (err error) {
		if absTime >= recordJob.Event.AfterDuration+recordJob.Event.BeforeDuration {
			r.RecordJob.Stop(task.ErrStopByUser)
		}
		return
	}

	checkFragment := func(reader *pkg.AVRingReader) (err error) {
		if duration := int64(reader.AbsTime); time.Duration(duration)*time.Millisecond >= recordJob.RecConf.Fragment {
			r.writeTailer(reader.Value.WriteTime)
			err = r.createStream(reader.Value.WriteTime)
			if err != nil {
				return
			}
			at, vt = nil, nil
			if vr := sub.VideoReader; vr != nil {
				vr.ResetAbsTime()
			}
			if ar := sub.AudioReader; ar != nil {
				ar.ResetAbsTime()
			}
		}
		return
	}

	return m7s.PlayBlock(sub, func(audio *AudioFrame) error {
		if r.Event.StartTime.IsZero() {
			err = r.createStream(sub.AudioReader.Value.WriteTime)
			if err != nil {
				return err
			}
		}
		r.Event.Duration = sub.AudioReader.AbsTime
		if sub.VideoReader == nil {
			if recordJob.Event != nil {
				err = checkEventRecordStop(sub.VideoReader.AbsTime)
				if err != nil {
					return err
				}
			}
			if recordJob.RecConf.Fragment != 0 {
				err = checkFragment(sub.AudioReader)
				if err != nil {
					return err
				}
			}
		}
		if at == nil {
			at = sub.AudioReader.Track
			switch at.ICodecCtx.GetBase().(type) {
			case *codec.AACCtx:
				track := r.muxer.AddTrack(box.MP4_CODEC_AAC)
				audioTrack = track
				track.ICodecCtx = at.ICodecCtx
			case *codec.PCMACtx:
				track := r.muxer.AddTrack(box.MP4_CODEC_G711A)
				audioTrack = track
				track.ICodecCtx = at.ICodecCtx
			case *codec.PCMUCtx:
				track := r.muxer.AddTrack(box.MP4_CODEC_G711U)
				audioTrack = track
				track.ICodecCtx = at.ICodecCtx
			}
		}
		sample := box.Sample{
			Timestamp: sub.AudioReader.AbsTime,
			Memory:    audio.Memory,
		}
		return r.muxer.WriteSample(r.file, audioTrack, sample)
	}, func(video *VideoFrame) error {
		if r.Event.StartTime.IsZero() {
			err = r.createStream(sub.VideoReader.Value.WriteTime)
			if err != nil {
				return err
			}
		}
		r.Event.Duration = sub.VideoReader.AbsTime
		if sub.VideoReader.Value.IDR {
			if recordJob.Event != nil {
				err = checkEventRecordStop(sub.VideoReader.AbsTime)
				if err != nil {
					return err
				}
			}
			if recordJob.RecConf.Fragment != 0 {
				err = checkFragment(sub.VideoReader)
				if err != nil {
					return err
				}
			}
		}

		if vt == nil {
			vt = sub.VideoReader.Track
			switch vt.ICodecCtx.GetBase().(type) {
			case *codec.H264Ctx:
				track := r.muxer.AddTrack(box.MP4_CODEC_H264)
				videoTrack = track
				track.ICodecCtx = vt.ICodecCtx
			case *codec.H265Ctx:
				track := r.muxer.AddTrack(box.MP4_CODEC_H265)
				videoTrack = track
				track.ICodecCtx = vt.ICodecCtx
			}
		}
		ctx := vt.ICodecCtx.(pkg.IVideoCodecCtx)
		if videoTrackCtx, ok := videoTrack.ICodecCtx.(pkg.IVideoCodecCtx); ok && videoTrackCtx != ctx {
			width, height := uint32(ctx.Width()), uint32(ctx.Height())
			oldWidth, oldHeight := uint32(videoTrackCtx.Width()), uint32(videoTrackCtx.Height())
			r.Info("ctx  changed, restarting recording",
				"old", fmt.Sprintf("%dx%d", oldWidth, oldHeight),
				"new", fmt.Sprintf("%dx%d", width, height))
			r.writeTailer(sub.VideoReader.Value.WriteTime)
			err = r.createStream(sub.VideoReader.Value.WriteTime)
			if err != nil {
				return nil
			}
			at, vt = nil, nil
			if vr := sub.VideoReader; vr != nil {
				vr.ResetAbsTime()
			}
			if ar := sub.AudioReader; ar != nil {
				ar.ResetAbsTime()
			}
		}
		sample := box.Sample{
			Timestamp: sub.VideoReader.AbsTime,
			KeyFrame:  video.IDR,
			CTS:       video.GetCTS32(),
			Memory:    video.Memory,
		}
		return r.muxer.WriteSample(r.file, videoTrack, sample)
	})
}
