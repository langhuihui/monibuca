package mpegts

import (
	"errors"
	"io"
	"m7s.live/v5/pkg/util"
	"net"
)

// ios13818-1-CN.pdf 45/166
//
// PES
//

// 每个传输流和节目流在逻辑上都是由 PES 包构造的
type MpegTsPesStream struct {
	TsPkt  MpegTsPacket
	PesPkt MpegTsPESPacket
}

// PES--Packetized  Elementary Streams  (分组的ES),ES形成的分组称为PES分组,是用来传递ES的一种数据结构
// 1110 xxxx 为视频流(0xE0)
// 110x xxxx 为音频流(0xC0)
type MpegTsPESPacket struct {
	Header  MpegTsPESHeader
	Payload util.Buffer //从TS包中读取的数据
	Buffers net.Buffers //用于写TS包
}

type MpegTsPESHeader struct {
	PacketStartCodePrefix uint32 // 24 bits 同跟随它的 stream_id 一起组成标识包起始端的包起始码.packet_start_code_prefix 为比特串"0000 0000 0000 0000 0000 0001"(0x000001)
	StreamID              byte   // 8 bits stream_id 指示基本流的类型和编号,如 stream_id 表 2-22 所定义的.传输流中,stream_id 可以设置为准确描述基本流类型的任何有效值,如表 2-22 所规定的.传输流中，基本流类型在 2.4.4 中所指示的节目特定信息中指定
	PesPacketLength       uint16 // 16 bits 指示 PES 包中跟随该字段最后字节的字节数.0->指示 PES 包长度既未指示也未限定并且仅在这样的 PES 包中才被允许,该 PES 包的有效载荷由来自传输流包中所包含的视频基本流的字节组成

	MpegTsOptionalPESHeader

	PayloadLength uint64 // 这个不是标准文档里面的字段,是自己添加的,方便计算
}

// 可选的PES Header = MpegTsOptionalPESHeader + stuffing bytes(0xFF) m * 8
type MpegTsOptionalPESHeader struct {
	ConstTen               byte // 2 bits 常量10
	PesScramblingControl   byte // 2 bit 指示 PES 包有效载荷的加扰方式.当加扰在 PES 等级上实施时, PES 包头,其中包括任选字段只要存在,应不加扰(见表 2-23)
	PesPriority            byte // 1 bit 指示在此 PES 包中该有效载荷的优先级.1->指示该 PES 包有效载荷比具有此字段置于"0"的其他 PES 包有效载荷有更高的有效载荷优先级.多路复用器能够使用该PES_priority 比特最佳化基本流内的数据
	DataAlignmentIndicator byte // 1 bit 1->指示 PES 包头之后紧随 2.6.10 中data_stream_alignment_descriptor 字段中指示的视频句法单元或音频同步字,只要该描述符字段存在.若置于值"1"并且该描述符不存在,则要求表 2-53,表 2-54 或表 2-55 的 alignment_type"01"中所指示的那种校准.0->不能确定任何此类校准是否发生
	Copyright              byte // 1 bit 1->指示相关 PES 包有效载荷的素材依靠版权所保护.0->不能确定该素材是否依靠版权所保护
	OriginalOrCopy         byte // 1 bit 1->指示相关 PES 包有效载荷的内容是原始的.0->指示相关 PES 包有效载荷的内容是复制的
	PtsDtsFlags            byte // 2 bits 10->PES 包头中 PTS 字段存在. 11->PES 包头中 PTS 字段和 DTS 字段均存在. 00->PES 包头中既无任何 PTS 字段也无任何 DTS 字段存在. 01->禁用
	EscrFlag               byte // 1 bit 1->指示 PES 包头中 ESCR 基准字段和 ESCR 扩展字段均存在.0->指示无任何 ESCR 字段存在
	EsRateFlag             byte // 1 bit 1->指示 PES 包头中 ES_rate 字段存在.0->指示无任何 ES_rate 字段存在
	DsmTrickModeFlag       byte // 1 bit 1->指示 8 比特特技方式字段存在.0->指示此字段不存在
	AdditionalCopyInfoFlag byte // 1 bit 1->指示 additional_copy_info 存在.0->时指示此字段不存在
	PesCRCFlag             byte // 1 bit 1->指示 PES 包中 CRC 字段存在.0->指示此字段不存在
	PesExtensionFlag       byte // 1 bit 1->时指示 PES 包头中扩展字段存在.0->指示此字段不存在
	PesHeaderDataLength    byte // 8 bits 指示在此 PES包头中包含的由任选字段和任意填充字节所占据的字节总数.任选字段的存在由前导 PES_header_data_length 字段的字节来指定

	// Optional Field
	Pts                  uint64 // 33 bits 指示时间与解码时间的关系如下: PTS 为三个独立字段编码的 33 比特数.它指示基本流 n 的显示单元 k 在系统目标解码器中的显示时间 tpn(k).PTS 值以系统时钟频率除以 300(产生 90 kHz)的周期为单位指定.显示时间依照以下公式 2-11 从 PTS 中推出.有关编码显示时间标记频率上的限制参阅 2.7.4
	Dts                  uint64 // 33 bits 指示基本流 n 的存取单元 j 在系统目标解码器中的解码时间 tdn(j). DTS 的值以系统时钟频率除以 300（生成90 kHz)的周期为单位指定.依照以下公式 2-12 从 DTS 中推出解码时间
	EscrBase             uint64 // 33 bits 其值由 ESCR_base(i) 给出,如公式 2-14 中给出的
	EscrExtension        uint16 // 9 bits 其值由 ESCR_ext(i) 给出,如公式 2-15 中给出的. ESCR 字段指示包含 ESCR_base 最后比特的字节到达 PES流的 PES-STD 输入端的预期时间(参阅 2.5.2.4)
	EsRate               uint32 // 22 bits 在PES 流情况中,指定系统目标解码器接收 PES 包字节的速率.ES_rate 在包括它的 PES 包以及相同 PES 流的后续 PES 包中持续有效直至遇到新的 ES_rate 字段时为止.ES 速率值以 50 字节/秒为度量单位.0 值禁用
	TrickModeControl     byte   // 3 bits 指示适用于相关视频流的特技方式.在其他类型基本流的情况中,此字段以及后随 5 比特所规定的那些含义未确定.对于 trick_mode 状态的定义,参阅 2.4.2.3 的特技方式段落
	TrickModeValue       byte   // 5 bits
	AdditionalCopyInfo   byte   // 7 bits 包含与版权信息有关的专用数据
	PreviousPESPacketCRC uint16 // 16 bits 包含产生解码器中 16 寄存器零输出的 CRC 值, 类似于附件 A 中定义的解码器. 但在处理先前的 PES 包数据字节之后, PES 包头除外,采用多项式

	// PES Extension
	PesPrivateDataFlag               byte // 1 bit 1->指示该 PES 包头包含专用数据. 0->指示 PES 包头中不存在专用数据
	PackHeaderFieldFlag              byte // 1 bit 1->指示 ISO/IEC 11172-1 包头或节目流包头在此 PES包头中存储.若此字段处于节目流中包含的 PES 包中,则此字段应设置为"0.传输流中, 0->指示该 PES 头中无任何包头存在
	ProgramPacketSequenceCounterFlag byte // 1 bit 1->指示 program_packet_sequence_counter, MPEG1_MPEG2_identifier 以及 original_stuff_length 字段在 PES 包中存在.0->它指示这些字段在 PES 头中不存在
	PSTDBufferFlag                   byte // 1 bit 1->指示 P-STD_buffer_scale 和 P-STD_buffer_size 在 PES包头中存在.0->指示这些字段在 PES 头中不存在
	Reserved                         byte // 3 bits
	PesExtensionFlag2                byte // 1 bits 1->指示 PES_extension_field_length 字段及相关的字段存在.0->指示 PES_extension_field_length 字段以及任何相关的字段均不存在.

	// Optional Field
	PesPrivateData               [16]byte // 128 bits 此数据,同前后字段数据结合,应不能仿真packet_start_code_prefix (0x000001)
	PackHeaderField              byte     // 8 bits 指示 pack_header_field() 的长度，以字节为单位
	ProgramPacketSequenceCounter byte     // 7 bits
	Mpeg1Mpeg2Identifier         byte     // 1 bit 1->指示此 PES 包承载来自 ISO/IEC 11172-1 流的信息.0->指示此 PES 包承载来自节目流的信息
	OriginalStuffLength          byte     // 6 bits 在原始 ITU-T H.222.0 建议书| ISO/IEC 13818-1 PES 包头或在原始 ISO/IEC 11172-1 包头中所使用的填充字节数
	PSTDBufferScale              byte     // 1bit 它的含义仅当节目流中包含此 PES 包时才规定.它指示所使用的标度因子用于解释后续的 P-STD_buffer_size 字段.若前导 stream_id 指示音频流,则P-STD 缓冲器标度字段必为"0"值.若前导 stream_id 指示视频流,则 P-STD_buffer_scale 字段必为"1"值.对于所有其他流类型,该值可为"1"或为"0"
	PSTDBufferSize               uint16   // 13 bits 其含义仅当节目流中包含此 PES包时才规定.它规定在 P-STD 中,输入缓冲器 BSn 的尺寸.若 STD_buffer_scale 为 "0"值，则 P-STD_buffer_size以 128 字节为单位度量该缓冲器尺寸.若 P-STD_buffer_scale 为"1",则 P-STD_buffer_size 以 1024 字节为单位度量该缓冲器尺寸
	PesExtensionFieldLength      byte     // 7 bits 指示 PES 扩展字段中跟随此长度字段的直至并包括任何保留字节为止的数据长度,以字节为度量单位
	StreamIDExtensionFlag        byte     // 1 bits
	//pesExtensionField              []byte   // PES_extension_field_length bits
	//packField                        []byte   // pack_field_length bits
}

// pts_dts_Flags == "10" -> PTS
// 0010				4
// PTS[32...30]		3
// marker_bit		1
// PTS[29...15]		15
// marker_bit		1
// PTS[14...0]		15
// marker_bit		1

// pts_dts_Flags == "11" -> PTS + DTS

type MpegtsPESFrame struct {
	Pid                       uint16
	IsKeyFrame                bool
	ContinuityCounter         byte
	ProgramClockReferenceBase uint64
}

func ReadPESHeader(r io.Reader) (header MpegTsPESHeader, err error) {
	var flags uint8
	var length uint

	// packetStartCodePrefix(24) (0x000001)
	header.PacketStartCodePrefix, err = util.ReadByteToUint24(r, true)
	if err != nil {
		return
	}

	if header.PacketStartCodePrefix != 0x0000001 {
		err = errors.New("read PacketStartCodePrefix is not 0x0000001")
		return
	}

	// streamID(8)
	header.StreamID, err = util.ReadByteToUint8(r)
	if err != nil {
		return
	}

	// pes_PacketLength(16)
	header.PesPacketLength, err = util.ReadByteToUint16(r, true)
	if err != nil {
		return
	}

	length = uint(header.PesPacketLength)

	// PES包长度可能为0,这个时候,需要自己去算
	// 0 <= len <= 65535
	// 如果当length为0,那么先设置为最大值,然后用LimitedReade去读,如果读到最后面剩下的字节数小于65536,才是正确的包大小.
	// 一个包一般情况下不可能会读1<<31个字节.
	if length == 0 {
		length = 1 << 31
	}

	// lrPacket 和 lrHeader 位置指针是在同一位置的
	lrPacket := &io.LimitedReader{R: r, N: int64(length)}
	lrHeader := lrPacket

	// constTen(2)
	// pes_ScramblingControl(2)
	// pes_Priority(1)
	// dataAlignmentIndicator(1)
	// copyright(1)
	// originalOrCopy(1)
	flags, err = util.ReadByteToUint8(lrHeader)
	if err != nil {
		return
	}

	header.ConstTen = flags & 0xc0
	header.PesScramblingControl = flags & 0x30
	header.PesPriority = flags & 0x08
	header.DataAlignmentIndicator = flags & 0x04
	header.Copyright = flags & 0x02
	header.OriginalOrCopy = flags & 0x01

	// pts_dts_Flags(2)
	// escr_Flag(1)
	// es_RateFlag(1)
	// dsm_TrickModeFlag(1)
	// additionalCopyInfoFlag(1)
	// pes_CRCFlag(1)
	// pes_ExtensionFlag(1)
	flags, err = util.ReadByteToUint8(lrHeader)
	if err != nil {
		return
	}

	header.PtsDtsFlags = flags & 0xc0
	header.EscrFlag = flags & 0x20
	header.EsRateFlag = flags & 0x10
	header.DsmTrickModeFlag = flags & 0x08
	header.AdditionalCopyInfoFlag = flags & 0x04
	header.PesCRCFlag = flags & 0x02
	header.PesExtensionFlag = flags & 0x01

	// pes_HeaderDataLength(8)
	header.PesHeaderDataLength, err = util.ReadByteToUint8(lrHeader)
	if err != nil {
		return
	}

	length = uint(header.PesHeaderDataLength)

	lrHeader = &io.LimitedReader{R: lrHeader, N: int64(length)}

	// 00 -> PES 包头中既无任何PTS 字段也无任何DTS 字段存在
	// 10 -> PES 包头中PTS 字段存在
	// 11 -> PES 包头中PTS 字段和DTS 字段均存在
	// 01 -> 禁用

	// PTS(33)
	if flags&0x80 != 0 {
		var pts uint64
		pts, err = util.ReadByteToUint40(lrHeader, true)
		if err != nil {
			return
		}

		header.Pts = util.GetPtsDts(pts)
	}

	// DTS(33)
	if flags&0x80 != 0 && flags&0x40 != 0 {
		var dts uint64
		dts, err = util.ReadByteToUint40(lrHeader, true)
		if err != nil {
			return
		}

		header.Dts = util.GetPtsDts(dts)
	}

	// reserved(2) + escr_Base1(3) + marker_bit(1) +
	// escr_Base2(15) + marker_bit(1) + escr_Base23(15) +
	// marker_bit(1) + escr_Extension(9) + marker_bit(1)
	if header.EscrFlag != 0 {
		_, err = util.ReadByteToUint48(lrHeader, true)
		if err != nil {
			return
		}

		//s.pes.escr_Base = escrBaseEx & 0x3fffffffe00
		//s.pes.escr_Extension = uint16(escrBaseEx & 0x1ff)
	}

	// es_Rate(22)
	if header.EsRateFlag != 0 {
		header.EsRate, err = util.ReadByteToUint24(lrHeader, true)
		if err != nil {
			return
		}
	}

	// 不知道为什么这里不用
	/*
		// trickModeControl(3) + trickModeValue(5)
		if s.pes.dsm_TrickModeFlag != 0 {
			trickMcMv, err := util.ReadByteToUint8(lrHeader)
			if err != nil {
				return err
			}

			s.pes.trickModeControl = trickMcMv & 0xe0
			s.pes.trickModeValue = trickMcMv & 0x1f
		}
	*/

	// marker_bit(1) + additionalCopyInfo(7)
	if header.AdditionalCopyInfoFlag != 0 {
		header.AdditionalCopyInfo, err = util.ReadByteToUint8(lrHeader)
		if err != nil {
			return
		}

		header.AdditionalCopyInfo = header.AdditionalCopyInfo & 0x7f
	}

	// previous_PES_Packet_CRC(16)
	if header.PesCRCFlag != 0 {
		header.PreviousPESPacketCRC, err = util.ReadByteToUint16(lrHeader, true)
		if err != nil {
			return
		}
	}

	// pes_PrivateDataFlag(1) + packHeaderFieldFlag(1) + programPacketSequenceCounterFlag(1) +
	// p_STD_BufferFlag(1) + reserved(3) + pes_ExtensionFlag2(1)
	if header.PesExtensionFlag != 0 {
		var flags uint8
		flags, err = util.ReadByteToUint8(lrHeader)
		if err != nil {
			return
		}

		header.PesPrivateDataFlag = flags & 0x80
		header.PackHeaderFieldFlag = flags & 0x40
		header.ProgramPacketSequenceCounterFlag = flags & 0x20
		header.PSTDBufferFlag = flags & 0x10
		header.PesExtensionFlag2 = flags & 0x01

		// TODO:下面所有的标志位,可能获取到的数据,都简单的读取后,丢弃,如果日后需要,在这里处理

		// pes_PrivateData(128)
		if header.PesPrivateDataFlag != 0 {
			if _, err = io.CopyN(io.Discard, lrHeader, int64(16)); err != nil {
				return
			}
		}

		// packFieldLength(8)
		if header.PackHeaderFieldFlag != 0 {
			if _, err = io.CopyN(io.Discard, lrHeader, int64(1)); err != nil {
				return
			}
		}

		// marker_bit(1) + programPacketSequenceCounter(7) + marker_bit(1) +
		// mpeg1_mpeg2_Identifier(1) + originalStuffLength(6)
		if header.ProgramPacketSequenceCounterFlag != 0 {
			if _, err = io.CopyN(io.Discard, lrHeader, int64(2)); err != nil {
				return
			}
		}

		// 01 + p_STD_bufferScale(1) + p_STD_bufferSize(13)
		if header.PSTDBufferFlag != 0 {
			if _, err = io.CopyN(io.Discard, lrHeader, int64(2)); err != nil {
				return
			}
		}

		// marker_bit(1) + pes_Extension_Field_Length(7) +
		// streamIDExtensionFlag(1)
		if header.PesExtensionFlag != 0 {
			if _, err = io.CopyN(io.Discard, lrHeader, int64(2)); err != nil {
				return
			}
		}
	}

	// 把剩下的头的数据消耗掉
	if lrHeader.N > 0 {
		if _, err = io.CopyN(io.Discard, lrHeader, int64(lrHeader.N)); err != nil {
			return
		}
	}

	// 2的16次方,16个字节
	if lrPacket.N < 65536 {
		// 这里得到的其实是负载长度,因为已经偏移过了Header部分.
		//header.pes_PacketLength = uint16(lrPacket.N)
		header.PayloadLength = uint64(lrPacket.N)
	}

	return
}

func WritePESHeader(w io.Writer, header MpegTsPESHeader) (written int, err error) {
	if header.PacketStartCodePrefix != 0x0000001 {
		err = errors.New("write PacketStartCodePrefix is not 0x0000001")
		return
	}

	// packetStartCodePrefix(24) (0x000001)
	if err = util.WriteUint24ToByte(w, header.PacketStartCodePrefix, true); err != nil {
		return
	}

	written += 3

	// streamID(8)
	if err = util.WriteUint8ToByte(w, header.StreamID); err != nil {
		return
	}

	written += 1

	// pes_PacketLength(16)
	// PES包长度可能为0,这个时候,需要自己去算
	// 0 <= len <= 65535
	if err = util.WriteUint16ToByte(w, header.PesPacketLength, true); err != nil {
		return
	}

	//fmt.Println("Length :", payloadLength)
	//fmt.Println("PES Packet Length :", header.pes_PacketLength)

	written += 2

	// constTen(2)
	// pes_ScramblingControl(2)
	// pes_Priority(1)
	// dataAlignmentIndicator(1)
	// copyright(1)
	// originalOrCopy(1)
	// 1000 0001
	if header.ConstTen != 0x80 {
		err = errors.New("pes header ConstTen != 0x80")
		return
	}

	flags := header.ConstTen | header.PesScramblingControl | header.PesPriority | header.DataAlignmentIndicator | header.Copyright | header.OriginalOrCopy
	if err = util.WriteUint8ToByte(w, flags); err != nil {
		return
	}

	written += 1

	// pts_dts_Flags(2)
	// escr_Flag(1)
	// es_RateFlag(1)
	// dsm_TrickModeFlag(1)
	// additionalCopyInfoFlag(1)
	// pes_CRCFlag(1)
	// pes_ExtensionFlag(1)
	sevenFlags := header.PtsDtsFlags | header.EscrFlag | header.EsRateFlag | header.DsmTrickModeFlag | header.AdditionalCopyInfoFlag | header.PesCRCFlag | header.PesExtensionFlag
	if err = util.WriteUint8ToByte(w, sevenFlags); err != nil {
		return
	}

	written += 1

	// pes_HeaderDataLength(8)
	if err = util.WriteUint8ToByte(w, header.PesHeaderDataLength); err != nil {
		return
	}

	written += 1

	// PtsDtsFlags == 192(11), 128(10), 64(01)禁用, 0(00)
	if header.PtsDtsFlags&0x80 != 0 {
		// PTS和DTS都存在(11),否则只有PTS(10)
		if header.PtsDtsFlags&0x80 != 0 && header.PtsDtsFlags&0x40 != 0 {
			// 11:PTS和DTS
			// PTS(33) + 4 + 3
			pts := util.PutPtsDts(header.Pts) | 3<<36
			if err = util.WriteUint40ToByte(w, pts, true); err != nil {
				return
			}

			written += 5

			// DTS(33) + 4 + 3
			dts := util.PutPtsDts(header.Dts) | 1<<36
			if err = util.WriteUint40ToByte(w, dts, true); err != nil {
				return
			}

			written += 5
		} else {
			// 10:只有PTS
			// PTS(33) + 4 + 3
			pts := util.PutPtsDts(header.Pts) | 2<<36
			if err = util.WriteUint40ToByte(w, pts, true); err != nil {
				return
			}

			written += 5
		}
	}

	return
}
