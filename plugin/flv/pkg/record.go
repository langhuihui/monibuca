package flv

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
)

type WriteFlvMetaTagQueueTask struct {
	task.Work
}

var writeMetaTagQueueTask WriteFlvMetaTagQueueTask

func init() {
	m7s.Servers.AddTask(&writeMetaTagQueueTask)
}

type writeMetaTagTask struct {
	task.Task
	file     *os.File
	writer   *FlvWriter
	flags    byte
	metaData []byte
}

func (task *writeMetaTagTask) Start() (err error) {
	defer func() {
		err = task.file.Close()
		if info, err := task.file.Stat(); err == nil && info.Size() == 0 {
			err = os.Remove(info.Name())
		}
	}()
	var tempFile *os.File
	if tempFile, err = os.CreateTemp("", "*.flv"); err != nil {
		task.Error("create temp file failed", "err", err)
		return
	} else {
		defer func() {
			err = tempFile.Close()
			err = os.Remove(tempFile.Name())
			task.Info("writeMetaData success")
		}()
		_, err = tempFile.Write([]byte{'F', 'L', 'V', 0x01, task.flags, 0, 0, 0, 9, 0, 0, 0, 0})
		if err != nil {
			task.Error(err.Error())
			return
		}
		task.writer = NewFlvWriter(tempFile)
		err = task.writer.WriteTag(FLV_TAG_TYPE_SCRIPT, 0, uint32(len(task.metaData)), task.metaData)
		_, err = task.file.Seek(13, io.SeekStart)
		if err != nil {
			task.Error("writeMetaData Seek failed", "err", err)
			return
		}
		_, err = io.Copy(tempFile, task.file)
		if err != nil {
			task.Error("writeMetaData Copy failed", "err", err)
			return
		}
		_, err = tempFile.Seek(0, io.SeekStart)
		_, err = task.file.Seek(0, io.SeekStart)
		_, err = io.Copy(task.file, tempFile)
		if err != nil {
			task.Error("writeMetaData Copy failed", "err", err)
		}
		return
	}
}

func writeMetaTag(file *os.File, suber *m7s.Subscriber, filepositions []uint64, times []float64, duration *int64) {
	ar, vr := suber.AudioReader, suber.VideoReader
	hasAudio, hasVideo := ar != nil, vr != nil
	var amf rtmp.AMF
	metaData := rtmp.EcmaArray{
		"MetaDataCreator": "m7s/" + m7s.Version,
		"hasVideo":        hasVideo,
		"hasAudio":        hasAudio,
		"hasMatadata":     true,
		"canSeekToEnd":    true,
		"duration":        float64(*duration) / 1000,
		"hasKeyFrames":    len(filepositions) > 0,
		"filesize":        0,
	}
	var flags byte
	if hasAudio {
		ctx := ar.Track.ICodecCtx.GetBase().(pkg.IAudioCodecCtx)
		flags |= (1 << 2)
		metaData["audiocodecid"] = int(rtmp.ParseAudioCodec(ctx.FourCC()))
		metaData["audiosamplerate"] = ctx.GetSampleRate()
		metaData["audiosamplesize"] = ctx.GetSampleSize()
		metaData["stereo"] = ctx.GetChannels() == 2
	}
	if hasVideo {
		ctx := vr.Track.ICodecCtx.GetBase().(pkg.IVideoCodecCtx)
		flags |= 1
		metaData["videocodecid"] = int(rtmp.ParseVideoCodec(ctx.FourCC()))
		metaData["width"] = ctx.Width()
		metaData["height"] = ctx.Height()
		metaData["framerate"] = vr.Track.FPS
		metaData["videodatarate"] = vr.Track.BPS
		metaData["keyframes"] = map[string]any{
			"filepositions": filepositions,
			"times":         times,
		}
	}
	amf.Marshals("onMetaData", metaData)
	offset := amf.GetBuffer().Len() + 13 + 15
	if keyframesCount := len(filepositions); keyframesCount > 0 {
		metaData["filesize"] = uint64(offset) + filepositions[keyframesCount-1]
		for i := range filepositions {
			filepositions[i] += uint64(offset)
		}
		metaData["keyframes"] = map[string]any{
			"filepositions": filepositions,
			"times":         times,
		}
	}
	amf.GetBuffer().Reset()
	marshals := amf.Marshals("onMetaData", metaData)
	task := &writeMetaTagTask{
		file:     file,
		flags:    flags,
		metaData: marshals,
	}
	task.Logger = suber.Logger.With("file", file.Name())
	writeMetaTagQueueTask.AddTask(task)
}

func NewRecorder(conf config.Record) m7s.IRecorder {
	return &Recorder{}
}

type Recorder struct {
	m7s.DefaultRecorder
	writer *FlvWriter
	file   *os.File
}

var CustomFileName = func(job *m7s.RecordJob) string {
	if job.RecConf.Fragment == 0 || job.RecConf.Append {
		return fmt.Sprintf("%s.flv", job.RecConf.FilePath)
	}
	return filepath.Join(job.RecConf.FilePath, fmt.Sprintf("%d.flv", time.Now().Unix()))
}

func (r *Recorder) createStream(start time.Time) (err error) {
	r.RecordJob.RecConf.Type = "flv"
	err = r.CreateStream(start, CustomFileName)
	if err != nil {
		return
	}
	if r.file, err = os.OpenFile(r.Event.FilePath, os.O_CREATE|os.O_RDWR, 0666); err != nil {
		return
	}
	_, err = r.file.Write(FLVHead)
	r.writer = NewFlvWriter(r.file)
	if err != nil {
		return
	}
	return
}

func (r *Recorder) writeTailer(end time.Time) {
	if r.Event.EndTime.After(r.Event.StartTime) {
		return
	}
	r.Event.EndTime = end
	if r.RecordJob.Plugin.DB != nil {
		if r.RecordJob.Event != nil {
			r.RecordJob.Plugin.DB.Save(&r.Event)
		} else {
			r.RecordJob.Plugin.DB.Save(&r.Event.RecordStream)
		}
		writeMetaTagQueueTask.AddTask(m7s.NewEventRecordCheck(r.Event.Type, r.Event.StreamPath, r.RecordJob.Plugin.DB))
	}
}

func (r *Recorder) Dispose() {
	r.writeTailer(time.Now())
}

func (r *Recorder) Run() (err error) {
	var filepositions []uint64
	var times []float64
	var offset int64
	var duration int64
	ctx := &r.RecordJob
	suber := ctx.Subscriber
	noFragment := ctx.RecConf.Fragment == 0 || ctx.RecConf.Append
	checkFragment := func(absTime uint32, writeTime time.Time) {
		if duration = int64(absTime); time.Duration(duration)*time.Millisecond >= ctx.RecConf.Fragment {
			writeMetaTag(r.file, suber, filepositions, times, &duration)
			r.writeTailer(writeTime)
			filepositions = []uint64{0}
			times = []float64{0}
			offset = 0
			if err = r.createStream(writeTime); err != nil {
				return
			}
			if vr := suber.VideoReader; vr != nil {
				vr.ResetAbsTime()
				seq := vr.Track.ICodecCtx.(pkg.ISequenceCodecCtx[*rtmp.VideoFrame]).GetSequenceFrame()
				err = r.writer.WriteTag(FLV_TAG_TYPE_VIDEO, 0, uint32(seq.Size), seq.Buffers...)
				offset = int64(seq.Size + 15)
			}
			if ar := suber.AudioReader; ar != nil {
				ar.ResetAbsTime()
				if seqCtx, ok := ar.Track.ICodecCtx.(pkg.ISequenceCodecCtx[*rtmp.AudioFrame]); ok {
					seq := seqCtx.GetSequenceFrame()
					err = r.writer.WriteTag(FLV_TAG_TYPE_AUDIO, 0, uint32(seq.Size), seq.Buffers...)
					offset += int64(seq.Size + 15)
				}
			}
		}
	}

	return m7s.PlayBlock(ctx.Subscriber, func(audio *rtmp.AudioFrame) (err error) {
		if suber.VideoReader == nil && !noFragment {
			checkFragment(suber.AudioReader.AbsTime, suber.AudioReader.Value.WriteTime)
		}
		err = r.writer.WriteTag(FLV_TAG_TYPE_AUDIO, suber.AudioReader.AbsTime, uint32(audio.Size), audio.Buffers...)
		offset += int64(audio.Size + 15)
		return
	}, func(video *rtmp.VideoFrame) (err error) {
		if r.Event.StartTime.IsZero() {
			err = r.createStream(suber.VideoReader.Value.WriteTime)
			if err != nil {
				return err
			}
		}
		if suber.VideoReader.Value.IDR {
			filepositions = append(filepositions, uint64(offset))
			times = append(times, float64(suber.VideoReader.AbsTime)/1000)
			if !noFragment {
				checkFragment(suber.VideoReader.AbsTime, suber.VideoReader.Value.WriteTime)
			}
		}
		err = r.writer.WriteTag(FLV_TAG_TYPE_VIDEO, suber.VideoReader.AbsTime, uint32(video.Size), video.Buffers...)
		offset += int64(video.Size + 15)
		return
	})
}
