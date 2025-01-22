package hls

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"
	"m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	mpegts "m7s.live/v5/plugin/hls/pkg/ts"
)

func NewRecorder() m7s.IRecorder {
	return &Recorder{}
}

type Recorder struct {
	m7s.DefaultRecorder
	stream       m7s.RecordStream
	ts           *TsInFile
	pesAudio     *mpegts.MpegtsPESFrame
	pesVideo     *mpegts.MpegtsPESFrame
	segmentCount uint32
	lastTs       time.Duration
	firstSegment bool
}

var CustomFileName = func(job *m7s.RecordJob) string {
	if job.Fragment == 0 || job.Append {
		return fmt.Sprintf("%s/%s.ts", job.FilePath, time.Now().Format("20060102150405"))
	}
	return filepath.Join(job.FilePath, time.Now().Format("20060102150405")+".ts")
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
		Type:           "hls",
	}
	dir := filepath.Dir(r.stream.FilePath)
	dir = filepath.Clean(dir)
	if err = os.MkdirAll(dir, 0755); err != nil {
		r.Error("create directory failed", "err", err, "dir", dir)
		return
	}
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
		Type:       "hls",
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

func (r *Recorder) writeTailer(end time.Time) {
	if r.stream.EndTime.After(r.stream.StartTime) {
		return
	}
	r.stream.EndTime = end
	if r.RecordJob.Plugin.DB != nil {
		r.RecordJob.Plugin.DB.Save(&r.stream)
	}
}

func (r *Recorder) Dispose() {
	// 如果当前有未完成的片段，先保存
	if r.ts != nil {
		r.ts.Close()
	}
	r.writeTailer(time.Now())
}

func (r *Recorder) createNewTs() {
	var oldPMT util.Buffer
	if r.ts != nil {
		oldPMT = r.ts.PMT
		r.ts.Close()
	}
	var err error
	r.ts, err = NewTsInFile(r.stream.FilePath)
	if err != nil {
		r.Error("create ts file failed", "err", err, "path", r.stream.FilePath)
		return
	}
	if oldPMT.Len() > 0 {
		r.ts.PMT = oldPMT
	}
}

func (r *Recorder) writeSegment(ts time.Duration) (err error) {
	if dur := ts - r.lastTs; dur >= r.RecordJob.Fragment || r.lastTs == 0 {
		if dur == ts && r.lastTs == 0 { //时间戳不对的情况，首个默认为2s
			dur = time.Duration(2) * time.Second
		}

		// 如果是第一个片段，跳过写入，只记录时间戳
		if r.firstSegment {
			r.lastTs = ts
			r.firstSegment = false
			return nil
		}

		// 结束当前片段的记录
		r.writeTailer(time.Now())

		// 创建新的数据库记录
		err = r.createStream(time.Now())
		if err != nil {
			return
		}

		// 创建新的ts文件
		r.createNewTs()
		r.segmentCount++
		r.lastTs = ts
	}
	return
}

func (r *Recorder) Run() (err error) {
	ctx := &r.RecordJob
	suber := ctx.Subscriber
	startTime := time.Now()
	if ctx.BeforeDuration > 0 {
		startTime = startTime.Add(-ctx.BeforeDuration)
	}

	// 创建第一个片段记录
	if err = r.createStream(startTime); err != nil {
		return
	}

	// 初始化HLS相关结构
	r.createNewTs()
	r.pesAudio = &mpegts.MpegtsPESFrame{
		Pid: mpegts.PID_AUDIO,
	}
	r.pesVideo = &mpegts.MpegtsPESFrame{
		Pid: mpegts.PID_VIDEO,
	}
	r.firstSegment = true

	var audioCodec, videoCodec codec.FourCC
	if suber.Publisher.HasAudioTrack() {
		audioCodec = suber.Publisher.AudioTrack.FourCC()
	}
	if suber.Publisher.HasVideoTrack() {
		videoCodec = suber.Publisher.VideoTrack.FourCC()
	}
	r.ts.WritePMTPacket(audioCodec, videoCodec)

	return m7s.PlayBlock(suber, r.ProcessADTS, r.ProcessAnnexB)
}

func (r *Recorder) ProcessADTS(audio *pkg.ADTS) (err error) {
	return r.ts.WriteAudioFrame(audio, r.pesAudio)
}

func (r *Recorder) ProcessAnnexB(video *pkg.AnnexB) (err error) {
	if r.RecordJob.Subscriber.VideoReader.Value.IDR {
		if err = r.writeSegment(video.GetTimestamp()); err != nil {
			return
		}
	}
	return r.ts.WriteVideoFrame(video, r.pesVideo)
}
