package mpegps

import (
	"errors"
	"fmt"
	"io"
	"time"

	"m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/format"
	"m7s.live/v5/pkg/util"

	mpegts "m7s.live/v5/pkg/format/ts"
)

const (
	StartCodePS        = 0x000001ba
	StartCodeSYS       = 0x000001bb
	StartCodeMAP       = 0x000001bc
	StartCodePadding   = 0x000001be
	StartCodeVideo     = 0x000001e0
	StartCodeVideo1    = 0x000001e1
	StartCodeVideo2    = 0x000001e2
	StartCodeAudio     = 0x000001c0
	PrivateStreamCode  = 0x000001bd
	MEPGProgramEndCode = 0x000001b9
)

// PS包头常量
const (
	PSPackHeaderSize   = 14     // PS pack header basic size
	PSSystemHeaderSize = 18     // PS system header basic size
	PSMHeaderSize      = 12     // PS map header basic size
	PESHeaderMinSize   = 9      // PES header minimum size
	MaxPESPayloadSize  = 0xFFEB // 0xFFFF - 14 (to leave room for headers)
)

type MpegPsDemuxer struct {
	stAudio, stVideo byte
	Publisher        *m7s.Publisher
	Allocator        *util.ScalableMemoryAllocator
	writer           m7s.PublishWriter[*format.Mpeg2Audio, *format.AnnexB]
}

func (s *MpegPsDemuxer) Feed(reader *util.BufReader) (err error) {
	writer := &s.writer
	var payload util.Memory
	var pesHeader mpegts.MpegPESHeader
	var lastVideoPts, lastAudioPts uint64
	var annexbReader pkg.AnnexBReader
	for {
		code, err := reader.ReadBE32(4)
		if err != nil {
			return err
		}
		switch code {
		case StartCodePS:
			var psl byte
			if err = reader.Skip(9); err != nil {
				return err
			}
			psl, err = reader.ReadByte()
			if err != nil {
				return err
			}
			psl &= 0x07
			if err = reader.Skip(int(psl)); err != nil {
				return err
			}
		case StartCodeVideo:
			payload, err = s.ReadPayload(reader)
			if err != nil {
				return err
			}
			if !s.Publisher.PubVideo {
				continue
			}
			if writer.PublishVideoWriter == nil {
				writer.PublishVideoWriter = m7s.NewPublishVideoWriter[*format.AnnexB](s.Publisher, s.Allocator)
				switch s.stVideo {
				case mpegts.STREAM_TYPE_H264:
					writer.VideoFrame.ICodecCtx = &codec.H264Ctx{}
				case mpegts.STREAM_TYPE_H265:
					writer.VideoFrame.ICodecCtx = &codec.H265Ctx{}
				}
			}
			pes := writer.VideoFrame
			reader := payload.NewReader()
			pesHeader, err = mpegts.ReadPESHeader(&io.LimitedReader{R: &reader, N: int64(payload.Size)})
			if err != nil {
				return errors.Join(err, fmt.Errorf("failed to read PES header"))
			}
			if pesHeader.Pts != 0 && pesHeader.Pts != lastVideoPts {
				if pes.Size > 0 {
					err = writer.NextVideo()
					if err != nil {
						return errors.Join(err, fmt.Errorf("failed to get next video frame"))
					}
					pes = writer.VideoFrame
				}
				pes.SetDTS(time.Duration(pesHeader.Dts))
				pes.SetPTS(time.Duration(pesHeader.Pts))
				lastVideoPts = pesHeader.Pts
			}
			annexb := s.Allocator.Malloc(reader.Length)
			reader.Read(annexb)
			annexbReader.AppendBuffer(annexb)
			_, err = pes.Parse(&annexbReader)
			if err != nil {
				return errors.Join(err, fmt.Errorf("failed to parse annexb"))
			}
		case StartCodeAudio:
			payload, err = s.ReadPayload(reader)
			if err != nil {
				return errors.Join(err, fmt.Errorf("failed to read audio payload"))
			}
			if s.stAudio == 0 || !s.Publisher.PubAudio {
				continue
			}
			if writer.PublishAudioWriter == nil {
				writer.PublishAudioWriter = m7s.NewPublishAudioWriter[*format.Mpeg2Audio](s.Publisher, s.Allocator)
				switch s.stAudio {
				case mpegts.STREAM_TYPE_AAC:
					writer.AudioFrame.ICodecCtx = &codec.AACCtx{}
				case mpegts.STREAM_TYPE_G711A:
					writer.AudioFrame.ICodecCtx = codec.NewPCMACtx()
				case mpegts.STREAM_TYPE_G711U:
					writer.AudioFrame.ICodecCtx = codec.NewPCMUCtx()
				}
			}
			pes := writer.AudioFrame
			reader := payload.NewReader()
			pesHeader, err = mpegts.ReadPESHeader(&io.LimitedReader{R: &reader, N: int64(payload.Size)})
			if err != nil {
				return errors.Join(err, fmt.Errorf("failed to read PES header"))
			}
			if pesHeader.Pts != 0 && pesHeader.Pts != lastAudioPts {
				if pes.Size > 0 {
					err = writer.NextAudio()
					if err != nil {
						return errors.Join(err, fmt.Errorf("failed to get next audio frame"))
					}
					pes = writer.AudioFrame
				}
				pes.SetDTS(time.Duration(pesHeader.Pts))
				pes.SetPTS(time.Duration(pesHeader.Pts))
				lastAudioPts = pesHeader.Pts
			}
			reader.Range(func(buf []byte) {
				copy(pes.NextN(len(buf)), buf)
			})
			// reader.Range(pes.PushOne)
		case StartCodeMAP:
			var psm util.Memory
			psm, err = s.ReadPayload(reader)
			if err != nil {
				return errors.Join(err, fmt.Errorf("failed to read program stream map"))
			}
			err = s.decProgramStreamMap(psm)
			if err != nil {
				return errors.Join(err, fmt.Errorf("failed to decode program stream map"))
			}
		default:
			payloadlen, err := reader.ReadBE(2)
			if err != nil {
				return errors.Join(err, fmt.Errorf("failed to read payload length"))
			}
			reader.Skip(payloadlen)
		}
	}
}

func (s *MpegPsDemuxer) ReadPayload(reader *util.BufReader) (payload util.Memory, err error) {
	payloadlen, err := reader.ReadBE(2)
	if err != nil {
		return
	}
	return reader.ReadBytes(payloadlen)
}

func (s *MpegPsDemuxer) decProgramStreamMap(psm util.Memory) (err error) {
	var programStreamInfoLen, programStreamMapLen, elementaryStreamInfoLength uint32
	var streamType, elementaryStreamID byte
	reader := psm.NewReader()
	reader.Skip(2)
	programStreamInfoLen, err = reader.ReadBE(2)
	reader.Skip(int(programStreamInfoLen))
	programStreamMapLen, err = reader.ReadBE(2)
	for programStreamMapLen > 0 {
		streamType, err = reader.ReadByte()
		elementaryStreamID, err = reader.ReadByte()
		if elementaryStreamID >= 0xe0 && elementaryStreamID <= 0xef {
			s.stVideo = streamType

		} else if elementaryStreamID >= 0xc0 && elementaryStreamID <= 0xdf {
			s.stAudio = streamType
		}
		elementaryStreamInfoLength, err = reader.ReadBE(2)
		reader.Skip(int(elementaryStreamInfoLength))
		programStreamMapLen -= 4 + elementaryStreamInfoLength
	}
	return nil
}

type MpegPSMuxer struct {
	*m7s.Subscriber
	Packet *util.RecyclableMemory
}

func (muxer *MpegPSMuxer) Mux(onPacket func() error) {
	var pesAudio, pesVideo *MpegpsPESFrame
	puber := muxer.Publisher
	var elementary_stream_map_length uint16
	if puber.HasAudioTrack() {
		elementary_stream_map_length += 4
		pesAudio = &MpegpsPESFrame{}
		pesAudio.StreamID = mpegts.STREAM_ID_AUDIO
		switch puber.AudioTrack.ICodecCtx.FourCC() {
		case codec.FourCC_ALAW:
			pesAudio.StreamType = mpegts.STREAM_TYPE_G711A
		case codec.FourCC_ULAW:
			pesAudio.StreamType = mpegts.STREAM_TYPE_G711U
		case codec.FourCC_MP4A:
			pesAudio.StreamType = mpegts.STREAM_TYPE_AAC
		}
	}
	if puber.HasVideoTrack() {
		elementary_stream_map_length += 4
		pesVideo = &MpegpsPESFrame{}
		pesVideo.StreamID = mpegts.STREAM_ID_VIDEO
		switch puber.VideoTrack.ICodecCtx.FourCC() {
		case codec.FourCC_H264:
			pesVideo.StreamType = mpegts.STREAM_TYPE_H264
		case codec.FourCC_H265:
			pesVideo.StreamType = mpegts.STREAM_TYPE_H265
		}
	}
	var outputBuffer util.Buffer = muxer.Packet.NextN(PSPackHeaderSize + PSMHeaderSize + int(elementary_stream_map_length))
	outputBuffer.Reset()
	MuxPSHeader(&outputBuffer)
	// System Header - 定义流的缓冲区信息
	// outputBuffer.WriteUint32(StartCodeSYS)
	// outputBuffer.WriteByte(0x00)           // header_length high
	// outputBuffer.WriteByte(0x0C)           // header_length low (12 bytes)
	// outputBuffer.WriteByte(0x80)           // marker + rate_bound[21..15]
	// outputBuffer.WriteByte(0x62)           // rate_bound[14..8]
	// outputBuffer.WriteByte(0x4E)           // rate_bound[7..1] + marker
	// outputBuffer.WriteByte(0x01)           // audio_bound + fixed_flag + CSPS_flag + system_audio_lock_flag + system_video_lock_flag + marker
	// outputBuffer.WriteByte(0x01)           // video_bound + packet_rate_restriction_flag + reserved
	// outputBuffer.WriteByte(frame.StreamId) // stream_id
	// outputBuffer.WriteByte(0xC0)           // '11' + P-STD_buffer_bound_scale
	// outputBuffer.WriteByte(0x20)           // P-STD_buffer_size_bound low
	// outputBuffer.WriteByte(0x00)           // P-STD_buffer_size_bound high
	// outputBuffer.WriteByte(0x00)
	// outputBuffer.WriteByte(0x00)
	// outputBuffer.WriteByte(0x00)

	// PSM Header - 程序流映射，定义流类型
	outputBuffer.WriteUint32(StartCodeMAP)
	outputBuffer.WriteUint16(uint16(PSMHeaderSize) + elementary_stream_map_length - 6) // psm_length
	outputBuffer.WriteByte(0xE0)                                                       // current_next_indicator + reserved + psm_version
	outputBuffer.WriteByte(0xFF)                                                       // reserved + marker
	outputBuffer.WriteUint16(0)                                                        // program_stream_info_length

	outputBuffer.WriteUint16(elementary_stream_map_length)
	if pesAudio != nil {
		outputBuffer.WriteByte(pesAudio.StreamType) // stream_type
		outputBuffer.WriteByte(pesAudio.StreamID)   // elementary_stream_id
		outputBuffer.WriteUint16(0)                 // elementary_stream_info_length
	}
	if pesVideo != nil {
		outputBuffer.WriteByte(pesVideo.StreamType) // stream_type
		outputBuffer.WriteByte(pesVideo.StreamID)   // elementary_stream_id
		outputBuffer.WriteUint16(0)                 // elementary_stream_info_length
	}
	onPacket()
	m7s.PlayBlock(muxer.Subscriber, func(audio *format.Mpeg2Audio) error {
		pesAudio.Pts = uint64(audio.GetPTS())
		pesAudio.WritePESPacket(audio.Memory, muxer.Packet)
		return onPacket()
	}, func(video *format.AnnexB) error {
		pesVideo.Pts = uint64(video.GetPTS())
		pesVideo.Dts = uint64(video.GetDTS())
		pesVideo.WritePESPacket(video.Memory, muxer.Packet)

		return onPacket()
	})
}

func MuxPSHeader(outputBuffer *util.Buffer) {
	// 写入PS Pack Header - 参考MPEG-2程序流标准
	// Pack start code: 0x000001BA
	outputBuffer.WriteUint32(StartCodePS)
	// SCR字段 (System Clock Reference) - 参考ps-muxer.go的实现
	// 系统时钟参考
	scr := uint64(time.Now().UnixMilli()) * 90
	outputBuffer.WriteByte(0x44 | byte((scr>>30)&0x07)) // '01' + SCR[32..30]
	outputBuffer.WriteByte(byte((scr >> 22) & 0xFF))    // SCR[29..22]
	outputBuffer.WriteByte(0x04 | byte((scr>>20)&0x03)) // marker + SCR[21..20]
	outputBuffer.WriteByte(byte((scr >> 12) & 0xFF))    // SCR[19..12]
	outputBuffer.WriteByte(0x04 | byte((scr>>10)&0x03)) // marker + SCR[11..10]
	outputBuffer.WriteByte(byte((scr >> 2) & 0xFF))     // SCR[9..2]
	outputBuffer.WriteByte(0x04 | byte(scr&0x03))       // marker + SCR[1..0]
	outputBuffer.WriteByte(0x01)                        // SCR_ext + marker
	outputBuffer.WriteByte(0x89)                        // program_mux_rate high
	outputBuffer.WriteByte(0xC8)                        // program_mux_rate low + markers + reserved + stuffing_length(0)
}
