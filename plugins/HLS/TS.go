package HLS

import (
	"bytes"
	. "github.com/langhuihui/monibuca/monica"
	"github.com/langhuihui/monibuca/monica/avformat"
	"github.com/langhuihui/monibuca/monica/avformat/mpegts"
	"github.com/langhuihui/monibuca/monica/pool"
	"github.com/langhuihui/monibuca/monica/util"
	"log"
	"time"
)

type TS struct {
	InputStream
	*mpegts.MpegTsStream
	TSInfo
	//TsChan     chan io.Reader
	lastDts uint64
}
type TSInfo struct {
	TotalPesCount int
	IsSplitFrame  bool
	PTS           uint64
	DTS           uint64
	PesCount      int
	BufferLength  int
	RoomInfo      *RoomInfo
}

func (ts *TS) run() {
	//defer close(ts.TsChan)
	totalBuffer := cap(ts.TsPesPktChan)
	iframeHead := []byte{0x17, 0x01, 0, 0, 0}
	pframeHead := []byte{0x27, 0x01, 0, 0, 0}
	spsHead := []byte{0xE1, 0, 0}
	ppsHead := []byte{0x01, 0, 0}
	nalLength := []byte{0, 0, 0, 0}
	for {
		select {
		case <-ts.Done():
			return
		case tsPesPkt, ok := <-ts.TsPesPktChan:
			ts.BufferLength = len(ts.TsPesPktChan)
			if ok {
				ts.TotalPesCount++
				switch tsPesPkt.PesPkt.Header.StreamID & 0xF0 {
				case mpegts.STREAM_ID_AUDIO:
					av := pool.NewAVPacket(avformat.FLV_TAG_TYPE_AUDIO)
					av.Payload = tsPesPkt.PesPkt.Payload
					ts.PushAudio(av)
				case mpegts.STREAM_ID_VIDEO:
					var err error
					av := pool.NewAVPacket(avformat.FLV_TAG_TYPE_VIDEO)
					ts.PTS = tsPesPkt.PesPkt.Header.Pts
					ts.DTS = tsPesPkt.PesPkt.Header.Dts
					lastDts := ts.lastDts
					dts := ts.DTS
					pts := ts.PTS
					if dts == 0 {
						dts = pts
					}
					av.Timestamp = uint32(dts / 90)
					if ts.lastDts == 0 {
						ts.lastDts = dts
					}
					compostionTime := uint32((pts - dts) / 90)
					t1 := time.Now()
					duration := time.Millisecond * time.Duration((dts-ts.lastDts)/90)
					ts.lastDts = dts
					nalus0 := bytes.SplitN(tsPesPkt.PesPkt.Payload, avformat.NALU_Delimiter2, -1)
					nalus := make([][]byte, 0)
					for _, v := range nalus0 {
						if len(v) == 0 {
							continue
						}
						nalus = append(nalus, bytes.SplitN(v, avformat.NALU_Delimiter1, -1)...)
					}
					r := bytes.NewBuffer([]byte{})
					for _, v := range nalus {
						vl := len(v)
						if vl == 0 {
							continue
						}
						isFirst := v[1]&0x80 == 0x80 //第一个分片
						switch v[0] & 0x1f {
						case avformat.NALU_SPS:
							r.Write(avformat.RTMP_AVC_HEAD)
							util.BigEndian.PutUint16(spsHead[1:], uint16(vl))
							_, err = r.Write(spsHead)
						case avformat.NALU_PPS:
							util.BigEndian.PutUint16(ppsHead[1:], uint16(vl))
							_, err = r.Write(ppsHead)
							_, err = r.Write(v)
							av.VideoFrameType = 1
							av.Payload = r.Bytes()
							ts.PushVideo(av)
							av = pool.NewAVPacket(avformat.FLV_TAG_TYPE_VIDEO)
							av.Timestamp = uint32(dts / 90)
							r = bytes.NewBuffer([]byte{})
							continue
						case avformat.NALU_IDR_Picture:
							if isFirst {
								av.VideoFrameType = 1
								util.BigEndian.PutUint24(iframeHead[2:], compostionTime)
								_, err = r.Write(iframeHead)
							}
							util.BigEndian.PutUint32(nalLength, uint32(vl))
							_, err = r.Write(nalLength)
						case avformat.NALU_Non_IDR_Picture:
							if isFirst {
								av.VideoFrameType = 2
								util.BigEndian.PutUint24(pframeHead[2:], compostionTime)
								_, err = r.Write(iframeHead)
							} else {
								ts.IsSplitFrame = true
							}
							util.BigEndian.PutUint32(nalLength, uint32(vl))
							_, err = r.Write(nalLength)
						default:
							continue
						}
						_, err = r.Write(v)
					}
					if MayBeError(err) {
						return
					}
					av.Payload = r.Bytes()
					ts.PushVideo(av)
					t2 := time.Since(t1)
					if duration != 0 && t2 < duration {
						if duration < time.Second {
							//if ts.BufferLength > 50 {
							duration = duration - t2
							//}
							if ts.BufferLength > 150 {
								duration = duration - duration*time.Duration(ts.BufferLength)/time.Duration(totalBuffer)
							}
							time.Sleep(duration)
						} else {
							time.Sleep(time.Millisecond * 20)
							log.Printf("stream:%s,duration:%d,dts:%d,lastDts:%d\n", ts.StreamPath, duration/time.Millisecond, tsPesPkt.PesPkt.Header.Dts, lastDts)
						}
					}
				}
			}
		}
	}
}

func (ts *TS) Publish(streamPath string, publisher Publisher) (result bool) {
	if result = ts.InputStream.Publish(streamPath, publisher); result {
		ts.TSInfo.RoomInfo = &ts.Room.RoomInfo
		ts.MpegTsStream = mpegts.NewMpegTsStream(2048)
		go ts.run()
	}
	return
}
