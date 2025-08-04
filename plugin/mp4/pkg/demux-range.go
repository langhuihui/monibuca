package mp4

import (
	"context"
	"log/slog"
	"net"
	"os"
	"reflect"
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/plugin/mp4/pkg/box"
)

type DemuxerRange struct {
	*slog.Logger
	StartTime, EndTime     time.Time
	Streams                []m7s.RecordStream
	AudioCodec, VideoCodec codec.ICodecCtx
	OnAudio, OnVideo       func(box.Sample) error
	OnCodec                func(codec.ICodecCtx, codec.ICodecCtx)
}

func (d *DemuxerRange) Demux(ctx context.Context) error {
	var ts, tsOffset int64
	for _, stream := range d.Streams {
		// 检查流的时间范围是否在指定范围内
		if stream.EndTime.Before(d.StartTime) || stream.StartTime.After(d.EndTime) {
			continue
		}

		tsOffset = ts
		file, err := os.Open(stream.FilePath)
		if err != nil {
			continue
		}
		defer file.Close()

		demuxer := NewDemuxer(file)
		if err = demuxer.Demux(); err != nil {
			return err
		}

		// 处理每个轨道的额外数据 (序列头)
		for _, track := range demuxer.Tracks {
			if track.Cid.IsAudio() {
				d.AudioCodec = track.ICodecCtx
			} else {
				d.VideoCodec = track.ICodecCtx
			}
		}
		if d.OnCodec != nil {
			d.OnCodec(d.AudioCodec, d.VideoCodec)
		}

		// 计算起始时间戳偏移
		if !d.StartTime.IsZero() {
			startTimestamp := d.StartTime.Sub(stream.StartTime).Milliseconds()
			if startTimestamp < 0 {
				startTimestamp = 0
			}
			if startSample, err := demuxer.SeekTime(uint64(startTimestamp)); err == nil {
				tsOffset = -int64(startSample.Timestamp)
			} else {
				tsOffset = 0
			}
		}

		// 读取和处理样本
		for track, sample := range demuxer.ReadSample {
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			// 检查是否超出结束时间
			sampleTime := stream.StartTime.Add(time.Duration(sample.Timestamp) * time.Millisecond)
			if !d.EndTime.IsZero() && sampleTime.After(d.EndTime) {
				break
			}

			// 计算样本数据偏移和读取数据
			sampleOffset := int(sample.Offset) - int(demuxer.mdatOffset)
			if sampleOffset < 0 || sampleOffset+sample.Size > len(demuxer.mdat.Data) {
				continue
			}
			data := demuxer.mdat.Data[sampleOffset : sampleOffset+sample.Size]
			sample.Buffers = net.Buffers{data}

			// 计算时间戳
			if int64(sample.Timestamp)+tsOffset < 0 {
				ts = 0
			} else {
				ts = int64(sample.Timestamp + uint32(tsOffset))
			}
			sample.Timestamp = uint32(ts)
			if track.Cid.IsAudio() {
				if err := d.OnAudio(sample); err != nil {
					return err
				}
			} else {
				if err := d.OnVideo(sample); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

type DemuxerConverterRange[TA pkg.IAVFrame, TV pkg.IAVFrame] struct {
	DemuxerRange
	OnAudio func(TA) error
	OnVideo func(TV) error
}

func (d *DemuxerConverterRange[TA, TV]) Demux(ctx context.Context) error {
	var targetAudio TA
	var targetVideo TV

	targetAudioType, targetVideoType := reflect.TypeOf(targetAudio).Elem(), reflect.TypeOf(targetVideo).Elem()
	d.DemuxerRange.OnAudio = func(audio box.Sample) error {
		targetAudio = reflect.New(targetAudioType).Interface().(TA) // TODO: reuse
		var audioFrame AudioFrame
		audioFrame.ICodecCtx = d.AudioCodec
		audioFrame.BaseSample = &pkg.BaseSample{}
		audioFrame.Raw = &audio.Memory
		audioFrame.SetTS32(audio.Timestamp)
		err := pkg.ConvertFrameType(&audioFrame, targetAudio)
		if err == nil {
			err = d.OnAudio(targetAudio)
		}
		return err
	}
	d.DemuxerRange.OnVideo = func(video box.Sample) error {
		targetVideo = reflect.New(targetVideoType).Interface().(TV) // TODO: reuse
		var videoFrame VideoFrame
		videoFrame.ICodecCtx = d.VideoCodec
		videoFrame.BaseSample = &pkg.BaseSample{}
		videoFrame.Raw = &video.Memory
		videoFrame.SetTS32(video.Timestamp)
		videoFrame.CTS = time.Duration(video.CTS) / time.Millisecond
		err := pkg.ConvertFrameType(&videoFrame, targetVideo)
		if err == nil {
			err = d.OnVideo(targetVideo)
		}
		return err
	}
	return d.DemuxerRange.Demux(ctx)
}
