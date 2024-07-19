package box

import (
	"encoding/binary"
	"errors"
	"io"
)

// SaioBox - Sample Auxiliary Information Offsets Box (saiz) (in stbl or traf box)
type SaioBox struct {
	Box                  *FullBox
	AuxInfoType          string // Used for Common Encryption Scheme (4-bytes uint32 according to spec)
	AuxInfoTypeParameter uint32
	Offset               []int64
}

func (s *SaioBox) Decode(r io.Reader, size uint32) error {
	if _, err := s.Box.Decode(r); err != nil {
		return err
	}
	buf := make([]byte, size-12)
	if _, err := io.ReadFull(r, buf); err != nil {
		return err
	}
	var n int
	flags := uint32(s.Box.Flags[0])<<16 | uint32(s.Box.Flags[1])<<8 | uint32(s.Box.Flags[2])
	if flags&0x01 != 0 {
		s.AuxInfoType = string(buf[n : n+4])
		n += 4
		s.AuxInfoTypeParameter = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}
	entryCount := binary.BigEndian.Uint32(buf[n:])
	n += 4
	if s.Box.Version == 0 {
		for i := uint32(0); i < entryCount; i++ {
			s.Offset = append(s.Offset, int64(binary.BigEndian.Uint32(buf[n:])))
			n += 4
		}
	} else {
		for i := uint32(0); i < entryCount; i++ {
			s.Offset = append(s.Offset, int64(binary.BigEndian.Uint64(buf[n:])))
			n += 8
		}
	}
	return nil
}

func decodeSaioBox(demuxer *MovDemuxer, size uint32) error {
	saio := SaioBox{Box: new(FullBox)}
	err := saio.Decode(demuxer.reader, size)
	if err != nil {
		return err
	}
	if demuxer.currentTrack == nil {
		return errors.New("current track is nil")
	}
	if len(saio.Offset) > 0 && len(demuxer.currentTrack.subSamples) == 0 {
		var currentOffset int64
		currentOffset, err = demuxer.reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		demuxer.reader.Seek(demuxer.moofOffset+saio.Offset[0], io.SeekStart)
		saiz := demuxer.currentTrack.lastSaiz
		for i := uint32(0); i < saiz.SampleCount; i++ {
			sampleSize := saiz.DefaultSampleInfoSize
			if saiz.DefaultSampleInfoSize == 0 {
				sampleSize = saiz.SampleInfo[i]
			}
			buf := make([]byte, sampleSize)
			demuxer.reader.Read(buf)
			var se sencEntry
			se.iv = make([]byte, 16)
			copy(se.iv, buf[:8])
			if sampleSize == 8 {
				demuxer.currentTrack.subSamples = append(demuxer.currentTrack.subSamples, se)
				continue
			}
			n := 8
			sampleCount := binary.BigEndian.Uint16(buf[n:])
			n += 2

			se.subSamples = make([]subSampleEntry, sampleCount)
			for j := 0; j < int(sampleCount); j++ {
				se.subSamples[j].bytesOfClearData = binary.BigEndian.Uint16(buf[n:])
				n += 2
				se.subSamples[j].bytesOfProtectedData = binary.BigEndian.Uint32(buf[n:])
				n += 4
			}
			demuxer.currentTrack.subSamples = append(demuxer.currentTrack.subSamples, se)
		}
		demuxer.reader.Seek(currentOffset, io.SeekStart)
	}
	return nil
}
