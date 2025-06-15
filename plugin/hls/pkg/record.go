package hls

import (
	"fmt"
	"path/filepath"
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/util"
	mpegts "m7s.live/v5/plugin/hls/pkg/ts"
)

func NewRecorder(conf config.Record) m7s.IRecorder {
	return &Recorder{}
}

type Recorder struct {
	m7s.DefaultRecorder
	ts           *TsInFile
	pesAudio     *mpegts.MpegtsPESFrame
	pesVideo     *mpegts.MpegtsPESFrame
	segmentCount uint32
	lastTs       time.Duration
	firstSegment bool
}

var CustomFileName = func(job *m7s.RecordJob) string {
	if job.RecConf.Fragment == 0 || job.RecConf.Append {
		return fmt.Sprintf("%s/%s.ts", job.RecConf.FilePath, time.Now().Format("20060102150405"))
	}
	return filepath.Join(job.RecConf.FilePath, time.Now().Format("20060102150405")+".ts")
}

func (r *Recorder) createStream(start time.Time) (err error) {
	return r.CreateStream(start, CustomFileName)
}

func (r *Recorder) writeTailer(end time.Time) {
	r.WriteTail(end, nil)
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
	r.ts, err = NewTsInFile(r.Event.FilePath)
	if err != nil {
		r.Error("create ts file failed", "err", err, "path", r.Event.FilePath)
		return
	}
	if oldPMT.Len() > 0 {
		r.ts.PMT = oldPMT
	}
}

func (r *Recorder) writeSegment(ts time.Duration) (err error) {
	if dur := ts - r.lastTs; dur >= r.RecordJob.RecConf.Fragment || r.lastTs == 0 {
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
	if ctx.Event.BeforeDuration > 0 {
		startTime = startTime.Add(-time.Duration(ctx.Event.BeforeDuration) * time.Millisecond)
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
