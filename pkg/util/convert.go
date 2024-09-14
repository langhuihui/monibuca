package util

import (
	"encoding/binary"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
)

func TimeQueryParse(query string) (t time.Time, err error) {
	if !strings.Contains(query, "T") {
		query = time.Now().Format("2006-01-02") + "T" + query
	}
	t, err = time.ParseInLocation("2006-01-02T15:04:05", query, time.Local)
	return
}

func TimeQueryParseRefer(query string, refer time.Time) (t time.Time, err error) {
	if !strings.Contains(query, "T") {
		query = refer.Format("2006-01-02") + "T" + query
	}
	t, err = time.ParseInLocation("2006-01-02T15:04:05", query, time.Local)
	return
}

/*
func ReadByteToUintX(r io.Reader, l int) (data uint64, err error) {
	if l%8 != 0 || l > 64 {
		return 0, errors.New("disable convert")
	}

	bb := make([]byte, l)
	if _, err := io.ReadFull(r, bb); err != nil {
		return 0, err
	}

	switch l / 8 {
	case 1:
		{
			return uint8(bb[0]), nil
		}
	case 2:
		{
			return BigEndian.Uint16(bb), nil
		}
	case 3:
		{
			return BigEndian.Uint24(bb), nil
		}
	case 4:
		{
			return BigEndian.Uint32(bb), nil
		}
	case 5:
		{
			//return BigEndian.Uint40(bb), nil
			return 0, errors.New("disable convert")
		}
	case 6:
		{
			return BigEndian.Uint48(bb), nil
		}
	case 7:
		{
			//return BigEndian.Uint56(bb), nil
			return 0, errors.New("disable convert")
		}
	case 8:
		{
			return BigEndian.Uint64(bb), nil
		}
	}

	return 0, errors.New("convert not exist")
}
*/

// // 千万注意大小端,RTMP是大端
func ByteToUint32N(data []byte) (ret uint32, err error) {
	if len(data) > 4 {
		return 0, errors.New("ByteToUint32N error!")
	}

	for i := 0; i < len(data); i++ {
		ret <<= 8
		ret |= uint32(data[i])
	}

	return
}

// // 千万注意大小端,RTMP是大端
func ByteToUint64N(data []byte) (ret uint64, err error) {
	if len(data) > 8 {
		return 0, errors.New("ByteToUint64N error!")
	}

	for i := 0; i < len(data); i++ {
		ret <<= 8
		ret |= uint64(data[i])
	}

	return
}

// 千万注意大小端,RTMP是大端
func ByteToUint32(data []byte, bigEndian bool) (ret uint32, err error) {
	if bigEndian {
		return binary.BigEndian.Uint32(data), nil
	} else {
		return binary.LittleEndian.Uint32(data), nil
	}
}

func Uint32ToByte(data uint32, bigEndian bool) (ret []byte, err error) {
	if bigEndian {
		return BigEndian.ToUint32(data), nil
	} else {
		return LittleEndian.ToUint32(data), nil
	}
}

func ReadByteToUint8(r io.Reader) (data uint8, err error) {
	bb := make([]byte, 1)
	if _, err := io.ReadFull(r, bb); err != nil {
		return 0, err
	}

	return uint8(bb[0]), nil
}

func ReadByteToUint16(r io.Reader, bigEndian bool) (data uint16, err error) {
	bb := make([]byte, 2)
	if _, err := io.ReadFull(r, bb); err != nil {
		return 0, err
	}

	if bigEndian {
		return binary.BigEndian.Uint16(bb), nil
	} else {
		return binary.LittleEndian.Uint16(bb), nil
	}
}

func ReadByteToUint24(r io.Reader, bigEndian bool) (data uint32, err error) {
	bb := make([]byte, 3)
	if _, err := io.ReadFull(r, bb); err != nil {
		return 0, err
	}

	if bigEndian {
		return BigEndian.Uint24(bb), nil
	} else {
		return LittleEndian.Uint24(bb), nil
	}
}

func ReadByteToUint32(r io.Reader, bigEndian bool) (data uint32, err error) {
	bb := make([]byte, 4)
	if _, err := io.ReadFull(r, bb); err != nil {
		return 0, err
	}

	if bigEndian {
		return binary.BigEndian.Uint32(bb), nil
	} else {
		return binary.LittleEndian.Uint32(bb), nil
	}
}

func ReadByteToUint40(r io.Reader, bigEndian bool) (data uint64, err error) {
	bb := make([]byte, 5)
	if _, err := io.ReadFull(r, bb); err != nil {
		return 0, err
	}

	if bigEndian {
		return BigEndian.Uint40(bb), nil
	} else {
		return LittleEndian.Uint40(bb), nil
	}
}

func ReadByteToUint48(r io.Reader, bigEndian bool) (data uint64, err error) {
	bb := make([]byte, 6)
	if _, err := io.ReadFull(r, bb); err != nil {
		return 0, err
	}

	if bigEndian {
		return BigEndian.Uint48(bb), nil
	} else {
		return LittleEndian.Uint48(bb), nil
	}
}

/*
	func ReadByteToUint56(r io.Reader) (data uint64, err error) {
		bb := make([]byte, 7)
		if _, err := io.ReadFull(r, bb); err != nil {
			return 0, err
		}

		return uint8(bb[0]), nil
	}
*/
func BigLittleSwap(v uint) uint {
	return (v >> 24) | ((v>>16)&0xff)<<8 | ((v>>8)&0xff)<<16 | (v&0xff)<<24
}

func ReadByteToUint64(r io.Reader, bigEndian bool) (data uint64, err error) {
	bb := make([]byte, 8)
	if _, err := io.ReadFull(r, bb); err != nil {
		return 0, err
	}

	if bigEndian {
		return binary.BigEndian.Uint64(bb), nil
	} else {
		return binary.LittleEndian.Uint64(bb), nil
	}
}

func WriteUint8ToByte(w io.Writer, data uint8) error {
	bb := make([]byte, 8)
	bb[0] = byte(data)
	_, err := w.Write(bb[:1])
	if err != nil {
		return err
	}

	return nil
}

func WriteUint16ToByte(w io.Writer, data uint16, bigEndian bool) error {
	var bb []byte
	if bigEndian {
		bb = BigEndian.ToUint16(data)
	} else {
		bb = LittleEndian.ToUint16(data)
	}

	_, err := w.Write(bb)
	if err != nil {
		return err
	}

	return nil
}

func WriteUint24ToByte(w io.Writer, data uint32, bigEndian bool) error {
	var bb []byte
	if bigEndian {
		bb = BigEndian.ToUint24(data)
	} else {
		bb = LittleEndian.ToUint24(data)
	}

	_, err := w.Write(bb)
	if err != nil {
		return err
	}

	return nil
}

func WriteUint32ToByte(w io.Writer, data uint32, bigEndian bool) error {
	var bb []byte
	if bigEndian {
		bb = BigEndian.ToUint32(data)
	} else {
		bb = LittleEndian.ToUint32(data)
	}

	_, err := w.Write(bb)
	if err != nil {
		return err
	}

	return nil
}

func WriteUint40ToByte(w io.Writer, data uint64, bigEndian bool) error {
	var bb []byte
	if bigEndian {
		bb = BigEndian.ToUint40(data)
	} else {
		bb = LittleEndian.ToUint40(data)
	}

	_, err := w.Write(bb)
	if err != nil {
		return err
	}

	return nil
}

func WriteUint48ToByte(w io.Writer, data uint64, bigEndian bool) error {
	var bb []byte
	if bigEndian {
		bb = BigEndian.ToUint48(data)
	} else {
		bb = LittleEndian.ToUint48(data)
	}

	_, err := w.Write(bb)
	if err != nil {
		return err
	}

	return nil
}

func WriteUint64ToByte(w io.Writer, data uint64, bigEndian bool) error {
	var bb []byte
	if bigEndian {
		bb = BigEndian.ToUint64(data)
	} else {
		bb = LittleEndian.ToUint64(data)
	}

	_, err := w.Write(bb)
	if err != nil {
		return err
	}

	return nil
}

func GetPtsDts(v uint64) uint64 {
	// 4 + 3 + 1 + 15 + 1 + 15 + 1
	// 0011
	// 0010 + PTS[30-32] + marker_bit + PTS[29-15] + marker_bit + PTS[14-0] + marker_bit
	pts1 := ((v >> 33) & 0x7) << 30
	pts2 := ((v >> 17) & 0x7fff) << 15
	pts3 := ((v >> 1) & 0x7fff)

	return pts1 | pts2 | pts3
}

func PutPtsDts(v uint64) uint64 {
	// 4 + 3 + 1 + 15 + 1 + 15 + 1
	// 0011
	// 0010 + PTS[30-32] + marker_bit + PTS[29-15] + marker_bit + PTS[14-0] + marker_bit
	// 0x100010001
	// 0001 0000 0000 0000 0001 0000 0000 0000 0001
	// 3个 market_it
	pts1 := (v >> 30) & 0x7 << 33
	pts2 := (v >> 15) & 0x7fff << 17
	pts3 := (v & 0x7fff) << 1

	return pts1 | pts2 | pts3 | 0x100010001
}

func GetPCR(v uint64) uint64 {
	// program_clock_reference_base(33) + Reserved(6) + program_clock_reference_extension(9)
	base := v >> 15
	ext := v & 0x1ff
	return base*300 + ext
}

func PutPCR(pcr uint64) uint64 {
	base := pcr / 300
	ext := pcr % 300
	return base<<15 | 0x3f<<9 | ext
}

func GetFillBytes(data byte, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = data
	}

	return b
}
func ToFloat64(num interface{}) float64 {
	switch v := num.(type) {
	case uint:
		return float64(v)
	case int:
		return float64(v)
	case uint8:
		return float64(v)
	case uint16:
		return float64(v)
	case uint32:
		return float64(v)
	case uint64:
		return float64(v)
	case int8:
		return float64(v)
	case int16:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case float64:
		return v
	case float32:
		return float64(v)
	}
	return 0
}

func Conf2Listener(conf string) (protocol string, ports []uint16) {
	var port string
	protocol, port, _ = strings.Cut(conf, ":")
	if r := strings.Split(port, "-"); len(r) == 2 {
		min, err := strconv.Atoi(r[0])
		if err != nil {
			return
		}
		max, err := strconv.Atoi(r[1])
		if err != nil {
			return
		}
		if min < max {
			ports = append(ports, uint16(min), uint16(max))
		}
	} else if p, err := strconv.Atoi(port); err == nil {
		ports = append(ports, uint16(p))
	}
	return
}

type littleEndian struct{}

var LittleEndian littleEndian

func (littleEndian) Uint16(b []byte) uint16 { return uint16(b[0]) | uint16(b[1])<<8 }
func (littleEndian) Uint24(b []byte) uint32 { return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 }
func (littleEndian) Uint32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}
func (littleEndian) Uint40(b []byte) uint64 {
	return uint64(b[0]) | uint64(b[1])<<8 |
		uint64(b[2])<<16 | uint64(b[3])<<24 | uint64(b[4])<<32
}
func (littleEndian) Uint48(b []byte) uint64 {
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 |
		uint64(b[3])<<24 | uint64(b[4])<<32 | uint64(b[5])<<40
}
func (littleEndian) Uint64(b []byte) uint64 {
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}

func (littleEndian) ToUint16(v uint16) []byte {
	b := make([]byte, 2)
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	return b
}
func (littleEndian) ToUint24(v uint32) []byte {
	b := make([]byte, 3)
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	return b
}
func (littleEndian) ToUint32(v uint32) []byte {
	b := make([]byte, 4)
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
	return b
}
func (littleEndian) ToUint40(v uint64) []byte {
	b := make([]byte, 5)
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
	b[4] = byte(v >> 32)
	return b
}
func (littleEndian) ToUint48(v uint64) []byte {
	b := make([]byte, 6)
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
	b[4] = byte(v >> 32)
	b[5] = byte(v >> 40)
	return b
}
func (littleEndian) ToUint64(v uint64) []byte {
	b := make([]byte, 8)
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
	b[4] = byte(v >> 32)
	b[5] = byte(v >> 40)
	b[6] = byte(v >> 48)
	b[7] = byte(v >> 56)
	return b
}

type bigEndian struct{}

var BigEndian bigEndian

func (bigEndian) Uint16(b []byte) uint16 { return uint16(b[1]) | uint16(b[0])<<8 }
func (bigEndian) Uint24(b []byte) uint32 { return uint32(b[2]) | uint32(b[1])<<8 | uint32(b[0])<<16 }
func (bigEndian) Uint32(b []byte) uint32 {
	return uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24
}
func (bigEndian) Uint40(b []byte) uint64 {
	return uint64(b[4]) | uint64(b[3])<<8 |
		uint64(b[2])<<16 | uint64(b[1])<<24 | uint64(b[0])<<32
}
func (bigEndian) Uint48(b []byte) uint64 {
	return uint64(b[5]) | uint64(b[4])<<8 | uint64(b[3])<<16 |
		uint64(b[2])<<24 | uint64(b[1])<<32 | uint64(b[0])<<40
}
func (bigEndian) Uint64(b []byte) uint64 {
	return uint64(b[7]) | uint64(b[6])<<8 | uint64(b[5])<<16 | uint64(b[4])<<24 |
		uint64(b[3])<<32 | uint64(b[2])<<40 | uint64(b[1])<<48 | uint64(b[0])<<56
}
func (bigEndian) ToUint16(v uint16) []byte {
	b := make([]byte, 2)
	b[0] = byte(v >> 8)
	b[1] = byte(v)
	return b
}
func (bigEndian) ToUint24(v uint32) []byte {
	b := make([]byte, 3)
	b[0] = byte(v >> 16)
	b[1] = byte(v >> 8)
	b[2] = byte(v)
	return b
}
func (bigEndian) ToUint32(v uint32) []byte {
	b := make([]byte, 4)
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
	return b
}
func (bigEndian) ToUint40(v uint64) []byte {
	b := make([]byte, 5)
	b[0] = byte(v >> 32)
	b[1] = byte(v >> 24)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 8)
	b[4] = byte(v)
	return b
}
func (bigEndian) ToUint48(v uint64) []byte {
	b := make([]byte, 6)
	b[0] = byte(v >> 40)
	b[1] = byte(v >> 32)
	b[2] = byte(v >> 24)
	b[3] = byte(v >> 16)
	b[4] = byte(v >> 8)
	b[5] = byte(v)
	return b
}
func (bigEndian) ToUint64(v uint64) []byte {
	b := make([]byte, 8)
	b[0] = byte(v >> 56)
	b[1] = byte(v >> 48)
	b[2] = byte(v >> 40)
	b[3] = byte(v >> 32)
	b[4] = byte(v >> 24)
	b[5] = byte(v >> 16)
	b[6] = byte(v >> 8)
	b[7] = byte(v)
	return b
}
