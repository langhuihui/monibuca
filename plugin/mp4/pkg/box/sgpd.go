package box

import (
	"fmt"
)

// SeigSampleGroupEntry - CencSampleEncryptionInformationGroupEntry as defined in
// CEF ISO/IEC 23001-7 3rd edition 2016
type SeigSampleGroupEntry struct {
	CryptByteBlock  byte
	SkipByteBlock   byte
	IsProtected     byte
	PerSampleIVSize byte
	KID             [16]byte
	// ConstantIVSize byte given by len(ConstantIV)
	ConstantIV []byte
}

// SgpdBox - Sample Group Description Box, ISO/IEC 14496-12 6'th edition 2020 Section 8.9.3
// Version 0 is deprecated
type SgpdBox struct {
	Version                      byte
	Flags                        uint32
	GroupingType                 string // uint32, but takes values such as seig
	DefaultLength                uint32
	DefaultGroupDescriptionIndex uint32
	DescriptionLengths           []uint32
	SampleGroupEntries           []interface{}
}

type SampleGroupEntryDecoder func(name string, length uint32, buf []byte) (interface{}, int, error)

var sgeDecoders = map[string]SampleGroupEntryDecoder{
	"seig": DecodeSeigSampleGroupEntry,
}

func DecodeSampleGroupEntry(name string, length uint32, buf []byte) (interface{}, int, error) {
	decode, ok := sgeDecoders[name]
	if ok {
		return decode(name, length, buf)
	}
	return nil, 0, nil
}

// DecodeSeigSampleGroupEntry - decode Common Encryption Sample Group Entry
func DecodeSeigSampleGroupEntry(name string, length uint32, buf []byte) (interface{}, int, error) {
	s := &SeigSampleGroupEntry{}
	n := 0
	n += 1 // Reserved
	byteTwo := buf[n]
	n += 1

	s.CryptByteBlock = byteTwo >> 4
	s.SkipByteBlock = byteTwo % 0xf

	s.IsProtected = buf[n]
	n += 1

	s.PerSampleIVSize = buf[n]
	n += 1

	copy(s.KID[:], buf[n:n+16])
	n += 16

	if s.IsProtected == 1 && s.PerSampleIVSize == 0 {
		constantIVSize := int(buf[n])
		n += 1
		s.ConstantIV = buf[n : n+constantIVSize]
		n += constantIVSize
	}
	if length != uint32(s.Size()) {
		return nil, n, fmt.Errorf("seig: given length %d different from calculated size %d", length, s.Size())
	}
	return s, n, nil
}

// Size of SampleGroup Entry
func (s *SeigSampleGroupEntry) Size() uint64 {
	// reserved: 1
	// cryptByteBlock + SkipByteBlock : 1
	// isProtected: 1
	// perSampleIVSize: 1
	// KID: 16
	size := 20
	if s.IsProtected == 1 && s.PerSampleIVSize == 0 {
		size += 1 + len(s.ConstantIV)
	}
	return uint64(size)
}
