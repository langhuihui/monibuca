package plugin_srt

import (
	"bytes"
	"strings"
	"time"

	srt "github.com/datarhei/gosrt"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/task"
	mpegts "m7s.live/m7s/v5/plugin/hls/pkg/ts"
)

type SRTServer struct {
	task.Task
	server srt.Server
	plugin *SRTPlugin
}

type SRTPlugin struct {
	m7s.Plugin
	ListenAddr string
	Passphrase string
}

var _ = m7s.InstallPlugin[SRTPlugin]()

func (p *SRTPlugin) OnInit() error {
	var t SRTServer
	t.server.Addr = p.ListenAddr
	t.plugin = p
	p.AddTask(&t)
	return nil
}

func (t *SRTServer) Start() error {
	t.server.HandleConnect = func(conn srt.ConnRequest) srt.ConnType {
		streamid := conn.StreamId()
		conn.SetPassphrase(t.plugin.Passphrase)
		if strings.HasPrefix(streamid, "publish:") {
			return srt.PUBLISH
		}
		return srt.SUBSCRIBE
	}
	t.server.HandlePublish = func(conn srt.Conn) {
		var stream mpegts.MpegTsStream
		stream.PESChan = make(chan *mpegts.MpegTsPESPacket, 50)
		stream.PESBuffer = make(map[uint16]*mpegts.MpegTsPESPacket)
		_, streamPath, _ := strings.Cut(conn.StreamId(), "/")
		publisher, err := t.plugin.Publish(t.plugin, streamPath)
		if err != nil {
			conn.Close()
			return
		}
		go func() {
			defer conn.Close()
			var videoFrame *pkg.AnnexB
			for pes := range stream.PESChan {
				if t.Err() != nil {
					continue
				}
				if pes.Header.Dts == 0 {
					pes.Header.Dts = pes.Header.Pts
				}
				switch pes.Header.StreamID & 0xF0 {
				case mpegts.STREAM_ID_VIDEO:
					if videoFrame == nil {
						videoFrame = &pkg.AnnexB{
							PTS: time.Duration(pes.Header.Pts),
							DTS: time.Duration(pes.Header.Dts),
						}
						for _, s := range stream.PMT.Stream {
							switch s.StreamType {
							case mpegts.STREAM_TYPE_H265:
								videoFrame.Hevc = true
							}
						}
					} else {
						if videoFrame.PTS != time.Duration(pes.Header.Pts) {
							if publisher.PubVideo {
								err = publisher.WriteVideo(videoFrame)
								if err != nil {
									return
								}
							}
							videoFrame = &pkg.AnnexB{
								PTS:  time.Duration(pes.Header.Pts),
								DTS:  time.Duration(pes.Header.Dts),
								Hevc: videoFrame.Hevc,
							}
						}
					}
					copy(videoFrame.NextN(len(pes.Payload)), pes.Payload)
				default:
					var frame pkg.IAVFrame
					for _, s := range stream.PMT.Stream {
						switch s.StreamType {
						case mpegts.STREAM_TYPE_AAC:
							var audioFrame pkg.ADTS
							audioFrame.DTS = time.Duration(pes.Header.Dts)
							copy(audioFrame.NextN(len(pes.Payload)), pes.Payload)
							frame = &audioFrame
						case mpegts.STREAM_TYPE_G711A:
							var audioFrame pkg.RawAudio
							audioFrame.FourCC = codec.FourCC_ALAW
							audioFrame.Timestamp = time.Duration(pes.Header.Dts)
							copy(audioFrame.NextN(len(pes.Payload)), pes.Payload)
							frame = &audioFrame
						case mpegts.STREAM_TYPE_G711U:
							var audioFrame pkg.RawAudio
							audioFrame.FourCC = codec.FourCC_ULAW
							audioFrame.Timestamp = time.Duration(pes.Header.Dts)
							copy(audioFrame.NextN(len(pes.Payload)), pes.Payload)
							frame = &audioFrame
						}
					}
					if frame != nil && publisher.PubAudio {
						if err = publisher.WriteAudio(frame); err != nil {
							return
						}
					}
				}
			}
		}()
		for {
			packet, err := conn.ReadPacket()
			if err != nil {
				break
			}
			stream.Feed(bytes.NewReader(packet.Data()))
		}
	}
	t.server.HandleSubscribe = func(conn srt.Conn) {
	}
	return nil
}

func (t *SRTServer) Run() error {
	return t.server.ListenAndServe()
}
