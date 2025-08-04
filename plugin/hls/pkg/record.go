package hls

import (
	"fmt"
	"path/filepath"
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/format"
	mpegts "m7s.live/v5/pkg/format/ts"
)

func NewRecorder(conf config.Record) m7s.IRecorder {
	return &Recorder{}
}

type Recorder struct {
	m7s.DefaultRecorder
	ts           TsInFile
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
	r.RecordJob.RecConf.Type = "ts"
	return r.CreateStream(start, CustomFileName)
}

func (r *Recorder) writeTailer(end time.Time) {
	if !r.RecordJob.RecConf.RealTime {
		if r.ts.file != nil {
			r.ts.WriteTo(r.ts.file)
			r.ts.Recycle()
		}
	}
	r.ts.Close()
	r.WriteTail(end, nil)
}

func (r *Recorder) Dispose() {
	r.writeTailer(time.Now())
}

func (r *Recorder) createNewTs() (err error) {
	if r.RecordJob.RecConf.RealTime {
		if err = r.ts.Open(r.Event.FilePath); err != nil {
			r.Error("create ts file failed", "err", err, "path", r.Event.FilePath)
		}
	} else {
		r.ts.path = r.Event.FilePath
	}
	return
}

func (r *Recorder) writeSegment(ts time.Duration, writeTime time.Time) (err error) {
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
		r.writeTailer(writeTime)

		// 创建新的数据库记录
		err = r.createStream(writeTime)
		if err != nil {
			return
		}

		// 创建新的ts文件
		if err = r.createNewTs(); err != nil {
			return
		}
		if r.RecordJob.RecConf.RealTime {
			r.ts.file.Write(mpegts.DefaultPATPacket)
			r.ts.file.Write(r.ts.PMT)
		}
		r.segmentCount++
		r.lastTs = ts
	}
	return
}

func (r *Recorder) Run() (err error) {
	ctx := &r.RecordJob
	suber := ctx.Subscriber
	startTime := time.Now()
	// 创建第一个片段记录
	if err = r.createStream(startTime); err != nil {
		return
	}

	// 初始化HLS相关结构
	if err = r.createNewTs(); err != nil {
		return
	}
	pesAudio, pesVideo := mpegts.CreatePESWriters()
	r.firstSegment = true

	var audioCodec, videoCodec codec.FourCC
	if suber.Publisher.HasAudioTrack() {
		audioCodec = suber.Publisher.AudioTrack.FourCC()
	}
	if suber.Publisher.HasVideoTrack() {
		videoCodec = suber.Publisher.VideoTrack.FourCC()
	}
	r.ts.WritePMTPacket(audioCodec, videoCodec)
	if ctx.RecConf.RealTime {
		r.ts.file.Write(mpegts.DefaultPATPacket)
		r.ts.file.Write(r.ts.PMT)
	}
	return m7s.PlayBlock(suber, func(audio *format.Mpeg2Audio) (err error) {
		pesAudio.Pts = uint64(suber.AudioReader.AbsTime) * 90
		err = pesAudio.WritePESPacket(audio.Memory, &r.ts.RecyclableMemory)
		if err == nil {
			if ctx.RecConf.RealTime {
				r.ts.RecyclableMemory.WriteTo(r.ts.file)
				r.ts.RecyclableMemory.Recycle()
			}
		}
		return
	}, func(video *mpegts.VideoFrame) (err error) {
		vr := r.RecordJob.Subscriber.VideoReader
		if vr.Value.IDR {
			if err = r.writeSegment(video.Timestamp, vr.Value.WriteTime); err != nil {
				return
			}
		}
		pesVideo.IsKeyFrame = video.IDR
		pesVideo.Pts = uint64(vr.AbsTime+video.GetCTS32()) * 90
		pesVideo.Dts = uint64(vr.AbsTime) * 90
		err = pesVideo.WritePESPacket(video.Memory, &r.ts.RecyclableMemory)
		if err == nil {
			if ctx.RecConf.RealTime {
				r.ts.RecyclableMemory.WriteTo(r.ts.file)
				r.ts.RecyclableMemory.Recycle()
			}
		}
		return
	})
}
