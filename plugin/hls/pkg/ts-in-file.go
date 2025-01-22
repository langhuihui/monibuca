package hls

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"

	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
	mpegts "m7s.live/v5/plugin/hls/pkg/ts"
)

type TsInFile struct {
	PMT    util.Buffer
	file   *os.File
	path   string
	closed bool
}

func NewTsInFile(path string) (*TsInFile, error) {
	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	ts := &TsInFile{
		path: path,
		file: file,
	}
	return ts, nil
}

func (ts *TsInFile) Close() error {
	if ts.closed {
		return nil
	}
	ts.closed = true
	return ts.file.Close()
}

func (ts *TsInFile) WritePMTPacket(audio, video codec.FourCC) {
	ts.PMT.Reset()
	mpegts.WritePMTPacket(&ts.PMT, video, audio)
	// 写入PAT和PMT
	ts.file.Write(mpegts.DefaultPATPacket)
	ts.file.Write(ts.PMT)
}

func (ts *TsInFile) WritePESPacket(frame *mpegts.MpegtsPESFrame, packet mpegts.MpegTsPESPacket) (err error) {
	if packet.Header.PacketStartCodePrefix != 0x000001 {
		err = errors.New("packetStartCodePrefix != 0x000001")
		return
	}

	var pesHeadItem util.Buffer
	pesHeadItem.Reset()
	_, err = mpegts.WritePESHeader(&pesHeadItem, packet.Header)
	if err != nil {
		return
	}

	pesBuffers := append(net.Buffers{pesHeadItem}, packet.Buffers...)
	pesPktLength := int64(util.SizeOfBuffers(pesBuffers))

	for i := 0; pesPktLength > 0; i++ {
		var tsBuffer util.Buffer
		tsBuffer.Reset()

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

		if i == 0 {
			tsHeader.PayloadUnitStartIndicator = 1
			if frame.IsKeyFrame {
				tsHeader.AdaptionFieldControl = 0x03
				tsHeader.AdaptationFieldLength = 7
				tsHeader.PCRFlag = 1
				tsHeader.RandomAccessIndicator = 1
				tsHeader.ProgramClockReferenceBase = frame.ProgramClockReferenceBase
			}
		}

		if pesPktLength < mpegts.TS_PACKET_SIZE-4 {
			var tsStuffingLength uint8
			tsHeader.AdaptionFieldControl = 0x03
			tsHeader.AdaptationFieldLength = uint8(mpegts.TS_PACKET_SIZE - 4 - 1 - pesPktLength)

			if tsHeader.AdaptationFieldLength >= 1 {
				tsStuffingLength = tsHeader.AdaptationFieldLength - 1
			}

			tsHeaderLength, err := mpegts.WriteTsHeader(&tsBuffer, tsHeader)
			if err != nil {
				return err
			}

			if tsStuffingLength > 0 {
				if _, err = tsBuffer.Write(mpegts.Stuffing[:tsStuffingLength]); err != nil {
					return err
				}
			}

			tsPayloadLength := mpegts.TS_PACKET_SIZE - tsHeaderLength - int(tsStuffingLength)
			written, _ := io.CopyN(&tsBuffer, &pesBuffers, int64(tsPayloadLength))
			pesPktLength -= written

		} else {
			tsHeaderLength, err := mpegts.WriteTsHeader(&tsBuffer, tsHeader)
			if err != nil {
				return err
			}

			tsPayloadLength := mpegts.TS_PACKET_SIZE - tsHeaderLength
			written, _ := io.CopyN(&tsBuffer, &pesBuffers, int64(tsPayloadLength))
			pesPktLength -= written
		}

		// 直接写入文件
		if _, err = ts.file.Write(tsBuffer); err != nil {
			return err
		}
	}

	return nil
}

func (ts *TsInFile) WriteAudioFrame(frame *pkg.ADTS, pes *mpegts.MpegtsPESFrame) (err error) {
	var packet mpegts.MpegTsPESPacket
	packet.Header.PesPacketLength = uint16(frame.Size + 8)
	packet.Buffers = slices.Clone(frame.Buffers)
	packet.Header.Pts = uint64(frame.DTS)
	packet.Header.PacketStartCodePrefix = 0x000001
	packet.Header.ConstTen = 0x80
	packet.Header.StreamID = mpegts.STREAM_ID_AUDIO
	pes.ProgramClockReferenceBase = packet.Header.Pts
	packet.Header.PtsDtsFlags = 0x80
	packet.Header.PesHeaderDataLength = 5
	return ts.WritePESPacket(pes, packet)
}

func (ts *TsInFile) WriteVideoFrame(frame *pkg.AnnexB, pes *mpegts.MpegtsPESFrame) (err error) {
	var buffer net.Buffers
	if frame.Hevc {
		buffer = append(buffer, codec.AudNalu)
	} else {
		buffer = append(buffer, codec.NALU_AUD_BYTE)
	}
	buffer = append(buffer, frame.Buffers...)
	pktLength := util.SizeOfBuffers(buffer) + 10 + 3
	if pktLength > 0xffff {
		pktLength = 0
	}

	var packet mpegts.MpegTsPESPacket
	packet.Header.PacketStartCodePrefix = 0x000001
	packet.Header.ConstTen = 0x80
	packet.Header.StreamID = mpegts.STREAM_ID_VIDEO
	packet.Header.PesPacketLength = uint16(pktLength)
	packet.Header.Pts = uint64(frame.PTS)
	pes.ProgramClockReferenceBase = packet.Header.Pts
	packet.Header.Dts = uint64(frame.DTS)
	packet.Header.PtsDtsFlags = 0xC0
	packet.Header.PesHeaderDataLength = 10
	packet.Buffers = buffer
	return ts.WritePESPacket(pes, packet)
}
