package mp4

import (
	"errors"
	"io"
	"strings"
	"time"

	m7s "m7s.live/v5"
	pkg "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/util"
)

type HTTPReader struct {
	m7s.HTTPFilePuller
}

func (p *HTTPReader) Run() (err error) {
	pullJob := &p.PullJob

	// Move to parsing step
	pullJob.GoToStepConst(pkg.StepParsing)
	publisher := pullJob.Publisher
	if publisher == nil {
		io.Copy(io.Discard, p.ReadCloser)
		return
	}
	allocator := util.NewScalableMemoryAllocator(1 << util.MinPowerOf2)
	var demuxer *Demuxer
	defer allocator.Recycle()
	switch v := p.ReadCloser.(type) {
	case io.ReadSeeker:
		demuxer = NewDemuxer(v)
	default:
		var content []byte
		content, err = io.ReadAll(p.ReadCloser)
		demuxer = NewDemuxer(strings.NewReader(string(content)))
	}
	if err = demuxer.Demux(); err != nil {
		return
	}
	publisher.OnSeek = func(seekTime time.Time) {
		p.Stop(errors.New("seek"))
		pullJob.Connection.Args.Set(util.StartKey, seekTime.Local().Format(util.LocalTimeFormat))
		newHTTPReader := &HTTPReader{}
		pullJob.AddTask(newHTTPReader)
	}
	if pullJob.Connection.Args.Get(util.StartKey) != "" {
		seekTime, _ := time.Parse(util.LocalTimeFormat, pullJob.Connection.Args.Get(util.StartKey))
		demuxer.SeekTime(uint64(seekTime.UnixMilli()))
	}
	var writer m7s.PublishWriter[*AudioFrame, *VideoFrame]

	for _, track := range demuxer.Tracks {
		if track.Cid.IsAudio() {
			writer.PublishAudioWriter = m7s.NewPublishAudioWriter[*AudioFrame](publisher, allocator)
			writer.AudioFrame.ICodecCtx = track.ICodecCtx
		} else {
			writer.PublishVideoWriter = m7s.NewPublishVideoWriter[*VideoFrame](publisher, allocator)
			writer.VideoFrame.ICodecCtx = track.ICodecCtx
		}
	}

	// Move to streaming step
	pullJob.GoToStepConst(pkg.StepStreaming)

	// 计算最大时间戳用于累计偏移
	var maxTimestamp uint64
	for track, sample := range demuxer.ReadSample {
		timestamp := uint64(sample.Timestamp) * 1000 / uint64(track.Timescale)
		if timestamp > maxTimestamp {
			maxTimestamp = timestamp
		}
	}
	var timestampOffset uint64
	loop := p.PullJob.Loop
	for {
		demuxer.ReadSampleIdx = make([]uint32, len(demuxer.Tracks))
		for track, sample := range demuxer.ReadSample {
			if p.IsStopped() {
				return
			}
			if _, err = demuxer.reader.Seek(sample.Offset, io.SeekStart); err != nil {
				pullJob.Fail(err.Error())
				return
			}
			fixTimestamp := uint32(uint64(sample.Timestamp)*1000/uint64(track.Timescale) + timestampOffset)
			if track.Cid.IsAudio() {
				if _, err = io.ReadFull(demuxer.reader, writer.AudioFrame.NextN(sample.Size)); err != nil {
					writer.AudioFrame.Recycle()
					pullJob.Fail(err.Error())
					return
				}
				writer.AudioFrame.ICodecCtx = track.ICodecCtx
				writer.AudioFrame.SetTS32(fixTimestamp)
				err = writer.NextAudio()
				if err != nil {
					pullJob.Fail(err.Error())
					return
				}
			} else {
				if _, err = io.ReadFull(demuxer.reader, writer.VideoFrame.NextN(sample.Size)); err != nil {
					writer.VideoFrame.Recycle()
					pullJob.Fail(err.Error())
					return
				}
				writer.VideoFrame.ICodecCtx = track.ICodecCtx
				writer.VideoFrame.SetTS32(fixTimestamp)
				writer.VideoFrame.CTS = time.Duration(sample.CTS) * time.Millisecond
				writer.VideoFrame.IDR = sample.KeyFrame
				err = writer.NextVideo()
				if err != nil {
					pullJob.Fail(err.Error())
					return
				}
			}
		}
		if loop >= 0 {
			loop--
			if loop == -1 {
				break
			}
		}
		// 每次循环后累计时间戳偏移，确保下次循环的时间戳是递增的
		timestampOffset += maxTimestamp + 1
	}
	return
}
