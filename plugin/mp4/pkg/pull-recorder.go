package mp4

import (
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"m7s.live/v5/pkg"

	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/util"
	"m7s.live/v5/plugin/mp4/pkg/box"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
)

type (
	RecordReader struct {
		m7s.RecordFilePuller
		demuxer *Demuxer
	}
)

func NewPuller(conf config.Pull) m7s.IPuller {
	if strings.HasPrefix(conf.URL, "http") || strings.HasSuffix(conf.URL, ".mp4") {
		return &HTTPReader{}
	}
	if conf.Args.Get(util.StartKey) != "" {
		return &RecordReader{}
	}
	return nil
}

func (p *RecordReader) Run() (err error) {
	pullJob := &p.PullJob
	pullStartTime := p.PullStartTime
	publisher := pullJob.Publisher
	publisher.Type = m7s.PublishTypeVod
	allocator := util.NewScalableMemoryAllocator(1 << 10)
	var ts, tsOffset int64
	var realTime time.Time
	defer allocator.Recycle()
	publisher.OnSeek = func(seekTime time.Time) {
		pullStartTime = seekTime
		p.SetRetry(1, 0)
		if util.UnixTimeReg.MatchString(pullJob.Args.Get(util.EndKey)) {
			pullJob.Args.Set(util.StartKey, strconv.FormatInt(pullStartTime.Unix(), 10))
		} else {
			pullJob.Args.Set(util.StartKey, pullStartTime.Local().Format(util.LocalTimeFormat))
		}
		publisher.Stop(pkg.ErrSeek)
	}
	publisher.OnGetPosition = func() time.Time {
		return realTime
	}
	for i, stream := range p.Streams {
		tsOffset = ts
		if p.File != nil {
			p.File.Close()
		}
		p.File, err = os.Open(stream.FilePath)
		if err != nil {
			continue
		}
		p.demuxer = NewDemuxer(p.File)
		if err = p.demuxer.Demux(); err != nil {
			return
		}
		if i == 0 {
			for _, track := range p.demuxer.Tracks {
				switch track.Cid {
				case box.MP4_CODEC_H264:
					var sequence rtmp.RTMPVideo
					sequence.SetAllocator(allocator)
					sequence.Append([]byte{0x17, 0x00, 0x00, 0x00, 0x00}, track.ExtraData)
					err = publisher.WriteVideo(&sequence)
				case box.MP4_CODEC_H265:
					var sequence rtmp.RTMPVideo
					sequence.SetAllocator(allocator)
					sequence.Append([]byte{0b1001_0000 | rtmp.PacketTypeSequenceStart}, codec.FourCC_H265[:], track.ExtraData)
					err = publisher.WriteVideo(&sequence)
				case box.MP4_CODEC_AAC:
					var sequence rtmp.RTMPAudio
					sequence.SetAllocator(allocator)
					sequence.Append([]byte{0xaf, 0x00}, track.ExtraData)
					err = publisher.WriteAudio(&sequence)
				}
			}
			startTimestamp := pullStartTime.Sub(stream.StartTime).Milliseconds()
			if startTimestamp < 0 {
				startTimestamp = 0
			}
			var startSample *box.Sample
			if startSample, err = p.demuxer.SeekTime(uint64(startTimestamp)); err != nil {
				tsOffset = 0
				continue
			}
			tsOffset = -int64(startSample.DTS)
		}

		for track, sample := range p.demuxer.ReadSample {
			if p.IsStopped() {
				return p.StopReason()
			}
			if publisher.Paused != nil {
				publisher.Paused.Await()
			}

			if _, err = p.demuxer.reader.Seek(sample.Offset, io.SeekStart); err != nil {
				return
			}
			sample.Data = allocator.Malloc(sample.Size)
			if _, err = io.ReadFull(p.demuxer.reader, sample.Data); err != nil {
				allocator.Free(sample.Data)
				return
			}
			ts = int64(sample.DTS + uint64(tsOffset))
			realTime = stream.StartTime.Add(time.Duration(sample.DTS) * time.Millisecond)
			if p.MaxTS > 0 && ts > p.MaxTS {
				return
			}
			switch track.Cid {
			case box.MP4_CODEC_H264:
				var videoFrame rtmp.RTMPVideo
				videoFrame.SetAllocator(allocator)
				videoFrame.CTS = uint32(sample.PTS - sample.DTS)
				videoFrame.Timestamp = uint32(ts)
				videoFrame.AppendOne([]byte{util.Conditional[byte](sample.KeyFrame, 0x17, 0x27), 0x01, byte(videoFrame.CTS >> 24), byte(videoFrame.CTS >> 8), byte(videoFrame.CTS)})
				videoFrame.AddRecycleBytes(sample.Data)
				err = publisher.WriteVideo(&videoFrame)
			case box.MP4_CODEC_H265:
				var videoFrame rtmp.RTMPVideo
				videoFrame.SetAllocator(allocator)
				videoFrame.CTS = uint32(sample.PTS - sample.DTS)
				videoFrame.Timestamp = uint32(ts)
				var head []byte
				var b0 byte = 0b1010_0000
				if sample.KeyFrame {
					b0 = 0b1001_0000
				}
				if videoFrame.CTS == 0 {
					head = videoFrame.NextN(5)
					head[0] = b0 | rtmp.PacketTypeCodedFramesX
				} else {
					head = videoFrame.NextN(8)
					head[0] = b0 | rtmp.PacketTypeCodedFrames
					util.PutBE(head[5:8], videoFrame.CTS) // cts
				}
				copy(head[1:], codec.FourCC_H265[:])
				videoFrame.AddRecycleBytes(sample.Data)
				err = publisher.WriteVideo(&videoFrame)
			case box.MP4_CODEC_AAC:
				var audioFrame rtmp.RTMPAudio
				audioFrame.SetAllocator(allocator)
				audioFrame.Timestamp = uint32(ts)
				audioFrame.AppendOne([]byte{0xaf, 0x01})
				audioFrame.AddRecycleBytes(sample.Data)
				err = publisher.WriteAudio(&audioFrame)
			case box.MP4_CODEC_G711A:
				var audioFrame rtmp.RTMPAudio
				audioFrame.SetAllocator(allocator)
				audioFrame.Timestamp = uint32(ts)
				audioFrame.AppendOne([]byte{0x72})
				audioFrame.AddRecycleBytes(sample.Data)
				err = publisher.WriteAudio(&audioFrame)
			case box.MP4_CODEC_G711U:
				var audioFrame rtmp.RTMPAudio
				audioFrame.SetAllocator(allocator)
				audioFrame.Timestamp = uint32(ts)
				audioFrame.AppendOne([]byte{0x82})
				audioFrame.AddRecycleBytes(sample.Data)
				err = publisher.WriteAudio(&audioFrame)
			}
		}
	}
	return
}
