package mp4

import (
	"strings"
	"time"

	"github.com/langhuihui/gomem"
	task "github.com/langhuihui/gotask"
	m7s "m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/util"
	"m7s.live/v5/plugin/mp4/pkg/box"
)

type (
	RecordReader struct {
		m7s.RecordFilePuller
	}
)

func NewPuller(conf config.Pull) m7s.IPuller {
	if strings.HasPrefix(conf.URL, "http") || strings.HasSuffix(conf.URL, ".mp4") {
		p := &HTTPReader{}
		p.SetDescription(task.OwnerTypeKey, "Mp4Reader")
		return p
	}
	if conf.Args.Get(util.StartKey) != "" {
		p := &RecordReader{}
		p.Type = "mp4"
		p.SetDescription(task.OwnerTypeKey, "Mp4RecordReader")
		return p
	}
	return nil
}

func (p *RecordReader) Run() (err error) {
	pullJob := &p.PullJob
	publisher := pullJob.Publisher
	if publisher == nil {
		return pkg.ErrDisabled
	}

	var realTime time.Time
	publisher.OnGetPosition = func() time.Time {
		return realTime
	}

	allocator := gomem.NewScalableMemoryAllocator(1 << gomem.MinPowerOf2)
	defer allocator.Recycle()
	// 创建 PublishWriter
	var writer m7s.PublishWriter[*AudioFrame, *VideoFrame]

	// 创建可复用的 DemuxerRange 实例
	demuxerRange := DemuxerRange{
		Logger:  p.Logger.With("demuxer", "mp4"),
		Streams: p.Streams,
		OnCodec: func(audio, video codec.ICodecCtx) {
			if audio != nil {
				writer.PublishAudioWriter = m7s.NewPublishAudioWriter[*AudioFrame](publisher, allocator)
			}
			if video != nil {
				writer.PublishVideoWriter = m7s.NewPublishVideoWriter[*VideoFrame](publisher, allocator)
			}
		},
		storage: pullJob.Plugin.Server.Storage,
	}
	demuxerRange.OnAudio = func(a box.Sample) error {
		if publisher.Paused != nil {
			publisher.Paused.Await()
		}
		frame := writer.AudioFrame
		frame.ICodecCtx = demuxerRange.AudioCodec
		// 检查是否需要跳转
		if needSeek, seekErr := p.CheckSeek(); seekErr != nil {
			return seekErr
		} else if needSeek {
			return pkg.ErrSkip
		}
		frame.Memory = a.Memory
		// DemuxerRange 已经处理了时间戳偏移，直接使用
		frame.SetTS32(a.Timestamp)
		return writer.NextAudio()
	}
	demuxerRange.OnVideo = func(v box.Sample) error {
		if publisher.Paused != nil {
			publisher.Paused.Await()
		}
		frame := writer.VideoFrame
		frame.ICodecCtx = demuxerRange.VideoCodec
		// 检查是否需要跳转
		if needSeek, seekErr := p.CheckSeek(); seekErr != nil {
			return seekErr
		} else if needSeek {
			return pkg.ErrSkip
		}

		frame.Memory = v.Memory
		// 更新实时时间
		realTime = time.Now() // 这里可以根据需要调整为更精确的时间计算
		// DemuxerRange 已经处理了时间戳偏移，直接使用
		frame.SetTS32(v.Timestamp)
		frame.IDR = v.KeyFrame
		frame.CTS = time.Duration(v.CTS) * time.Millisecond
		return writer.NextVideo()
	}

	for loop := 0; loop < p.Loop; loop++ {
		demuxerRange.StartTime = p.PullStartTime
		if !p.PullEndTime.IsZero() {
			demuxerRange.EndTime = p.PullEndTime
		} else if p.MaxTS > 0 {
			demuxerRange.EndTime = p.PullStartTime.Add(time.Duration(p.MaxTS) * time.Millisecond)
		} else {
			demuxerRange.EndTime = time.Now()
		}

		if err = demuxerRange.Demux(p.Context); err != nil {
			if err == pkg.ErrSkip {
				loop--
				continue
			}
			return err
		}
	}
	return
}
