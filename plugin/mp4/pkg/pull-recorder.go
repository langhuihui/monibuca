package mp4

import (
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/deepch/vdk/codec/h265parser"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
	"m7s.live/m7s/v5/plugin/mp4/pkg/box"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
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
	return &RecordReader{}
}

func (p *RecordReader) Run() (err error) {
	pullJob := &p.PullJob
	publisher := pullJob.Publisher
	allocator := util.NewScalableMemoryAllocator(1 << 10)
	var ts, tsOffset int64
	defer allocator.Recycle()
	publisher.OnSeek = func(seekTime time.Duration) {
		targetTime := p.PullStartTime.Add(time.Duration(ts) * time.Millisecond).Add(seekTime)
		p.Stop(errors.New("seek"))
		pullJob.Args.Set("start", targetTime.Local().Format("2006-01-02T15:04:05"))
		newRecordReader := &RecordReader{}
		pullJob.AddTask(newRecordReader)
	}
	for i, stream := range p.Streams {
		tsOffset = ts
		p.File, err = os.Open(stream.FilePath)
		if err != nil {
			return
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
			startTimestamp := p.PullStartTime.Sub(stream.StartTime).Milliseconds()
			if _, err = p.demuxer.SeekTime(uint64(startTimestamp)); err != nil {
				return
			}
			tsOffset = -startTimestamp
		}

		for track, sample := range p.demuxer.ReadSample {
			if p.IsStopped() {
				break
			}
			if _, err = p.demuxer.reader.Seek(int64(sample.Offset), io.SeekStart); err != nil {
				return
			}
			sample.Data = allocator.Malloc(int(sample.Size))
			if _, err = io.ReadFull(p.demuxer.reader, sample.Data); err != nil {
				allocator.Free(sample.Data)
				return
			}
			ts = int64(sample.DTS + uint64(tsOffset))
			switch track.Cid {
			case box.MP4_CODEC_H264:
				keyFrame := codec.ParseH264NALUType(sample.Data[5]) == codec.NALU_IDR_Picture
				var videoFrame rtmp.RTMPVideo
				videoFrame.SetAllocator(allocator)
				videoFrame.CTS = uint32(sample.PTS - sample.DTS)
				videoFrame.Timestamp = uint32(ts)
				videoFrame.AppendOne([]byte{util.Conditional[byte](keyFrame, 0x17, 0x27), 0x01, byte(videoFrame.CTS >> 24), byte(videoFrame.CTS >> 8), byte(videoFrame.CTS)})
				videoFrame.AddRecycleBytes(sample.Data)
				err = publisher.WriteVideo(&videoFrame)
			case box.MP4_CODEC_H265:
				var keyFrame bool
				switch codec.ParseH265NALUType(sample.Data[5]) {
				case h265parser.NAL_UNIT_CODED_SLICE_BLA_W_LP,
					h265parser.NAL_UNIT_CODED_SLICE_BLA_W_RADL,
					h265parser.NAL_UNIT_CODED_SLICE_BLA_N_LP,
					h265parser.NAL_UNIT_CODED_SLICE_IDR_W_RADL,
					h265parser.NAL_UNIT_CODED_SLICE_IDR_N_LP,
					h265parser.NAL_UNIT_CODED_SLICE_CRA:
					keyFrame = true
				}
				var videoFrame rtmp.RTMPVideo
				videoFrame.SetAllocator(allocator)
				videoFrame.CTS = uint32(sample.PTS - sample.DTS)
				videoFrame.Timestamp = uint32(ts)
				var head []byte
				var b0 byte = 0b1010_0000
				if keyFrame {
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
