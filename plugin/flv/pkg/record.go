package flv

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	task "github.com/langhuihui/gotask"
	"m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/storage"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
)

// MetaData 结构体保存 writeMetaTag 需要的编解码器元数据
type MetaData struct {
	// 音频相关
	HasAudio        bool
	AudioCodecID    int
	AudioSampleRate int
	AudioSampleSize int
	AudioChannels   int

	// 视频相关
	HasVideo     bool
	VideoCodecID int
	VideoWidth   int
	VideoHeight  int
	VideoFPS     int
	VideoBPS     int

	// 日志记录器
	Logger *slog.Logger
}

type WriteFlvMetaTagQueueTask struct {
	task.Work
}

var writeMetaTagQueueTask WriteFlvMetaTagQueueTask

func init() {
	m7s.Servers.AddTask(&writeMetaTagQueueTask)
}

type writeMetaTagTask struct {
	task.Task
	file     storage.File
	writer   *FlvWriter
	flags    byte
	metaData []byte
}

func (task *writeMetaTagTask) Start() (err error) {
	defer func() {
		err = task.file.Close()
		if info, err := task.file.Stat(); err == nil && info.Size() == 0 {
			err = os.Remove(info.Name())
			if err != nil {
				task.Error("writeMetaTagTask", "remove file err", err)
			}
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

func writeMetaTag(file storage.File, metadata *MetaData, filepositions []uint64, times []float64, duration *int64) {
	hasAudio, hasVideo := metadata.HasAudio, metadata.HasVideo
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
		flags |= (1 << 2)
		metaData["audiocodecid"] = metadata.AudioCodecID
		metaData["audiosamplerate"] = metadata.AudioSampleRate
		metaData["audiosamplesize"] = metadata.AudioSampleSize
		metaData["stereo"] = metadata.AudioChannels == 2
	}
	if hasVideo {
		flags |= 1
		metaData["videocodecid"] = metadata.VideoCodecID
		metaData["width"] = metadata.VideoWidth
		metaData["height"] = metadata.VideoHeight
		metaData["framerate"] = metadata.VideoFPS
		metaData["videodatarate"] = metadata.VideoBPS
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
	wrTask := &writeMetaTagTask{
		file:     file,
		flags:    flags,
		metaData: marshals,
	}
	wrTask.Logger = metadata.Logger.With("file", file.Name())
	writeMetaTagQueueTask.AddTask(wrTask)
}

func NewRecorder(conf config.Record) m7s.IRecorder {
	return &Recorder{}
}

type Recorder struct {
	m7s.DefaultRecorder
	writer   *FlvWriter
	file     storage.File
	metadata *MetaData // 保存编解码器元数据，避免在OnStop回调时指针失效
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

	// 获取存储实例
	st := r.RecordJob.GetStorage()

	if st == nil {
		return fmt.Errorf("global storage is nil")
	}
	// 使用存储抽象层
	r.file, err = st.CreateFile(context.Background(), r.Event.FilePath)
	if err != nil {
		return
	}
	r.writer = NewFlvWriter(r.file)

	_, err = r.writer.Write(FLVHead)
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
	suber.OnStop(func() {
		writeMetaTag(r.file, r.metadata, filepositions, times, &duration)
	})
	checkFragment := func(absTime uint32, writeTime time.Time) {
		if duration = int64(absTime); time.Duration(duration)*time.Millisecond >= ctx.RecConf.Fragment {
			writeMetaTag(r.file, r.metadata, filepositions, times, &duration)
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
		// 初始化元数据结构体（如果还没有）
		if r.metadata == nil {
			r.metadata = &MetaData{Logger: suber.Logger}
		}

		// 如果还没有设置音频参数，并且当前有音频流，则设置音频参数
		if !r.metadata.HasAudio && suber.AudioReader != nil {
			r.metadata.HasAudio = true
			audioCtx := suber.AudioReader.Track.ICodecCtx.GetBase().(pkg.IAudioCodecCtx)
			r.metadata.AudioCodecID = int(rtmp.ParseAudioCodec(audioCtx.FourCC()))
			r.metadata.AudioSampleRate = audioCtx.GetSampleRate()
			r.metadata.AudioSampleSize = audioCtx.GetSampleSize()
			r.metadata.AudioChannels = audioCtx.GetChannels()
		}

		if suber.VideoReader == nil && !noFragment {
			checkFragment(suber.AudioReader.AbsTime, suber.AudioReader.Value.WriteTime)
		}
		err = r.writer.WriteTag(FLV_TAG_TYPE_AUDIO, suber.AudioReader.AbsTime, uint32(audio.Size), audio.Buffers...)
		offset += int64(audio.Size + 15)
		return
	}, func(video *rtmp.VideoFrame) (err error) {
		// 初始化元数据结构体（如果还没有）
		if r.metadata == nil {
			r.metadata = &MetaData{Logger: suber.Logger}
		}

		if r.Event.StartTime.IsZero() {
			err = r.createStream(suber.VideoReader.Value.WriteTime)
			if err != nil {
				return err
			}
		}

		// 如果还没有设置视频参数，并且当前有视频流，则设置视频参数
		if !r.metadata.HasVideo && suber.VideoReader != nil {
			r.metadata.HasVideo = true
			videoCtx := suber.VideoReader.Track.ICodecCtx.GetBase().(pkg.IVideoCodecCtx)
			r.metadata.VideoCodecID = int(rtmp.ParseVideoCodec(videoCtx.FourCC()))
			r.metadata.VideoWidth = videoCtx.Width()
			r.metadata.VideoHeight = videoCtx.Height()
			r.metadata.VideoFPS = suber.VideoReader.Track.FPS
			r.metadata.VideoBPS = suber.VideoReader.Track.BPS
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
