package srt

import (
	"errors"
	"fmt"
	"io"
	"net"

	srt "github.com/datarhei/gosrt"
	"m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	mpegts "m7s.live/v5/plugin/hls/pkg/ts"
)

type Sender struct {
	task.Task
	Subscriber         *m7s.Subscriber
	pesAudio, pesVideo *mpegts.MpegtsPESFrame
	PMT                util.Buffer
	srt.Conn
	allocator *util.ScalableMemoryAllocator
}

func (s *Sender) Start() error {
	s.pesAudio = &mpegts.MpegtsPESFrame{
		Pid: mpegts.STREAM_ID_AUDIO,
	}
	s.pesVideo = &mpegts.MpegtsPESFrame{
		Pid: mpegts.STREAM_ID_VIDEO,
	}
	s.allocator = util.NewScalableMemoryAllocator(1 << util.MinPowerOf2)
	var audioCodec, videoCodec codec.FourCC
	if s.Subscriber.Publisher.HasAudioTrack() {
		audioCodec = s.Subscriber.Publisher.AudioTrack.FourCC()
	}
	if s.Subscriber.Publisher.HasVideoTrack() {
		videoCodec = s.Subscriber.Publisher.VideoTrack.FourCC()
	}
	mpegts.WritePMTPacket(&s.PMT, videoCodec, audioCodec)
	s.Write(mpegts.DefaultPATPacket)
	s.Write(s.PMT)
	return nil
}

func (s *Sender) sendAudio(packet mpegts.MpegTsPESPacket) (err error) {
	packet.Header.PacketStartCodePrefix = 0x000001
	packet.Header.ConstTen = 0x80
	packet.Header.StreamID = mpegts.STREAM_ID_AUDIO

	s.pesAudio.ProgramClockReferenceBase = packet.Header.Pts
	packet.Header.PtsDtsFlags = 0x80
	packet.Header.PesHeaderDataLength = 5
	return s.WritePESPacket(s.pesAudio, packet)
}

func (s *Sender) sendADTS(audio *pkg.ADTS) (err error) {
	var packet mpegts.MpegTsPESPacket
	packet.Header.PesPacketLength = uint16(audio.Size + 8)
	packet.Buffers = audio.Buffers
	packet.Header.Pts = uint64(audio.DTS)
	return s.sendAudio(packet)
}

func (s *Sender) sendRawAudio(audio *pkg.RawAudio) (err error) {
	var packet mpegts.MpegTsPESPacket
	packet.Header.PesPacketLength = uint16(audio.Size + 8)
	packet.Buffers = audio.Buffers
	packet.Header.Pts = uint64(audio.Timestamp)
	return s.sendAudio(packet)
}

func (s *Sender) sendVideo(video *pkg.AnnexB) (err error) {
	var buffer net.Buffers
	//需要对原始数据(ES),进行一些预处理,视频需要分割nalu(H264编码),并且打上sps,pps,nalu_aud信息.
	if video.Hevc {
		buffer = append(buffer, codec.AudNalu)
	} else {
		buffer = append(buffer, codec.NALU_AUD_BYTE)
	}
	buffer = append(buffer, video.Buffers...)
	pktLength := util.SizeOfBuffers(buffer) + 10 + 3
	if pktLength > 0xffff {
		pktLength = 0
	}

	var packet mpegts.MpegTsPESPacket
	packet.Header.PacketStartCodePrefix = 0x000001
	packet.Header.ConstTen = 0x80
	packet.Header.StreamID = mpegts.STREAM_ID_VIDEO
	packet.Header.PesPacketLength = uint16(pktLength)
	packet.Header.Pts = uint64(video.PTS)
	s.pesVideo.ProgramClockReferenceBase = packet.Header.Pts
	packet.Header.Dts = uint64(video.DTS)
	packet.Header.PtsDtsFlags = 0xC0
	packet.Header.PesHeaderDataLength = 10
	packet.Buffers = buffer
	return s.WritePESPacket(s.pesVideo, packet)
}

func (s *Sender) Go() error {
	if s.Subscriber.Publisher.HasAudioTrack() {
		if s.Subscriber.Publisher.AudioTrack.FourCC() == codec.FourCC_MP4A {
			return m7s.PlayBlock(s.Subscriber, s.sendADTS, s.sendVideo)
		}
	}
	return m7s.PlayBlock(s.Subscriber, s.sendRawAudio, s.sendVideo)
}

func (s *Sender) WritePESPacket(frame *mpegts.MpegtsPESFrame, pesPacket mpegts.MpegTsPESPacket) (err error) {
	if pesPacket.Header.PacketStartCodePrefix != 0x000001 {
		err = errors.New("packetStartCodePrefix != 0x000001")
		return
	}
	var pesHeadItem util.Buffer = s.allocator.Malloc(32)

	_, err = mpegts.WritePESHeader(&pesHeadItem, pesPacket.Header)
	if err != nil {
		return
	}
	pesBuffers := append(net.Buffers{pesHeadItem}, pesPacket.Buffers...)
	defer s.allocator.Free(pesHeadItem)
	pesPktLength := util.SizeOfBuffers(pesBuffers)
	var buffer util.Buffer = s.allocator.Malloc((pesPktLength/mpegts.TS_PACKET_SIZE+1)*6 + pesPktLength)
	bwTsHeader := &buffer
	bigLen := bwTsHeader.Len()
	bwTsHeader.Reset()
	defer s.allocator.Free(buffer)
	var tsHeaderLength int
	for i := 0; len(pesBuffers) > 0; i++ {
		if bigLen < mpegts.TS_PACKET_SIZE {
			// if i == 0 {
			// 	ts.Recycle()
			// }
			var headerItem util.Buffer = s.allocator.Malloc(mpegts.TS_PACKET_SIZE)
			defer s.allocator.Free(headerItem)
			bwTsHeader = &headerItem
			bwTsHeader.Reset()
		}
		bigLen -= mpegts.TS_PACKET_SIZE
		pesPktLength = util.SizeOfBuffers(pesBuffers)
		tsHeader := mpegts.MpegTsHeader{
			SyncByte:                   0x47,
			TransportErrorIndicator:    0,
			PayloadUnitStartIndicator:  0,
			TransportPriority:          0,
			Pid:                        frame.Pid,
			TransportScramblingControl: 0,
			AdaptionFieldControl:       1,
			ContinuityCounter:          frame.ContinuityCounter,
		}

		frame.ContinuityCounter++
		frame.ContinuityCounter = frame.ContinuityCounter % 16

		// 每一帧的开头,当含有pcr的时候,包含调整字段
		if i == 0 {
			tsHeader.PayloadUnitStartIndicator = 1

			// 当PCRFlag为1的时候,包含调整字段
			if frame.IsKeyFrame {
				tsHeader.AdaptionFieldControl = 0x03
				tsHeader.AdaptationFieldLength = 7
				tsHeader.PCRFlag = 1
				tsHeader.RandomAccessIndicator = 1
				tsHeader.ProgramClockReferenceBase = frame.ProgramClockReferenceBase
			}
		}

		// 每一帧的结尾,当不满足188个字节的时候,包含调整字段
		if pesPktLength < mpegts.TS_PACKET_SIZE-4 {
			var tsStuffingLength uint8

			tsHeader.AdaptionFieldControl = 0x03
			tsHeader.AdaptationFieldLength = uint8(mpegts.TS_PACKET_SIZE - 4 - 1 - pesPktLength)

			// TODO:如果第一个TS包也是最后一个TS包,是不是需要考虑这个情况?
			// MpegTsHeader最少占6个字节.(前4个走字节 + AdaptationFieldLength(1 byte) + 3个指示符5个标志位(1 byte))
			if tsHeader.AdaptationFieldLength >= 1 {
				tsStuffingLength = tsHeader.AdaptationFieldLength - 1
			} else {
				tsStuffingLength = 0
			}
			// error
			tsHeaderLength, err = mpegts.WriteTsHeader(bwTsHeader, tsHeader)
			if err != nil {
				return
			}
			if tsStuffingLength > 0 {
				if _, err = bwTsHeader.Write(mpegts.Stuffing[:tsStuffingLength]); err != nil {
					return
				}
			}
			tsHeaderLength += int(tsStuffingLength)
		} else {

			tsHeaderLength, err = mpegts.WriteTsHeader(bwTsHeader, tsHeader)
			if err != nil {
				return
			}
		}

		tsPayloadLength := mpegts.TS_PACKET_SIZE - tsHeaderLength

		//fmt.Println("tsPayloadLength :", tsPayloadLength)

		// 这里不断的减少PES包
		io.CopyN(bwTsHeader, &pesBuffers, int64(tsPayloadLength))
		// tmp := tsHeaderByte[3] << 2
		// tmp = tmp >> 6
		// if tmp == 2 {
		// 	fmt.Println("fuck you mother.")
		// }
		tsPktByteLen := bwTsHeader.Len()
		_, err = s.Write(*bwTsHeader)
		if err != nil {
			return
		}
		if tsPktByteLen != (i+1)*mpegts.TS_PACKET_SIZE && tsPktByteLen != mpegts.TS_PACKET_SIZE {
			err = errors.New(fmt.Sprintf("%s, packet size=%d", "TS_PACKET_SIZE != 188,", tsPktByteLen))
			return
		}
	}

	return nil
}
