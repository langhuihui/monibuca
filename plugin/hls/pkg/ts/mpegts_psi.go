package mpegts

import (
	"errors"
	"fmt"
	"io"
	"m7s.live/v5/pkg/util"
)

//
// PSI
//

const (
	PSI_TYPE_PAT      = 1
	PSI_TYPE_PMT      = 2
	PSI_TYPE_NIT      = 3
	PSI_TYPE_CAT      = 4
	PSI_TYPE_TST      = 5
	PSI_TYPE_IPMP_CIT = 6
)

type MpegTsPSI struct {
	// PAT
	// PMT
	// CAT
	// NIT
	Pat MpegTsPAT
	Pmt MpegTsPMT
}

// 当传输流包有效载荷包含 PSI 数据时,payload_unit_start_indicator 具有以下意义:
// 若传输流包承载 PSI分段的首字节,则 payload_unit_start_indicator 值必为 1,指示此传输流包的有效载荷的首字节承载pointer_field.
// 若传输流包不承载 PSI 分段的首字节,则 payload_unit_start_indicator 值必为 0,指示在此有效载荷中不存在 pointer_field
// 只要是PSI就一定会有pointer_field
func ReadPSI(r io.Reader, pt uint32) (lr *io.LimitedReader, psi MpegTsPSI, err error) {
	// pointer field(8)
	cr, ok := r.(*util.Crc32Reader)
	if ok {
		r = cr.R
	}
	pointer_field, err := util.ReadByteToUint8(r)
	if err != nil {
		return
	}

	if pointer_field != 0 {
		// 无论如何都应该确保能将pointer_field读取到,并且io.Reader指针向下移动
		// ioutil.Discard常用在,http中,如果Get请求,获取到了很大的Body,要丢弃Body,就用这个方法.
		// 因为http默认重链接的时候,必须等body读取完成.
		// 用于发送需要读取但不想存储的数据,目的是耗尽读取端的数据
		if _, err = io.CopyN(io.Discard, r, int64(pointer_field)); err != nil {
			return
		}
	}
	if ok {
		r = cr
	}

	// table id(8)
	tableId, err := util.ReadByteToUint8(r)
	if err != nil {
		return
	}

	// sectionSyntaxIndicator(1) + zero(1)  + reserved1(2) + sectionLength(12)
	// sectionLength 前两个字节固定为00
	sectionSyntaxIndicatorAndSectionLength, err := util.ReadByteToUint16(r, true)
	if err != nil {
		return
	}

	// 指定该分段的字节数,紧随 section_length 字段开始,并包括 CRC
	// 因此剩下最多只能在读sectionLength长度的字节
	lr = &io.LimitedReader{R: r, N: int64(sectionSyntaxIndicatorAndSectionLength & 0x3FF)}

	// PAT TransportStreamID(16) or PMT ProgramNumber(16)
	transportStreamIdOrProgramNumber, err := util.ReadByteToUint16(lr, true)
	if err != nil {
		return
	}

	// reserved2(2) + versionNumber(5) + currentNextIndicator(1)
	versionNumberAndCurrentNextIndicator, err := util.ReadByteToUint8(lr)
	if err != nil {
		return
	}

	// sectionNumber(8)
	sectionNumber, err := util.ReadByteToUint8(lr)
	if err != nil {
		return
	}

	// lastSectionNumber(8)
	lastSectionNumber, err := util.ReadByteToUint8(lr)
	if err != nil {
		return
	}

	// 因为lr.N是从sectionLength开始计算,所以要减去 pointer_field(8) + table id(8) +  sectionSyntaxIndicator(1) + zero(1)  + reserved1(2) + sectionLength(12)
	lr.N -= 4

	switch pt {
	case PSI_TYPE_PAT:
		{
			if tableId != TABLE_PAS {
				err = errors.New(fmt.Sprintf("%s, id=%d", "read pmt table id != 2", tableId))
				return
			}

			psi.Pat.TableID = tableId
			psi.Pat.SectionSyntaxIndicator = uint8((sectionSyntaxIndicatorAndSectionLength & 0x8000) >> 15)
			psi.Pat.SectionLength = sectionSyntaxIndicatorAndSectionLength & 0x3FF
			psi.Pat.TransportStreamID = transportStreamIdOrProgramNumber
			psi.Pat.VersionNumber = versionNumberAndCurrentNextIndicator & 0x3e
			psi.Pat.CurrentNextIndicator = versionNumberAndCurrentNextIndicator & 0x01
			psi.Pat.SectionNumber = sectionNumber
			psi.Pat.LastSectionNumber = lastSectionNumber
		}
	case PSI_TYPE_PMT:
		{
			if tableId != TABLE_TSPMS {
				err = errors.New(fmt.Sprintf("%s, id=%d", "read pmt table id != 2", tableId))
				return
			}

			psi.Pmt.TableID = tableId
			psi.Pmt.SectionSyntaxIndicator = uint8((sectionSyntaxIndicatorAndSectionLength & 0x8000) >> 15)
			psi.Pmt.SectionLength = sectionSyntaxIndicatorAndSectionLength & 0x3FF
			psi.Pmt.ProgramNumber = transportStreamIdOrProgramNumber
			psi.Pmt.VersionNumber = versionNumberAndCurrentNextIndicator & 0x3e
			psi.Pmt.CurrentNextIndicator = versionNumberAndCurrentNextIndicator & 0x01
			psi.Pmt.SectionNumber = sectionNumber
			psi.Pmt.LastSectionNumber = lastSectionNumber
		}
	}

	return
}

func WritePSI(w io.Writer, pt uint32, psi MpegTsPSI, data []byte) (err error) {
	var tableId, versionNumberAndCurrentNextIndicator, sectionNumber, lastSectionNumber uint8
	var sectionSyntaxIndicatorAndSectionLength, transportStreamIdOrProgramNumber uint16

	switch pt {
	case PSI_TYPE_PAT:
		{
			if psi.Pat.TableID != TABLE_PAS {
				err = errors.New(fmt.Sprintf("%s, id=%d", "write pmt table id != 0", tableId))
				return
			}

			tableId = psi.Pat.TableID
			sectionSyntaxIndicatorAndSectionLength = uint16(psi.Pat.SectionSyntaxIndicator)<<15 | 3<<12 | psi.Pat.SectionLength
			transportStreamIdOrProgramNumber = psi.Pat.TransportStreamID
			versionNumberAndCurrentNextIndicator = psi.Pat.VersionNumber<<1 | psi.Pat.CurrentNextIndicator
			sectionNumber = psi.Pat.SectionNumber
			lastSectionNumber = psi.Pat.LastSectionNumber
		}
	case PSI_TYPE_PMT:
		{
			if psi.Pmt.TableID != TABLE_TSPMS {
				err = errors.New(fmt.Sprintf("%s, id=%d", "write pmt table id != 2", tableId))
				return
			}

			tableId = psi.Pmt.TableID
			sectionSyntaxIndicatorAndSectionLength = uint16(psi.Pmt.SectionSyntaxIndicator)<<15 | 3<<12 | psi.Pmt.SectionLength
			transportStreamIdOrProgramNumber = psi.Pmt.ProgramNumber
			versionNumberAndCurrentNextIndicator = psi.Pmt.VersionNumber<<1 | psi.Pmt.CurrentNextIndicator
			sectionNumber = psi.Pmt.SectionNumber
			lastSectionNumber = psi.Pmt.LastSectionNumber
		}
	}

	// pointer field(8)
	if err = util.WriteUint8ToByte(w, 0); err != nil {
		return
	}

	cw := &util.Crc32Writer{W: w, Crc32: 0xffffffff}

	// table id(8)
	if err = util.WriteUint8ToByte(cw, tableId); err != nil {
		return
	}

	// sectionSyntaxIndicator(1) + zero(1)  + reserved1(2) + sectionLength(12)
	// sectionLength 前两个字节固定为00
	// 1 0 11 sectionLength
	if err = util.WriteUint16ToByte(cw, sectionSyntaxIndicatorAndSectionLength, true); err != nil {
		return
	}

	// PAT TransportStreamID(16) or PMT ProgramNumber(16)
	if err = util.WriteUint16ToByte(cw, transportStreamIdOrProgramNumber, true); err != nil {
		return
	}

	// reserved2(2) + versionNumber(5) + currentNextIndicator(1)
	// 0x3 << 6 -> 1100 0000
	// 0x3 << 6  | 1 -> 1100 0001
	if err = util.WriteUint8ToByte(cw, versionNumberAndCurrentNextIndicator); err != nil {
		return
	}

	// sectionNumber(8)
	if err = util.WriteUint8ToByte(cw, sectionNumber); err != nil {
		return
	}

	// lastSectionNumber(8)
	if err = util.WriteUint8ToByte(cw, lastSectionNumber); err != nil {
		return
	}

	// data
	if _, err = cw.Write(data); err != nil {
		return
	}

	// crc32
	crc32 := util.BigLittleSwap(uint(cw.Crc32))
	if err = util.WriteUint32ToByte(cw, uint32(crc32), true); err != nil {
		return
	}

	return
}
