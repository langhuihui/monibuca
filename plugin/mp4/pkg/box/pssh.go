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
	SystemID [16]byte
	KIDs     [][16]byte
	Data     []byte
}

func (pssh *PsshBox) Decode(r io.Reader, basebox *BasicBox) (offset int, err error) {
	var fullbox FullBox
	if offset, err = fullbox.Decode(r); err != nil {
		return
	}
	buf := make([]byte, basebox.Size-FullBoxLen)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	n := 0
	copy(pssh.SystemID[:], buf[n:n+16])
	n += 16
	if fullbox.Version > 0 {
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
