package util

import (
	"io"
)

type GolombBitReader struct {
	R    io.Reader
	buf  [1]byte
	left byte
}

func (r *GolombBitReader) ReadBit() (res uint, err error) {
	if r.left == 0 {
		if _, err = r.R.Read(r.buf[:]); err != nil {
			return
		}
		r.left = 8
	}
	r.left--
	res = uint(r.buf[0]>>r.left) & 1
	return
}

func (r *GolombBitReader) ReadBits(n int) (res uint, err error) {
	for i := 0; i < n; i++ {
		var bit uint
		if bit, err = r.ReadBit(); err != nil {
			return
		}
		res |= bit << uint(n-i-1)
	}
	return
}

func (r *GolombBitReader) ReadExponentialGolombCode() (res uint, err error) {
	i := 0
	for {
		var bit uint
		if bit, err = r.ReadBit(); err != nil {
			return
		}
		if !(bit == 0 && i < 32) {
			break
		}
		i++
	}
	if res, err = r.ReadBits(i); err != nil {
		return
	}
	res += (1 << uint(i)) - 1
	return
}

func (r *GolombBitReader) ReadSE() (res uint, err error) {
	if res, err = r.ReadExponentialGolombCode(); err != nil {
		return
	}
	if res&0x01 != 0 {
		res = (res + 1) / 2
	} else {
		res = -res / 2
	}
	return
}
