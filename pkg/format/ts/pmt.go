package mpegts

import (
	"bytes"
	"io"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
	"net"
)

// ios13818-1-CN.pdf 46(60)-153(167)/page
//
// PMT

var (
	TSHeader = []byte{0x47, 0x40 | (PID_PMT >> 8), PID_PMT & 0xff, 0x10, 0x00} //PID:0x100
	PSI      = []byte{0x02, 0xb0, 0x17, 0x00, 0x01, 0xc1, 0x00, 0x00}
	PMT      = []byte{0xe0 | (PID_VIDEO >> 8), PID_VIDEO & 0xff, 0xf0, 0x00} //PcrPID:0x101
	h264     = []byte{STREAM_TYPE_H264, 0xe0 | (PID_VIDEO >> 8), PID_VIDEO & 0xff, 0xf0, 0x00}
	h265     = []byte{STREAM_TYPE_H265, 0xe0 | (PID_VIDEO >> 8), PID_VIDEO & 0xff, 0xf0, 0x00}
	aac      = []byte{STREAM_TYPE_AAC, 0xe0 | (PID_AUDIO >> 8), PID_AUDIO & 0xff, 0xf0, 0x00}
	pcma     = []byte{STREAM_TYPE_G711A, 0xe0 | (PID_AUDIO >> 8), PID_AUDIO & 0xff, 0xf0, 0x00}
	pcmu     = []byte{STREAM_TYPE_G711U, 0xe0 | (PID_AUDIO >> 8), PID_AUDIO & 0xff, 0xf0, 0x00}
	Stuffing []byte
)

func init() {
	Stuffing = util.GetFillBytes(0xff, TS_PACKET_SIZE)
}

// TS Header :
// SyncByte = 0x47
// TransportErrorIndicator = 0(B:0), PayloadUnitStartIndicator = 1(B:0), TransportPriority = 0(B:0),
// Pid = 4097(0x1001),
// TransportScramblingControl = 0(B:00), AdaptionFieldControl = 1(B:01), ContinuityCounter = 0(B:0000),

// PSI :
// TableID = 0x02,
// SectionSyntaxIndicator = 1(B:1), Zero = 0(B:0), Reserved1 = 3(B:11),
// SectionLength = 23(0x17)
// ProgramNumber = 0x0001
// Reserved2 = 3(B:11), VersionNumber = (B:00000), CurrentNextIndicator = 1(B:0),
// SectionNumber = 0x00
// LastSectionNumber = 0x00

// PMT:
// Reserved3 = 15(B:1110), PcrPID = 256(0x100)
// Reserved4 = 16(B:1111), ProgramInfoLength = 0(0x000)
// H264:
// StreamType = 0x1b,
// Reserved5 = 15(B:1110), ElementaryPID = 256(0x100)
// Reserved6 = 16(B:1111), EsInfoLength = 0(0x000)
// AAC:
// StreamType = 0x0f,
// Reserved5 = 15(B:1110), ElementaryPID = 257(0x101)
// Reserved6 = 16(B:1111), EsInfoLength = 0(0x000)

type MpegTsPmtStream struct {
	StreamType    byte   // 8 bits 指示具有 PID值的包内承载的节目元类型,其 PID值由 elementary_PID所指定
	Reserved5     byte   // 3 bits 保留位
	ElementaryPID uint16 // 13 bits 指定承载相关节目元的传输流包的 PID
	Reserved6     byte   // 4 bits 保留位
	EsInfoLength  uint16 // 12 bits 该字段的头两比特必为'00',剩余 10比特指示紧随 ES_info_length字段的相关节目元描述符的字节数

	// N Loop Descriptors
	Descriptor []MpegTsDescriptor // 不确定字节数,可变
}

// Program Map Table (节目映射表)
type MpegTsPMT struct {
	// PSI
	TableID                byte   // 8 bits 0x00->PAT,0x02->PMT
	SectionSyntaxIndicator byte   // 1 bit  段语法标志位,固定为1
	Zero                   byte   // 1 bit  0
	Reserved1              byte   // 2 bits 保留位
	SectionLength          uint16 // 12 bits 该字段的头两比特必为'00',剩余 10 比特指定该分段的字节数,紧随 section_length 字段开始,并包括 CRC.此字段中的值应不超过 1021(0x3FD)
	ProgramNumber          uint16 // 16 bits 指定 program_map_PID 所适用的节目
	Reserved2              byte   // 2 bits  保留位
	VersionNumber          byte   // 5 bits  范围0-31,表示PAT的版本号
	CurrentNextIndicator   byte   // 1 bit  发送的PAT是当前有效还是下一个PAT有效
	SectionNumber          byte   // 8 bits  分段的号码.PAT可能分为多段传输.第一段为00,以后每个分段加1,最多可能有256个分段
	LastSectionNumber      byte   // 8 bits  最后一个分段的号码

	Reserved3             byte               // 3 bits  保留位 0x07
	PcrPID                uint16             // 13 bits 指明TS包的PID值.该TS包含有PCR域,该PCR值对应于由节目号指定的对应节目.如果对于私有数据流的节目定义与PCR无关.这个域的值将为0x1FFF
	Reserved4             byte               // 4 bits  预留位 0x0F
	ProgramInfoLength     uint16             // 12 bits 前两位bit为00.该域指出跟随其后对节目信息的描述的byte数
	ProgramInfoDescriptor []MpegTsDescriptor // N Loop Descriptors 可变 节目信息描述

	// N Loop
	Stream []MpegTsPmtStream // PMT表里面的所有音视频索引信息

	Crc32 uint32 // 32 bits 包含处理全部传输流节目映射分段之后,在附件 B 规定的解码器中给出寄存器零输出的 CRC 值
}

func ReadPMT(r io.Reader) (pmt MpegTsPMT, err error) {
	lr, psi, err := ReadPSI(r, PSI_TYPE_PMT)
	if err != nil {
		return
	}

	pmt = psi.Pmt

	// reserved3(3) + pcrPID(13)
	pcrPID, err := util.ReadByteToUint16(lr, true)
	if err != nil {
		return
	}

	pmt.PcrPID = pcrPID & 0x1fff

	// reserved4(4) + programInfoLength(12)
	// programInfoLength(12) == 0x00(固定为0) + programInfoLength(10)
	programInfoLength, err := util.ReadByteToUint16(lr, true)
	if err != nil {
		return
	}

	pmt.ProgramInfoLength = programInfoLength & 0x3ff

	// 如果length>0那么,紧跟programInfoLength后面就有length个字节
	if pmt.ProgramInfoLength > 0 {
		lr := &io.LimitedReader{R: lr, N: int64(pmt.ProgramInfoLength)}
		pmt.ProgramInfoDescriptor, err = ReadPMTDescriptor(lr)
		if err != nil {
			return
		}
	}

	// N Loop
	// 开始N循环,读取所有的流的信息
	for lr.N > 0 {
		var streams MpegTsPmtStream
		// streamType(8)
		streams.StreamType, err = util.ReadByteToUint8(lr)
		if err != nil {
			return
		}

		// reserved5(3) + elementaryPID(13)
		streams.ElementaryPID, err = util.ReadByteToUint16(lr, true)
		if err != nil {
			return
		}

		streams.ElementaryPID = streams.ElementaryPID & 0x1fff

		// reserved6(4) + esInfoLength(12)
		// esInfoLength(12) == 0x00(固定为0) + esInfoLength(10)
		streams.EsInfoLength, err = util.ReadByteToUint16(lr, true)
		if err != nil {
			return
		}

		streams.EsInfoLength = streams.EsInfoLength & 0x3ff

		// 如果length>0那么,紧跟esInfoLength后面就有length个字节
		if streams.EsInfoLength > 0 {
			lr := &io.LimitedReader{R: lr, N: int64(streams.EsInfoLength)}
			streams.Descriptor, err = ReadPMTDescriptor(lr)
			if err != nil {
				return
			}
		}

		// 每读取一个流的信息(音频流或者视频流或者其他),都保存起来
		pmt.Stream = append(pmt.Stream, streams)
	}
	if cr, ok := r.(*util.Crc32Reader); ok {
		err = cr.ReadCrc32UIntAndCheck()
		if err != nil {
			return
		}
	}
	return
}

func ReadPMTDescriptor(lr *io.LimitedReader) (Desc []MpegTsDescriptor, err error) {
	var desc MpegTsDescriptor
	for lr.N > 0 {
		// tag (8)
		desc.Tag, err = util.ReadByteToUint8(lr)
		if err != nil {
			return
		}

		// length (8)
		desc.Length, err = util.ReadByteToUint8(lr)
		if err != nil {
			return
		}

		desc.Data = make([]byte, desc.Length)
		_, err = lr.Read(desc.Data)
		if err != nil {
			return
		}

		Desc = append(Desc, desc)
	}

	return
}

func WritePMTDescriptor(w io.Writer, descs []MpegTsDescriptor) (err error) {
	for _, desc := range descs {
		// tag(8)
		if err = util.WriteUint8ToByte(w, desc.Tag); err != nil {
			return
		}

		// length (8)
		if err = util.WriteUint8ToByte(w, uint8(len(desc.Data))); err != nil {
			return
		}

		// data
		if _, err = w.Write(desc.Data); err != nil {
			return
		}
	}

	return
}

func WritePMTBody(w io.Writer, pmt MpegTsPMT) (err error) {
	// reserved3(3) + pcrPID(13)
	if err = util.WriteUint16ToByte(w, pmt.PcrPID|7<<13, true); err != nil {
		return
	}

	// programInfoDescriptor 节目信息描述,字节数不能确定
	bw := &bytes.Buffer{}
	if err = WritePMTDescriptor(bw, pmt.ProgramInfoDescriptor); err != nil {
		return
	}

	pmt.ProgramInfoLength = uint16(bw.Len())

	// reserved4(4) + programInfoLength(12)
	// programInfoLength(12) == 0x00(固定为0) + programInfoLength(10)
	if err = util.WriteUint16ToByte(w, pmt.ProgramInfoLength|0xf000, true); err != nil {
		return
	}

	// programInfoDescriptor
	if _, err = w.Write(bw.Bytes()); err != nil {
		return
	}

	// 循环读取所有的流的信息(音频或者视频)
	for _, esinfo := range pmt.Stream {
		// streamType(8)
		if err = util.WriteUint8ToByte(w, esinfo.StreamType); err != nil {
			return
		}

		// reserved5(3) + elementaryPID(13)
		if err = util.WriteUint16ToByte(w, esinfo.ElementaryPID|7<<13, true); err != nil {
			return
		}

		// descriptor ES流信息描述,字节数不能确定
		bw := &bytes.Buffer{}
		if err = WritePMTDescriptor(bw, esinfo.Descriptor); err != nil {
			return
		}

		esinfo.EsInfoLength = uint16(bw.Len())

		// reserved6(4) + esInfoLength(12)
		// esInfoLength(12) == 0x00(固定为0) + esInfoLength(10)
		if err = util.WriteUint16ToByte(w, esinfo.EsInfoLength|0xf000, true); err != nil {
			return
		}

		// descriptor
		if _, err = w.Write(bw.Bytes()); err != nil {
			return
		}
	}

	return
}

func WritePMT(w io.Writer, pmt MpegTsPMT) (err error) {
	bw := &bytes.Buffer{}

	if err = WritePMTBody(bw, pmt); err != nil {
		return
	}

	if pmt.SectionLength == 0 {
		pmt.SectionLength = 2 + 3 + 4 + uint16(len(bw.Bytes()))
	}

	psi := MpegTsPSI{}

	psi.Pmt = pmt

	if err = WritePSI(w, PSI_TYPE_PMT, psi, bw.Bytes()); err != nil {
		return
	}

	return
}

// func WritePMTPacket(w io.Writer, tsHeader []byte, pmt MpegTsPMT) (err error) {
// 	if pmt.TableID != TABLE_TSPMS {
// 		err = errors.New("PMT table ID error")
// 		return
// 	}

// 	// 将所有要写的数据(PMT),全部放入到buffer中去.
// 	// 	buffer 里面已经写好了整个PMT表(PointerField+PSI+PMT+CRC)
// 	bw := &bytes.Buffer{}
// 	if err = WritePMT(bw, pmt); err != nil {
// 		return
// 	}

// 	// TODO:如果Pmt.Stream里面包含的信息很大,大于188?
// 	stuffingBytes := util.GetFillBytes(0xff, TS_PACKET_SIZE-4-bw.Len())

// 	var PMTPacket []byte
// 	PMTPacket = append(PMTPacket, tsHeader...)
// 	PMTPacket = append(PMTPacket, bw.Bytes()...)
// 	PMTPacket = append(PMTPacket, stuffingBytes...)

// 	fmt.Println("-------------------------")
// 	fmt.Println("Write PMT :", PMTPacket)
// 	fmt.Println("-------------------------")

// 	// 写PMT负载
// 	if _, err = w.Write(PMTPacket); err != nil {
// 		return
// 	}

// 	return
// }

func WritePMTPacket(w io.Writer, videoCodec codec.FourCC, audioCodec codec.FourCC) {
	w.Write(TSHeader)
	crc := make([]byte, 4)
	paddingSize := TS_PACKET_SIZE - len(crc) - len(PSI) - len(PMT) - len(TSHeader) - 10
	pmt := net.Buffers{PSI, PMT}
	switch videoCodec {
	case codec.FourCC_H264:
		pmt = append(pmt, h264)
	case codec.FourCC_H265:
		pmt = append(pmt, h265)
	default:
		paddingSize += 5
	}
	switch audioCodec {
	case codec.FourCC_MP4A:
		pmt = append(pmt, aac)
	case codec.FourCC_ALAW:
		pmt = append(pmt, pcma)
	case codec.FourCC_ULAW:
		pmt = append(pmt, pcmu)
	default:
		paddingSize += 5
	}
	util.PutBE(crc, GetCRC32_2(pmt))
	pmt = append(pmt, crc, Stuffing[:paddingSize])
	pmt.WriteTo(w)
}
