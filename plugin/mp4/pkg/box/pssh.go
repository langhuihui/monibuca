package box

import (
	"bytes"
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
	FullBox
	SystemID [16]byte
	KIDs     [][16]byte
	Data     []byte
}

func CreatePsshBox(systemID [16]byte, kids [][16]byte, data []byte) *PsshBox {
	version := uint8(0)
	size := uint32(FullBoxLen + 16 + 4 + len(data))
	if len(kids) > 0 {
		version = 1
		size += 4 + uint32(len(kids)*16)
	}
	return &PsshBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypePSSH,
				size: size,
			},
			Version: version,
			Flags:   [3]byte{0, 0, 0},
		},
		SystemID: systemID,
		KIDs:     kids,
		Data:     data,
	}
}

func (box *PsshBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [4]byte
	buffers := make([][]byte, 0, 3+len(box.KIDs))
	buffers = append(buffers, box.SystemID[:])

	if box.Version > 0 {
		binary.BigEndian.PutUint32(tmp[:], uint32(len(box.KIDs)))
		buffers = append(buffers, tmp[:])
		for _, kid := range box.KIDs {
			buffers = append(buffers, kid[:])
		}
	}

	binary.BigEndian.PutUint32(tmp[:], uint32(len(box.Data)))
	buffers = append(buffers, tmp[:], box.Data)

	nn, err := io.Copy(w, io.MultiReader(bytes.NewReader(box.SystemID[:]), bytes.NewReader(tmp[:]), bytes.NewReader(box.Data)))
	return int64(nn), err
}

func (box *PsshBox) Unmarshal(buf []byte) (IBox, error) {
	if len(buf) < 20 {
		return nil, io.ErrShortBuffer
	}
	n := 0
	copy(box.SystemID[:], buf[n:n+16])
	n += 16
	if box.Version > 0 {
		if len(buf) < n+4 {
			return nil, io.ErrShortBuffer
		}
		kidCount := binary.BigEndian.Uint32(buf[n:])
		n += 4
		if len(buf) < n+int(kidCount)*16 {
			return nil, io.ErrShortBuffer
		}
		for i := uint32(0); i < kidCount; i++ {
			var kid [16]byte
			copy(kid[:], buf[n:n+16])
			n += 16
			box.KIDs = append(box.KIDs, kid)
		}
	}
	if len(buf) < n+4 {
		return nil, io.ErrShortBuffer
	}
	dataLen := binary.BigEndian.Uint32(buf[n:])
	n += 4
	if len(buf) < n+int(dataLen) {
		return nil, io.ErrShortBuffer
	}
	box.Data = buf[n : n+int(dataLen)]
	return box, nil
}

func (box *PsshBox) IsWidevine() bool {
	return hex.EncodeToString(box.SystemID[:]) == UUIDWidevine
}

func (box *PsshBox) IsPlayReady() bool {
	return hex.EncodeToString(box.SystemID[:]) == UUIDPlayReady
}

func (box *PsshBox) IsFairPlay() bool {
	return hex.EncodeToString(box.SystemID[:]) == UUIDFairPlay
}

func init() {
	RegisterBox[*PsshBox](TypePSSH)
}
