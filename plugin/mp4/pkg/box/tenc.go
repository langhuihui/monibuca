package box

import (
	"encoding/binary"
	"io"
)

func decodeTencBox(demuxer *MovDemuxer, size uint32) (err error) {
	buf := make([]byte, size-BasicBoxLen)
	if _, err = io.ReadFull(demuxer.reader, buf); err != nil {
		return
	}
	n := 0
	versionAndFlags := binary.BigEndian.Uint32(buf[n:])
	n += 5
	version := byte(versionAndFlags >> 24)
	track := demuxer.tracks[len(demuxer.tracks)-1]
	if version != 0 {
		infoByte := buf[n]
		track.defaultCryptByteBlock = infoByte >> 4
		track.defaultSkipByteBlock = infoByte & 0x0f
	}
	n += 1
	track.defaultIsProtected = buf[n]
	n += 1
	track.defaultPerSampleIVSize = buf[n]
	n += 1
	copy(track.defaultKID[:], buf[n:n+16])
	n += 16
	if track.defaultIsProtected == 1 && track.defaultPerSampleIVSize == 0 {
		defaultConstantIVSize := int(buf[n])
		n += 1
		track.defaultConstantIV = make([]byte, defaultConstantIVSize)
		copy(track.defaultConstantIV, buf[n:n+defaultConstantIVSize])
	}
	return nil
}
