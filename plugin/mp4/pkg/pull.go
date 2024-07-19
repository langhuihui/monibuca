package mp4

import (
	"github.com/deepch/vdk/codec/h265parser"
	"io"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/util"
	"m7s.live/m7s/v5/plugin/mp4/pkg/box"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type MP4Puller struct {
	*util.ScalableMemoryAllocator
	*box.MovDemuxer
}

func NewMP4Puller() *MP4Puller {
	return &MP4Puller{
		ScalableMemoryAllocator: util.NewScalableMemoryAllocator(1 << 10),
	}
}

func NewPullHandler() m7s.PullHandler {
	return NewMP4Puller()
}

func (puller *MP4Puller) Connect(p *m7s.Client) (err error) {
	if strings.HasPrefix(p.RemoteURL, "http") {
		var res *http.Response
		client := http.DefaultClient
		if proxyConf := p.Proxy; proxyConf != "" {
			proxy, err := url.Parse(proxyConf)
			if err != nil {
				return err
			}
			transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
			client = &http.Client{Transport: transport}
		}
		if res, err = client.Get(p.RemoteURL); err == nil {
			if res.StatusCode != http.StatusOK {
				return io.EOF
			}
			p.Closer = res.Body

			content, err := io.ReadAll(res.Body)
			if err != nil {
				return err
			}
			puller.MovDemuxer = box.CreateMp4Demuxer(strings.NewReader(string(content)))
		}
	} else {
		var res *os.File
		if res, err = os.Open(p.RemoteURL); err == nil {
			p.Closer = res
		}
		puller.MovDemuxer = box.CreateMp4Demuxer(res)
	}
	return
}

func (puller *MP4Puller) Pull(p *m7s.Puller) (err error) {
	var tracks []box.TrackInfo
	if tracks, err = puller.ReadHead(); err != nil {
		return
	}
	for _, track := range tracks {
		switch track.Cid {
		case box.MP4_CODEC_H264:
			var sequece rtmp.RTMPVideo
			sequece.Append([]byte{0x17, 0x00, 0x00, 0x00, 0x00}, track.ExtraData)
			p.WriteVideo(&sequece)
		case box.MP4_CODEC_H265:
			var sequece rtmp.RTMPVideo
			sequece.Append([]byte{0b1001_0000 | rtmp.PacketTypeSequenceStart}, codec.FourCC_H265[:], track.ExtraData)
			p.WriteVideo(&sequece)
		case box.MP4_CODEC_AAC:
			var sequence rtmp.RTMPAudio
			sequence.Append([]byte{0xaf, 0x00}, track.ExtraData)
			p.WriteAudio(&sequence)
		}
	}
	for {
		pkg, err := puller.ReadPacket(puller.ScalableMemoryAllocator)
		if err != nil {
			p.Error("Error reading MP4 packet", "err", err)
			return err
		}
		switch track := tracks[pkg.TrackId-1]; track.Cid {
		case box.MP4_CODEC_H264:
			var videoFrame rtmp.RTMPVideo
			videoFrame.SetAllocator(puller.ScalableMemoryAllocator)
			videoFrame.CTS = uint32(pkg.Pts - pkg.Dts)
			videoFrame.Timestamp = uint32(pkg.Dts)
			keyFrame := codec.H264NALUType(pkg.Data[5]&0x1F) == codec.NALU_IDR_Picture
			videoFrame.AppendOne([]byte{util.Conditoinal[byte](keyFrame, 0x17, 0x27), 0x01, byte(videoFrame.CTS >> 24), byte(videoFrame.CTS >> 8), byte(videoFrame.CTS)})
			videoFrame.AddRecycleBytes(pkg.Data)
			p.WriteVideo(&videoFrame)
		case box.MP4_CODEC_H265:
			var videoFrame rtmp.RTMPVideo
			videoFrame.SetAllocator(puller.ScalableMemoryAllocator)
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
			p.WriteVideo(&videoFrame)
		case box.MP4_CODEC_AAC:
			var audioFrame rtmp.RTMPAudio
			audioFrame.SetAllocator(puller.ScalableMemoryAllocator)
			audioFrame.Timestamp = uint32(pkg.Dts)
			audioFrame.AppendOne([]byte{0xaf, 0x01})
			audioFrame.AddRecycleBytes(pkg.Data)
			p.WriteAudio(&audioFrame)
		}
	}
}
