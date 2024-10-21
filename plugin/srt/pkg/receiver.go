package srt

import (
	"bytes"
	"time"

	srt "github.com/datarhei/gosrt"
	"m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/task"
	mpegts "m7s.live/v5/plugin/hls/pkg/ts"
)

type Receiver struct {
	task.Task
	Publisher *m7s.Publisher
	TSStream  mpegts.MpegTsStream
	srt.Conn
}

func (r *Receiver) Start() error {
	r.TSStream.PESChan = make(chan *mpegts.MpegTsPESPacket, 50)
	r.TSStream.PESBuffer = make(map[uint16]*mpegts.MpegTsPESPacket)
	go r.readPES()
	return nil
}

func (r *Receiver) readPES() {
	var videoFrame *pkg.AnnexB
	var err error
	defer func() {
		if err != nil {
			r.Stop(err)
		}
	}()
	for pes := range r.TSStream.PESChan {
		if r.Err() != nil {
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
				for _, s := range r.TSStream.PMT.Stream {
					switch s.StreamType {
					case mpegts.STREAM_TYPE_H265:
						videoFrame.Hevc = true
					}
				}
			} else {
				if videoFrame.PTS != time.Duration(pes.Header.Pts) {
					if r.Publisher.PubVideo {
						err = r.Publisher.WriteVideo(videoFrame)
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
			for _, s := range r.TSStream.PMT.Stream {
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
			if frame != nil && r.Publisher.PubAudio {
				if err = r.Publisher.WriteAudio(frame); err != nil {
					return
				}
			}
		}
	}
}

func (r *Receiver) Go() error {
	for !r.IsStopped() {
		packet, err := r.ReadPacket()
		if err != nil {
			return err
		}
		r.TSStream.Feed(bytes.NewReader(packet.Data()))
	}
	return r.StopReason()
}

func (r *Receiver) Dispose() {
	r.Close()
	close(r.TSStream.PESChan)
}
