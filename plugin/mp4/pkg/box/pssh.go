package box

import (
	"encoding/binary"
	"encoding/hex"
	"io"
)

// UUIDs for different DRM systems
const (
	UUIDPlayReady = "9a04f07998404286ab92e65be0885f95"
	UUIDWidevine  = "edef8ba979d64acea3c827dcd51d21ed"
	UUIDFairPlay  = "94CE86FB07FF4F43ADB893D2FA968CA2"
)

// PsshBox - Protection System Specific Header Box
// Defined in ISO/IEC 23001-7 Section 8.1
type PsshBox struct {
	Box      *FullBox
	SystemID [16]byte
	KIDs     [][16]byte
	Data     []byte
}

func (pssh *PsshBox) Decode(r io.Reader, size uint32) (offset int, err error) {
	if offset, err = pssh.Box.Decode(r); err != nil {
		return
	}
	buf := make([]byte, size-12)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	n := 0
	copy(pssh.SystemID[:], buf[n:n+16])
	n += 16
	if pssh.Box.Version > 0 {
		kidCount := binary.BigEndian.Uint32(buf[n:])
		n += 4
		for i := uint32(0); i < kidCount; i++ {
			var kid [16]byte
			copy(kid[:], buf[n:n+16])
			n += 16
			pssh.KIDs = append(pssh.KIDs, kid)
		}
	}
	dataLen := binary.BigEndian.Uint32(buf[n:])
	n += 4
	pssh.Data = buf[n : n+int(dataLen)]
	return
}

func (pssh *PsshBox) IsWidevine() bool {
	return hex.EncodeToString(pssh.SystemID[:]) == UUIDWidevine
}

func (pssh *PsshBox) IsPlayReady() bool {
	return hex.EncodeToString(pssh.SystemID[:]) == UUIDPlayReady
}

func (pssh *PsshBox) IsFairPlay() bool {
	return hex.EncodeToString(pssh.SystemID[:]) == UUIDFairPlay
}

func decodePsshBox(demuxer *MovDemuxer, size uint32) (err error) {
	pssh := PsshBox{Box: new(FullBox)}
	if _, err = pssh.Decode(demuxer.reader, size); err != nil {
		return err
	}
	demuxer.pssh = append(demuxer.pssh, pssh)
	return
}
