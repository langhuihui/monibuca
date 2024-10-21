package mpegts

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"m7s.live/v5/pkg/util"
)

// ios13818-1-CN.pdf 43(57)/166
//
// PAT
//

var DefaultPATPacket = []byte{
	// TS Header
	0x47, 0x40, 0x00, 0x10,

	// Pointer Field
	0x00,

	// PSI
	0x00, 0xb0, 0x0d, 0x00, 0x01, 0xc1, 0x00, 0x00,

	// PAT
	0x00, 0x01, 0xe1, 0x00,

	// CRC
	0xe8, 0xf9, 0x5e, 0x7d,

	// Stuffing 167 bytes
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
}

// TS Header :
// SyncByte = 0x47
// TransportErrorIndicator = 0(B:0), PayloadUnitStartIndicator = 1(B:0), TransportPriority = 0(B:0),
// Pid = 0,
// TransportScramblingControl = 0(B:00), AdaptionFieldControl = 1(B:01), ContinuityCounter = 0(B:0000),

// PSI :
// TableID = 0x00,
// SectionSyntaxIndicator = 1(B:1), Zero = 0(B:0), Reserved1 = 3(B:11),
// SectionLength = 13(0x00d)
// TransportStreamID = 0x0001
// Reserved2 = 3(B:11), VersionNumber = (B:00000), CurrentNextIndicator = 1(B:0),
// SectionNumber = 0x00
// LastSectionNumber = 0x00

// PAT :
// ProgramNumber = 0x0001
// Reserved3 = 15(B:1110), ProgramMapPID = 4097(0x1001)

// PAT表主要包含频道号码和每一个频道对应的PMT的PID号码,这些信息我们在处理PAT表格的时候会保存起来，以后会使用到这些数据
type MpegTsPATProgram struct {
	ProgramNumber uint16 // 16 bit 节目号
	Reserved3     byte   // 3 bits 保留位
	NetworkPID    uint16 // 13 bits 网络信息表(NIT)的PID,节目号为0时对应的PID为network_PID
	ProgramMapPID uint16 // 13 bit 节目映射表的PID,节目号大于0时对应的PID.每个节目对应一个
}

// Program Association Table (节目关联表)
// 节目号为0x0000时,表示这是NIT,PID=0x001f,即3.
// 节目号为0x0001时,表示这是PMT,PID=0x100,即256
type MpegTsPAT struct {
	// PSI
	TableID                byte   // 8 bits 0x00->PAT,0x02->PMT
	SectionSyntaxIndicator byte   // 1 bit  段语法标志位,固定为1
	Zero                   byte   // 1 bit  0
	Reserved1              byte   // 2 bits 保留位
	SectionLength          uint16 // 12 bits 该字段的头两比特必为'00',剩余 10 比特指定该分段的字节数,紧随 section_length 字段开始,并包括 CRC.此字段中的值应不超过 1021(0x3FD)
	TransportStreamID      uint16 // 16 bits 该字段充当标签,标识网络内此传输流有别于任何其他多路复用流.其值由用户规定
	Reserved2              byte   // 2 bits  保留位
	VersionNumber          byte   // 5 bits  范围0-31,表示PAT的版本号
	CurrentNextIndicator   byte   // 1 bit  发送的PAT是当前有效还是下一个PAT有效,0则要等待下一个表
	SectionNumber          byte   // 8 bits  分段的号码.PAT可能分为多段传输.第一段为00,以后每个分段加1,最多可能有256个分段
	LastSectionNumber      byte   // 8 bits  最后一个分段的号码

	// N Loop
	Program []MpegTsPATProgram // PAT表里面的所有频道索引信息

	Crc32 uint32 // 32 bits 包含处理全部传输流节目映射分段之后,在附件 B 规定的解码器中给出寄存器零输出的 CRC 值
}

func ReadPAT(r io.Reader) (pat MpegTsPAT, err error) {
	lr, psi, err := ReadPSI(r, PSI_TYPE_PAT)
	if err != nil {
		return
	}

	pat = psi.Pat

	// N Loop
	// 一直循环去读4个字节,用lr的原因是确保不会读过头了.
	for lr.N > 0 {

		// 获取每一个频道的节目信息,保存起来
		programs := MpegTsPATProgram{}

		programs.ProgramNumber, err = util.ReadByteToUint16(lr, true)
		if err != nil {
			return
		}

		// 如果programNumber为0,则是NetworkPID,否则是ProgramMapPID(13)
		if programs.ProgramNumber == 0 {
			programs.NetworkPID, err = util.ReadByteToUint16(lr, true)
			if err != nil {
				return
			}

			programs.NetworkPID = programs.NetworkPID & 0x1fff
		} else {
			programs.ProgramMapPID, err = util.ReadByteToUint16(lr, true)
			if err != nil {
				return
			}

			programs.ProgramMapPID = programs.ProgramMapPID & 0x1fff
		}

		pat.Program = append(pat.Program, programs)
	}
	if cr, ok := r.(*util.Crc32Reader); ok {
		err = cr.ReadCrc32UIntAndCheck()
		if err != nil {
			return
		}
	}

	return
}

func WritePAT(w io.Writer, pat MpegTsPAT) (err error) {
	bw := &bytes.Buffer{}

	// 将pat(所有的节目索引信息)写入到缓冲区中
	for _, pats := range pat.Program {
		if err = util.WriteUint16ToByte(bw, pats.ProgramNumber, true); err != nil {
			return
		}

		if pats.ProgramNumber == 0 {
			if err = util.WriteUint16ToByte(bw, pats.NetworkPID&0x1fff|7<<13, true); err != nil {
				return
			}
		} else {
			// | 0001 1111 | 1111 1111 |
			// 7 << 13 -> 1110 0000 0000 0000
			if err = util.WriteUint16ToByte(bw, pats.ProgramMapPID&0x1fff|7<<13, true); err != nil {
				return
			}
		}
	}

	if pat.SectionLength == 0 {
		pat.SectionLength = 2 + 3 + 4 + uint16(len(bw.Bytes()))
	}

	psi := MpegTsPSI{}

	psi.Pat = pat

	if err = WritePSI(w, PSI_TYPE_PAT, psi, bw.Bytes()); err != nil {
		return
	}

	return
}

func WritePATPacket(w io.Writer, tsHeader []byte, pat MpegTsPAT) (err error) {
	if pat.TableID != TABLE_PAS {
		err = errors.New("PAT table ID error")
		return
	}

	// 将所有要写的数据(PAT),全部放入到buffer中去.
	// 	buffer 里面已经写好了整个pat表(PointerField+PSI+PAT+CRC)
	bw := &bytes.Buffer{}
	if err = WritePAT(bw, pat); err != nil {
		return
	}

	// TODO:如果Pat.Program里面包含的信息很大,大于188?
	stuffingBytes := util.GetFillBytes(0xff, TS_PACKET_SIZE-4-bw.Len())

	// PATPacket = TsHeader + PAT + Stuffing Bytes
	var PATPacket []byte
	PATPacket = append(PATPacket, tsHeader...)
	PATPacket = append(PATPacket, bw.Bytes()...)
	PATPacket = append(PATPacket, stuffingBytes...)

	fmt.Println("-------------------------")
	fmt.Println("Write PAT :", PATPacket)
	fmt.Println("-------------------------")

	// 写PAT负载
	if _, err = w.Write(PATPacket); err != nil {
		return
	}

	return
}

func WriteDefaultPATPacket(w io.Writer) (err error) {
	_, err = w.Write(DefaultPATPacket)
	if err != nil {
		return
	}

	return
}
