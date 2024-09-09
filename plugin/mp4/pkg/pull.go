package mp4

import (
	"errors"
	"fmt"
	"github.com/deepch/vdk/codec/h265parser"
	"io"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/util"
	"m7s.live/m7s/v5/plugin/mp4/pkg/box"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
	"os"
	"path/filepath"
	"strings"
)

type (
	RecordReader struct {
		m7s.RecordFilePuller
		demuxer *box.MovDemuxer
	}
	HTTPReader struct {
		m7s.HTTPFilePuller
	}
)

func NewPuller(conf config.Pull) m7s.IPuller {
	if strings.HasPrefix(conf.URL, "http") || strings.HasSuffix(conf.URL, ".mp4") {
		return &HTTPReader{}
	}
	return &RecordReader{}
}

func (p *HTTPReader) Run() (err error) {
	pullJob := &p.PullJob
	var demuxer *box.MovDemuxer
	allocator := util.NewScalableMemoryAllocator(1 << 10)
	defer allocator.Recycle()
	switch v := p.ReadCloser.(type) {
	case io.ReadSeeker:
		demuxer = box.CreateMp4Demuxer(v)
	default:
		var content []byte
		content, err = io.ReadAll(p.ReadCloser)
		demuxer = box.CreateMp4Demuxer(strings.NewReader(string(content)))
	}
	var tracks []box.TrackInfo
	if tracks, err = demuxer.ReadHead(); err != nil {
		return
	}
	publisher := pullJob.Publisher
	for _, track := range tracks {
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
	for !p.IsStopped() {
		pkg, err := demuxer.ReadPacket(allocator)
		if err != nil {
			pullJob.Error("Error reading MP4 packet", "err", err)
			return err
		}
		switch track := tracks[pkg.TrackId-1]; track.Cid {
		case box.MP4_CODEC_H264:
			var videoFrame rtmp.RTMPVideo
			videoFrame.SetAllocator(allocator)
			videoFrame.CTS = uint32(pkg.Pts - pkg.Dts)
			videoFrame.Timestamp = uint32(pkg.Dts)
			keyFrame := codec.H264NALUType(pkg.Data[5]&0x1F) == codec.NALU_IDR_Picture
			videoFrame.AppendOne([]byte{util.Conditoinal[byte](keyFrame, 0x17, 0x27), 0x01, byte(videoFrame.CTS >> 24), byte(videoFrame.CTS >> 8), byte(videoFrame.CTS)})
			videoFrame.AddRecycleBytes(pkg.Data)
			err = publisher.WriteVideo(&videoFrame)
		case box.MP4_CODEC_H265:
			var videoFrame rtmp.RTMPVideo
			videoFrame.SetAllocator(allocator)
			videoFrame.CTS = uint32(pkg.Pts - pkg.Dts)
			videoFrame.Timestamp = uint32(pkg.Dts)
			var head []byte
			var b0 byte = 0b1010_0000
			switch codec.ParseH265NALUType(pkg.Data[5]) {
			case h265parser.NAL_UNIT_CODED_SLICE_BLA_W_LP,
				h265parser.NAL_UNIT_CODED_SLICE_BLA_W_RADL,
				h265parser.NAL_UNIT_CODED_SLICE_BLA_N_LP,
				h265parser.NAL_UNIT_CODED_SLICE_IDR_W_RADL,
				h265parser.NAL_UNIT_CODED_SLICE_IDR_N_LP,
				h265parser.NAL_UNIT_CODED_SLICE_CRA:
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
			videoFrame.AddRecycleBytes(pkg.Data)
			err = publisher.WriteVideo(&videoFrame)
		case box.MP4_CODEC_AAC:
			var audioFrame rtmp.RTMPAudio
			audioFrame.SetAllocator(allocator)
			audioFrame.Timestamp = uint32(pkg.Dts)
			audioFrame.AppendOne([]byte{0xaf, 0x01})
			audioFrame.AddRecycleBytes(pkg.Data)
			err = publisher.WriteAudio(&audioFrame)
		}
	}
	return
}

func (p *RecordReader) Run() (err error) {
	pullJob := &p.PullJob
	publisher := pullJob.Publisher
	allocator := util.NewScalableMemoryAllocator(1 << 10)
	var ts int64
	var tsOffset int64
	defer allocator.Recycle()
	var firstKeyFrameSent bool
	for i, stream := range p.Streams {
		tsOffset = ts
		p.File, err = os.Open(filepath.Join(p.PullJob.RemoteURL, fmt.Sprintf("%d.mp4", stream.ID)))
		if err != nil {
			return
		}
		p.demuxer = box.CreateMp4Demuxer(p.File)
		var tracks []box.TrackInfo
		if tracks, err = p.demuxer.ReadHead(); err != nil {
			return
		}
		if i == 0 {
			for _, track := range tracks {
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
			if err = p.demuxer.SeekTime(uint64(startTimestamp)); err != nil {
				return
			}
			tsOffset = -startTimestamp
		}

		for !p.IsStopped() {
			pkg, err := p.demuxer.ReadPacket(allocator)
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				pullJob.Error("Error reading MP4 packet", "err", err)
				return err
			}
			ts = int64(pkg.Dts + uint64(tsOffset))
			switch track := tracks[pkg.TrackId-1]; track.Cid {
			case box.MP4_CODEC_H264:
				keyFrame := codec.ParseH264NALUType(pkg.Data[5]) == codec.NALU_IDR_Picture
				if !firstKeyFrameSent {
					if keyFrame {
						firstKeyFrameSent = true
					} else {
						continue
					}
				}
				var videoFrame rtmp.RTMPVideo
				videoFrame.SetAllocator(allocator)
				videoFrame.CTS = uint32(pkg.Pts - pkg.Dts)
				videoFrame.Timestamp = uint32(ts)
				videoFrame.AppendOne([]byte{util.Conditoinal[byte](keyFrame, 0x17, 0x27), 0x01, byte(videoFrame.CTS >> 24), byte(videoFrame.CTS >> 8), byte(videoFrame.CTS)})
				videoFrame.AddRecycleBytes(pkg.Data)
				err = publisher.WriteVideo(&videoFrame)
			case box.MP4_CODEC_H265:
				var keyFrame bool
				switch codec.ParseH265NALUType(pkg.Data[5]) {
				case h265parser.NAL_UNIT_CODED_SLICE_BLA_W_LP,
					h265parser.NAL_UNIT_CODED_SLICE_BLA_W_RADL,
					h265parser.NAL_UNIT_CODED_SLICE_BLA_N_LP,
					h265parser.NAL_UNIT_CODED_SLICE_IDR_W_RADL,
					h265parser.NAL_UNIT_CODED_SLICE_IDR_N_LP,
					h265parser.NAL_UNIT_CODED_SLICE_CRA:
					keyFrame = true
				}
				if !firstKeyFrameSent {
					if keyFrame {
						firstKeyFrameSent = true
					} else {
						continue
					}
				}
				var videoFrame rtmp.RTMPVideo
				videoFrame.SetAllocator(allocator)
				videoFrame.CTS = uint32(pkg.Pts - pkg.Dts)
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
				videoFrame.AddRecycleBytes(pkg.Data)
				err = publisher.WriteVideo(&videoFrame)
			case box.MP4_CODEC_AAC:
				var audioFrame rtmp.RTMPAudio
				audioFrame.SetAllocator(allocator)
				audioFrame.Timestamp = uint32(ts)
				audioFrame.AppendOne([]byte{0xaf, 0x01})
				audioFrame.AddRecycleBytes(pkg.Data)
				err = publisher.WriteAudio(&audioFrame)
			case box.MP4_CODEC_G711A:
				var audioFrame rtmp.RTMPAudio
				audioFrame.SetAllocator(allocator)
				audioFrame.Timestamp = uint32(ts)
				audioFrame.AppendOne([]byte{0x72})
				audioFrame.AddRecycleBytes(pkg.Data)
				err = publisher.WriteAudio(&audioFrame)
			case box.MP4_CODEC_G711U:
				var audioFrame rtmp.RTMPAudio
				audioFrame.SetAllocator(allocator)
				audioFrame.Timestamp = uint32(ts)
				audioFrame.AppendOne([]byte{0x82})
				audioFrame.AddRecycleBytes(pkg.Data)
				err = publisher.WriteAudio(&audioFrame)
			}
		}
	}
	return
}
