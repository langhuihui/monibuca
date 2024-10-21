package mpegts

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"m7s.live/v5/pkg/util"
	//"sync"
)

// NALU AUD 00 00 00 01 09 F0

const (
	TS_PACKET_SIZE      = 188
	TS_DVHS_PACKET_SIZE = 192
	TS_FEC_PACKET_SIZE  = 204

	TS_MAX_PACKET_SIZE = 204

	PID_PAT        = 0x0000
	PID_CAT        = 0x0001
	PID_TSDT       = 0x0002
	PID_RESERVED1  = 0x0003
	PID_RESERVED2  = 0x000F
	PID_NIT_ST     = 0x0010
	PID_SDT_BAT_ST = 0x0011
	PID_EIT_ST     = 0x0012
	PID_RST_ST     = 0x0013
	PID_TDT_TOT_ST = 0x0014
	PID_NET_SYNC   = 0x0015
	PID_RESERVED3  = 0x0016
	PID_RESERVED4  = 0x001B
	PID_SIGNALLING = 0x001C
	PID_MEASURE    = 0x001D
	PID_DIT        = 0x001E
	PID_SIT        = 0x001F
	PID_PMT        = 0x0100
	PID_VIDEO      = 0x0101
	PID_AUDIO      = 0x0102
	// 0x0003 - 0x000F Reserved
	// 0x0010 - 0x1FFE May be assigned as network_PID, Program_map_PID, elementary_PID, or for other purposes
	// 0x1FFF Null Packet

	// program_association_section
	// conditional_access_section
	// TS_program_map_section
	// TS_description_section
	// ISO_IEC_14496_scene_description_section
	// ISO_IEC_14496_object_descriptor_section
	// Metadata_section
	// IPMP_Control_Information_section (defined in ISO/IEC 13818-11)
	TABLE_PAS               = 0x00
	TABLE_CAS               = 0x01
	TABLE_TSPMS             = 0x02
	TABLE_TSDS              = 0x03
	TABLE_ISO_IEC_14496_SDC = 0x04
	TABLE_ISO_IEC_14496_ODC = 0x05
	TABLE_MS                = 0x06
	TABLE_IPMP_CIS          = 0x07
	// 0x06 - 0x37 ITU-T Rec. H.222.0 | ISO/IEC 13818-1 reserved
	// 0x38 - 0x3F Defined in ISO/IEC 13818-6
	// 0x40 - 0xFE User private
	// 0xFF Forbidden
	STREAM_TYPE_VIDEO_MPEG1      = 0x01
	STREAM_TYPE_VIDEO_MPEG2      = 0x02
	STREAM_TYPE_AUDIO_MPEG1      = 0x03
	STREAM_TYPE_AUDIO_MPEG2      = 0x04
	STREAM_TYPE_PRIVATE_SECTIONS = 0x05
	STREAM_TYPE_PRIVATE_DATA     = 0x06
	STREAM_TYPE_MHEG             = 0x07

	STREAM_TYPE_H264   = 0x1B
	STREAM_TYPE_H265   = 0x24
	STREAM_TYPE_AAC    = 0x0F
	STREAM_TYPE_G711A  = 0x90
	STREAM_TYPE_G711U  = 0x91
	STREAM_TYPE_G722_1 = 0x92
	STREAM_TYPE_G723_1 = 0x93
	STREAM_TYPE_G726   = 0x94
	STREAM_TYPE_G729   = 0x99

	STREAM_TYPE_ADPCM = 0x11
	STREAM_TYPE_PCM   = 0x0A
	STREAM_TYPE_AC3   = 0x81
	STREAM_TYPE_DTS   = 0x8A
	STREAM_TYPE_LPCM  = 0x8B
	// 1110 xxxx
	// 110x xxxx
	STREAM_ID_VIDEO = 0xE0 // ITU-T Rec. H.262 | ISO/IEC 13818-2 or ISO/IEC 11172-2 or ISO/IEC14496-2 video stream number xxxx
	STREAM_ID_AUDIO = 0xC0 // ISO/IEC 13818-3 or ISO/IEC 11172-3 or ISO/IEC 13818-7 or ISO/IEC14496-3 audio stream number x xxxx

	PAT_PKT_TYPE = 0
	PMT_PKT_TYPE = 1
	PES_PKT_TYPE = 2
)

//
// MPEGTS -> PAT + PMT + PES
// ES -> PES -> TS
//

type MpegTsStream struct {
	PAT       MpegTsPAT // PAT表信息
	PMT       MpegTsPMT // PMT表信息
	PESBuffer map[uint16]*MpegTsPESPacket
	PESChan   chan *MpegTsPESPacket
}

// ios13818-1-CN.pdf 33/165
//
// TS
//

// Packet == Header + Payload == 188 bytes
type MpegTsPacket struct {
	Header  MpegTsHeader
	Payload []byte
}

// 前面32bit的数据即TS分组首部,它指出了这个分组的属性
type MpegTsHeader struct {
	SyncByte                   byte   // 8 bits  同步字节,固定为0x47,表示后面是一个TS分组
	TransportErrorIndicator    byte   // 1 bit  传输错误标志位
	PayloadUnitStartIndicator  byte   // 1 bit  负载单元开始标志(packet不满188字节时需填充).为1时,表示在4个字节后,有一个调整字节
	TransportPriority          byte   // 1 bit  传输优先级
	Pid                        uint16 // 13 bits Packet ID号码,唯一的号码对应不同的包.为0表示携带的是PAT表
	TransportScramblingControl byte   // 2 bits  加密标志位(00:未加密;其他表示已加密)
	AdaptionFieldControl       byte   // 2 bits  附加区域控制.表示TS分组首部后面是否跟随有调整字段和有效负载.01仅含有效负载(没有adaptation_field),10仅含调整字段(没有Payload),11含有调整字段和有效负载(有adaptation_field,adaptation_field之后是Payload).为00的话解码器不进行处理.空分组没有调整字段
	ContinuityCounter          byte   // 4 bits  包递增计数器.范围0-15,具有相同的PID的TS分组传输时每次加1,到15后清0.不过,有些情况下是不计数的.

	MpegTsHeaderAdaptationField
}

// 调整字段,只可能出现在每一帧的开头(当含有pcr的时候),或者结尾(当不满足188个字节的时候)
// adaptionFieldControl 00 -> 高字节代表调整字段, 低字节代表负载字段 0x20 0x10
// PCR字段编码在MPEG-2 TS包的自适应字段(Adaptation field)的6个Byte中,其中6 bits为预留位,42 bits为有效位()
// MpegTsHeaderAdaptationField + stuffing bytes
type MpegTsHeaderAdaptationField struct {
	AdaptationFieldLength             byte // 8bits 本区域除了本字节剩下的长度(不包含本字节!!!切记), if adaptationFieldLength > 0, 那么就有下面8个字段. adaptation_field_length 值必须在 0 到 182 的区间内.当 adaptation_field_control 值为'10'时,adaptation_field_length 值必须为 183
	DiscontinuityIndicator            byte // 1bit 置于"1"时,指示当前传输流包的不连续性状态为真.当 discontinuity_indicator 设置为"0"或不存在时,不连续性状态为假.不连续性指示符用于指示两种类型的不连续性,系统时间基不连续性和 continuity_counter 不连续性.
	RandomAccessIndicator             byte // 1bit 指示当前的传输流包以及可能的具有相同 PID 的后续传输流包,在此点包含有助于随机接入的某些信息.特别的,该比特置于"1"时,在具有当前 PID 的传输流包的有效载荷中起始的下一个 PES 包必须包含一个 discontinuity_indicator 字段中规定的基本流接入点.此外,在视频情况中,显示时间标记必须在跟随基本流接入点的第一图像中存在
	ElementaryStreamPriorityIndicator byte // 1bit 在具有相同 PID 的包之间,它指示此传输流包有效载荷内承载的基本流数据的优先级.1->指示该有效载荷具有比其他传输流包有效载荷更高的优先级
	PCRFlag                           byte // 1bit 1->指示 adaptation_field 包含以两部分编码的 PCR 字段.0->指示自适应字段不包含任何 PCR 字段
	OPCRFlag                          byte // 1bit 1->指示 adaptation_field 包含以两部分编码的 OPCR字段.0->指示自适应字段不包含任何 OPCR 字段
	SplicingPointFlag                 byte // 1bit 1->指示 splice_countdown 字段必须在相关自适应字段中存在,指定拼接点的出现.0->指示自适应字段中 splice_countdown 字段不存在
	TrasportPrivateDataFlag           byte // 1bit 1->指示自适应字段包含一个或多个 private_data 字节.0->指示自适应字段不包含任何 private_data 字节
	AdaptationFieldExtensionFlag      byte // 1bit 1->指示自适应字段扩展的存在.0->指示自适应字段中自适应字段扩展不存在

	// Optional Fields
	ProgramClockReferenceBase              uint64 // 33 bits pcr
	Reserved1                              byte   // 6 bits
	ProgramClockReferenceExtension         uint16 // 9 bits
	OriginalProgramClockReferenceBase      uint64 // 33 bits opcr
	Reserved2                              byte   // 6 bits
	OriginalProgramClockReferenceExtension uint16 // 9 bits
	SpliceCountdown                        byte   // 8 bits
	TransportPrivateDataLength             byte   // 8 bits 指定紧随传输private_data_length 字段的 private_data 字节数. private_data 字节数不能使专用数据扩展超出自适应字段的范围
	PrivateDataByte                        byte   // 8 bits 不通过 ITU-T|ISO/IEC 指定
	AdaptationFieldExtensionLength         byte   // 8 bits 指定紧随此字段的扩展的自适应字段数据的字节数,包括要保留的字节(如果存在)
	LtwFlag                                byte   // 1 bit 1->指示 ltw_offset 字段存在
	PiecewiseRateFlag                      byte   // 1 bit 1->指示 piecewise_rate 字段存在
	SeamlessSpliceFlag                     byte   // 1 bit 1->指示 splice_type 以及 DTS_next_AU 字段存在. 0->指示无论是 splice_type 字段还是 DTS_next_AU 字段均不存在

	// Optional Fields
	LtwValidFlag  byte   // 1 bit 1->指示 ltw_offset 的值必将生效.0->指示 ltw_offset 字段中该值未定义
	LtwOffset     uint16 // 15 bits 其值仅当 ltw_valid 标志字段具有'1'值时才定义.定义时,法定时间窗补偿以(300/fs)秒为度量单位,其中 fs 为此 PID 所归属的节目的系统时钟频率
	Reserved3     byte   // 2 bits 保留
	PiecewiseRate uint32 // 22 bits 只要当 ltw_flag 和 ltw_valid_flag 均置于‘1’时,此 22 比特字段的含义才确定
	SpliceType    byte   // 4 bits
	DtsNextAU     uint64 // 33 bits (解码时间标记下一个存取单元)

	// stuffing bytes
	// 此为固定的 8 比特值等于'1111 1111',能够通过编码器插入.它亦能被解码器丢弃
}

// ios13818-1-CN.pdf 77
//
// Descriptor
//

type MpegTsDescriptor struct {
	Tag    byte // 8 bits 标识每一个描述符
	Length byte // 8 bits 指定紧随 descriptor_length 字段的描述符的字节数
	Data   []byte
}

func ReadTsPacket(r io.Reader) (packet MpegTsPacket, err error) {
	lr := &io.LimitedReader{R: r, N: TS_PACKET_SIZE}

	// header
	packet.Header, err = ReadTsHeader(lr)
	if err != nil {
		return
	}

	// payload
	packet.Payload = make([]byte, lr.N)
	_, err = lr.Read(packet.Payload)
	if err != nil {
		return
	}

	return
}

func ReadTsHeader(r io.Reader) (header MpegTsHeader, err error) {
	var h uint32

	// MPEGTS Header 4个字节
	h, err = util.ReadByteToUint32(r, true)
	if err != nil {
		return
	}

	// payloadUnitStartIndicator
	// 为1时,表示在4个字节后,有一个调整字节.包头后需要除去一个字节才是有效数据(payload_unit_start_indicator="1")
	// header.payloadUnitStartIndicator = uint8(h & 0x400000)

	// | 1111 1111 | 0000 0000 | 0000 0000 | 0000 0000 |

	// | 1111 1111 | 0000 0000 | 0000 0000 | 0000 0000 |
	header.SyncByte = byte((h & 0xff000000) >> 24)

	if header.SyncByte != 0x47 {
		err = errors.New("mpegts header sync error!")
		return
	}

	// | 0000 0000 | 1000 0000 | 0000 0000 | 0000 0000 |
	header.TransportErrorIndicator = byte((h & 0x800000) >> 23)

	// | 0000 0000 | 0100 0000 | 0000 0000 | 0000 0000 |
	header.PayloadUnitStartIndicator = byte((h & 0x400000) >> 22)

	// | 0000 0000 | 0010 0000 | 0000 0000 | 0000 0000 |
	header.TransportPriority = byte((h & 0x200000) >> 21)

	// | 0000 0000 | 0001 1111 | 1111 1111 | 0000 0000 |
	header.Pid = uint16((h & 0x1fff00) >> 8)

	// | 0000 0000 | 0000 0000 | 0000 0000 | 1100 0000 |
	header.TransportScramblingControl = byte((h & 0xc0) >> 6)

	// | 0000 0000 | 0000 0000 | 0000 0000 | 0011 0000 |
	// 0x30 , 0x20 -> adaptation_field, 0x10 -> Payload
	header.AdaptionFieldControl = byte((h & 0x30) >> 4)

	// | 0000 0000 | 0000 0000 | 0000 0000 | 0000 1111 |
	header.ContinuityCounter = byte(h & 0xf)

	// | 0010 0000 |
	// adaptionFieldControl
	// 表示TS分组首部后面是否跟随有调整字段和有效负载.
	// 01仅含有效负载(没有adaptation_field)
	// 10仅含调整字段(没有Payload)
	// 11含有调整字段和有效负载(有adaptation_field,adaptation_field之后是Payload).
	// 为00的话解码器不进行处理.空分组没有调整字段
	// 当值为'11时,adaptation_field_length 值必须在0 到182 的区间内.
	// 当值为'10'时,adaptation_field_length 值必须为183.
	// 对于承载PES 包的传输流包,只要存在欠充足的PES 包数据就需要通过填充来完全填满传输流包的有效载荷字节.
	// 填充通过规定自适应字段长度比自适应字段中数据元的长度总和还要长来实现,以致于自适应字段在完全容纳有效的PES 包数据后,有效载荷字节仍有剩余.自适应字段中额外空间采用填充字节填满.
	if header.AdaptionFieldControl >= 2 {
		// adaptationFieldLength
		header.AdaptationFieldLength, err = util.ReadByteToUint8(r)
		if err != nil {
			return
		}

		if header.AdaptationFieldLength > 0 {
			lr := &io.LimitedReader{R: r, N: int64(header.AdaptationFieldLength)}

			// discontinuityIndicator(1)
			// randomAccessIndicator(1)
			// elementaryStreamPriorityIndicator
			// PCRFlag
			// OPCRFlag
			// splicingPointFlag
			// trasportPrivateDataFlag
			// adaptationFieldExtensionFlag
			var flags uint8
			flags, err = util.ReadByteToUint8(lr)
			if err != nil {
				return
			}

			header.DiscontinuityIndicator = flags & 0x80
			header.RandomAccessIndicator = flags & 0x40
			header.ElementaryStreamPriorityIndicator = flags & 0x20
			header.PCRFlag = flags & 0x10
			header.OPCRFlag = flags & 0x08
			header.SplicingPointFlag = flags & 0x04
			header.TrasportPrivateDataFlag = flags & 0x02
			header.AdaptationFieldExtensionFlag = flags & 0x01

			// randomAccessIndicator
			// 在此点包含有助于随机接入的某些信息.
			// 特别的,该比特置于"1"时,在具有当前 PID 的传输流包的有效载荷中起始的下一个 PES 包必须包含一个 discontinuity_indicator 字段中规定的基本流接入点.
			// 此外,在视频情况中,显示时间标记必须在跟随基本流接入点的第一图像中存在
			if header.RandomAccessIndicator != 0 {
			}

			// PCRFlag
			// 1->指示 adaptation_field 包含以两部分编码的 PCR 字段.
			// 0->指示自适应字段不包含任何 PCR 字段
			if header.PCRFlag != 0 {
				var pcr uint64
				pcr, err = util.ReadByteToUint48(lr, true)
				if err != nil {
					return
				}

				// PCR(i) = PCR_base(i)*300 + PCR_ext(i)
				// afd.programClockReferenceBase * 300 + afd.programClockReferenceExtension
				header.ProgramClockReferenceBase = pcr >> 15                // 9 bits  + 6 bits
				header.ProgramClockReferenceExtension = uint16(pcr & 0x1ff) // 9 bits -> | 0000 0001 | 1111 1111 |
			}

			// OPCRFlag
			if header.OPCRFlag != 0 {
				var opcr uint64
				opcr, err = util.ReadByteToUint48(lr, true)
				if err != nil {
					return
				}

				// OPCR(i) = OPCR_base(i)*300 + OPCR_ext(i)
				// afd.originalProgramClockReferenceBase * 300 + afd.originalProgramClockReferenceExtension
				header.OriginalProgramClockReferenceBase = opcr >> 15                // 9 bits  + 6 bits
				header.OriginalProgramClockReferenceExtension = uint16(opcr & 0x1ff) // 9 bits -> | 0000 0001 | 1111 1111 |
			}

			// splicingPointFlag
			// 1->指示 splice_countdown 字段必须在相关自适应字段中存在,指定拼接点的出现.
			// 0->指示自适应字段中 splice_countdown 字段不存在
			if header.SplicingPointFlag != 0 {
				header.SpliceCountdown, err = util.ReadByteToUint8(lr)
				if err != nil {
					return
				}
			}

			// trasportPrivateDataFlag
			// 1->指示自适应字段包含一个或多个 private_data 字节.
			// 0->指示自适应字段不包含任何 private_data 字节
			if header.TrasportPrivateDataFlag != 0 {
				header.TransportPrivateDataLength, err = util.ReadByteToUint8(lr)
				if err != nil {
					return
				}

				// privateDataByte
				b := make([]byte, header.TransportPrivateDataLength)
				if _, err = lr.Read(b); err != nil {
					return
				}
			}

			// adaptationFieldExtensionFlag
			if header.AdaptationFieldExtensionFlag != 0 {
			}

			// 消耗掉剩下的数据,我们不关心
			if lr.N > 0 {
				// Discard 是一个 io.Writer,对它进行的任何 Write 调用都将无条件成功
				// 但是ioutil.Discard不记录copy得到的数值
				// 用于发送需要读取但不想存储的数据,目的是耗尽读取端的数据
				if _, err = io.CopyN(ioutil.Discard, lr, int64(lr.N)); err != nil {
					return
				}
			}

		}
	}

	return
}

func WriteTsHeader(w io.Writer, header MpegTsHeader) (written int, err error) {
	if header.SyncByte != 0x47 {
		err = errors.New("mpegts header sync error!")
		return
	}

	h := uint32(header.SyncByte)<<24 + uint32(header.TransportErrorIndicator)<<23 + uint32(header.PayloadUnitStartIndicator)<<22 + uint32(header.TransportPriority)<<21 + uint32(header.Pid)<<8 + uint32(header.TransportScramblingControl)<<6 + uint32(header.AdaptionFieldControl)<<4 + uint32(header.ContinuityCounter)
	if err = util.WriteUint32ToByte(w, h, true); err != nil {
		return
	}

	written += 4

	if header.AdaptionFieldControl >= 2 {
		// adaptationFieldLength(8)
		if err = util.WriteUint8ToByte(w, header.AdaptationFieldLength); err != nil {
			return
		}

		written += 1

		if header.AdaptationFieldLength > 0 {

			// discontinuityIndicator(1)
			// randomAccessIndicator(1)
			// elementaryStreamPriorityIndicator(1)
			// PCRFlag(1)
			// OPCRFlag(1)
			// splicingPointFlag(1)
			// trasportPrivateDataFlag(1)
			// adaptationFieldExtensionFlag(1)
			threeIndicatorFiveFlags := uint8(header.DiscontinuityIndicator<<7) + uint8(header.RandomAccessIndicator<<6) + uint8(header.ElementaryStreamPriorityIndicator<<5) + uint8(header.PCRFlag<<4) + uint8(header.OPCRFlag<<3) + uint8(header.SplicingPointFlag<<2) + uint8(header.TrasportPrivateDataFlag<<1) + uint8(header.AdaptationFieldExtensionFlag)
			if err = util.WriteUint8ToByte(w, threeIndicatorFiveFlags); err != nil {
				return
			}

			written += 1

			// PCR(i) = PCR_base(i)*300 + PCR_ext(i)
			if header.PCRFlag != 0 {
				pcr := header.ProgramClockReferenceBase<<15 | 0x3f<<9 | uint64(header.ProgramClockReferenceExtension)
				if err = util.WriteUint48ToByte(w, pcr, true); err != nil {
					return
				}

				written += 6
			}

			// OPCRFlag
			if header.OPCRFlag != 0 {
				opcr := header.OriginalProgramClockReferenceBase<<15 | 0x3f<<9 | uint64(header.OriginalProgramClockReferenceExtension)
				if err = util.WriteUint48ToByte(w, opcr, true); err != nil {
					return
				}

				written += 6
			}
		}

	}

	return
}

//
//func (s *MpegTsStream) TestWrite(fileName string) error {
//
//	if fileName != "" {
//		file, err := os.Create(fileName)
//		if err != nil {
//			panic(err)
//		}
//		defer file.Close()
//
//		patTsHeader := []byte{0x47, 0x40, 0x00, 0x10}
//
//		if err := WritePATPacket(file, patTsHeader, *s.pat); err != nil {
//			panic(err)
//		}
//
//		// TODO:这里的pid应该是由PAT给的
//		pmtTsHeader := []byte{0x47, 0x41, 0x00, 0x10}
//
//		if err := WritePMTPacket(file, pmtTsHeader, *s.pmt); err != nil {
//			panic(err)
//		}
//	}
//
//	var videoFrame int
//	var audioFrame int
//	for {
//		tsPesPkt, ok := <-s.TsPesPktChan
//		if !ok {
//			fmt.Println("frame index, video , audio :", videoFrame, audioFrame)
//			break
//		}
//
//		if tsPesPkt.PesPkt.Header.StreamID == STREAM_ID_AUDIO {
//			audioFrame++
//		}
//
//		if tsPesPkt.PesPkt.Header.StreamID == STREAM_ID_VIDEO {
//			println(tsPesPkt.PesPkt.Header.Pts)
//			videoFrame++
//		}
//
//		fmt.Sprintf("%s", tsPesPkt)
//
//		// if err := WritePESPacket(file, tsPesPkt.TsPkt.Header, tsPesPkt.PesPkt); err != nil {
//		// 	return err
//		// }
//
//	}
//
//	return nil
//}

func (s *MpegTsStream) ReadPAT(packet *MpegTsPacket, pr io.Reader) (err error) {
	// 首先找到PID==0x00的TS包(PAT)
	if PID_PAT == packet.Header.Pid {
		if len(packet.Payload) == 188 {
			pr = &util.Crc32Reader{R: pr, Crc32: 0xffffffff}
		}
		// Header + PSI + Paylod
		s.PAT, err = ReadPAT(pr)
	}
	return
}
func (s *MpegTsStream) ReadPMT(packet *MpegTsPacket, pr io.Reader) (err error) {
	// 在读取PAT中已经将所有频道节目信息(PMT_PID)保存了起来
	// 接着读取所有TS包里面的PID,找出PID==PMT_PID的TS包,就是PMT表
	for _, v := range s.PAT.Program {
		if v.ProgramMapPID == packet.Header.Pid {
			if len(packet.Payload) == 188 {
				pr = &util.Crc32Reader{R: pr, Crc32: 0xffffffff}
			}
			// Header + PSI + Paylod
			s.PMT, err = ReadPMT(pr)
		}
	}
	return
}
func (s *MpegTsStream) Feed(ts io.Reader) (err error) {
	var reader bytes.Reader
	var lr io.LimitedReader
	lr.R = &reader
	var tsHeader MpegTsHeader
	tsData := make([]byte, TS_PACKET_SIZE)
	for {
		_, err = io.ReadFull(ts, tsData)
		if err == io.EOF {
			// 文件结尾 把最后面的数据发出去
			for _, pesPkt := range s.PESBuffer {
				if pesPkt != nil {
					s.PESChan <- pesPkt
				}
			}
			return nil
		} else if err != nil {
			return
		}
		reader.Reset(tsData)
		lr.N = TS_PACKET_SIZE
		if tsHeader, err = ReadTsHeader(&lr); err != nil {
			return
		}
		if tsHeader.Pid == PID_PAT {
			if s.PAT, err = ReadPAT(&lr); err != nil {
				return
			}
			continue
		}
		if len(s.PMT.Stream) == 0 {
			for _, v := range s.PAT.Program {
				if v.ProgramMapPID == tsHeader.Pid {
					if s.PMT, err = ReadPMT(&lr); err != nil {
						return
					}
					for _, v := range s.PMT.Stream {
						s.PESBuffer[v.ElementaryPID] = nil
					}
				}
				continue
			}
		} else if pesPkt, ok := s.PESBuffer[tsHeader.Pid]; ok {
			if tsHeader.PayloadUnitStartIndicator == 1 {
				if pesPkt != nil {
					s.PESChan <- pesPkt
				}
				pesPkt = &MpegTsPESPacket{}
				s.PESBuffer[tsHeader.Pid] = pesPkt
				if pesPkt.Header, err = ReadPESHeader(&lr); err != nil {
					return
				}
			}
			io.Copy(&pesPkt.Payload, &lr)
		}
	}
}
