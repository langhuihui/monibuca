package mp4

import (
	"errors"
	"io"
	"slices"
	"strings"
	"time"

	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/util"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
)

type HTTPReader struct {
	m7s.HTTPFilePuller
}

func (p *HTTPReader) Run() (err error) {
	pullJob := &p.PullJob
	publisher := pullJob.Publisher
	if publisher == nil {
		io.Copy(io.Discard, p.ReadCloser)
		return
	}
	allocator := util.NewScalableMemoryAllocator(1 << 10)
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

	// 设置RTMP分配器以启用RTMP帧收集
	demuxer.RTMPAllocator = allocator

	if err = demuxer.DemuxWithAllocator(allocator); err != nil {
		return
	}

	// 获取demuxer内部收集的RTMP帧
	rtmpFrames := demuxer.RTMPFrames

	// 按时间戳排序所有帧
	slices.SortFunc(rtmpFrames, func(a, b RTMPFrame) int {
		var timeA, timeB uint64
		switch f := a.Frame.(type) {
		case *rtmp.RTMPVideo:
			timeA = uint64(f.Timestamp)
		case *rtmp.RTMPAudio:
			timeA = uint64(f.Timestamp)
		}
		switch f := b.Frame.(type) {
		case *rtmp.RTMPVideo:
			timeB = uint64(f.Timestamp)
		case *rtmp.RTMPAudio:
			timeB = uint64(f.Timestamp)
		}
		if timeA < timeB {
			return -1
		} else if timeA > timeB {
			return 1
		}
		return 0
	})

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

	// 读取预生成的 RTMP 序列帧
	videoSeq, audioSeq := demuxer.GetRTMPSequenceFrames()
	if videoSeq != nil {
		err = publisher.WriteVideo(videoSeq)
		if err != nil {
			return err
		}
	}
	if audioSeq != nil {
		err = publisher.WriteAudio(audioSeq)
		if err != nil {
			return err
		}
	}

	// 计算最大时间戳用于累计偏移
	var maxTimestamp uint64
	for _, frame := range rtmpFrames {
		var timestamp uint64
		switch f := frame.Frame.(type) {
		case *rtmp.RTMPVideo:
			timestamp = uint64(f.Timestamp)
		case *rtmp.RTMPAudio:
			timestamp = uint64(f.Timestamp)
		}
		if timestamp > maxTimestamp {
			maxTimestamp = timestamp
		}
	}

	var timestampOffset uint64
	loop := p.PullJob.Loop
	for {
		// 使用预生成的 RTMP 帧进行播放
		for _, frame := range rtmpFrames {
			if p.IsStopped() {
				return nil
			}

			// 应用时间戳偏移
			switch f := frame.Frame.(type) {
			case *rtmp.RTMPVideo:
				f.Timestamp += uint32(timestampOffset)
				err = publisher.WriteVideo(f)
			case *rtmp.RTMPAudio:
				f.Timestamp += uint32(timestampOffset)
				err = publisher.WriteAudio(f)
			}

			if err != nil {
				return err
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
