package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) class ChunkOffsetBox
//     extends FullBox(‘stco’, version = 0, 0) {
//         unsigned int(32) entry_count;
//         for (i=1; i <= entry_count; i++) {
//             unsigned int(32) chunk_offset;
//     }
// }
// aligned(8) class ChunkLargeOffsetBox
//     extends FullBox(‘co64’, version = 0, 0) {
//         unsigned int(32) entry_count;
//         for (i=1; i <= entry_count; i++) {
//             unsigned int(64) chunk_offset;
//         }
// }

type ChunkOffsetBox []uint64

func (stco *ChunkOffsetBox) Decode(r io.Reader) (offset int, err error) {
	var fullbox FullBox
	if _, err = fullbox.Decode(r); err != nil {
		return
	}
	tmp := make([]byte, 4)
	if _, err = io.ReadFull(r, tmp); err != nil {
		return
	}
	offset = 8
	l := binary.BigEndian.Uint32(tmp)
	*stco = make([]uint64, l)
	buf := make([]byte, l*4)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	idx := 0
	for i := 0; i < int(l); i++ {
		(*stco)[i] = uint64(binary.BigEndian.Uint32(buf[idx:]))
		idx += 4
	}
	offset += idx
	return
}

func (stco ChunkOffsetBox) Encode() (int, []byte) {
	var fullbox *FullBox
	l := len(stco)
	if stco[l-1] > 0xFFFFFFFF {
		fullbox = NewFullBox(TypeCO64, 0)
		fullbox.Box.Size = FullBoxLen + 4 + 8*uint64(l)
	} else {
		fullbox = NewFullBox(TypeSTCO, 0)
		fullbox.Box.Size = FullBoxLen + 4 + 4*uint64(l)
	}
	offset, buf := fullbox.Encode()
	binary.BigEndian.PutUint32(buf[offset:], uint32(l))
	offset += 4
	for i := 0; i < int(l); i++ {
		if fullbox.Box.Type == TypeCO64 {
			binary.BigEndian.PutUint64(buf[offset:], uint64(stco[i]))
			offset += 8
		} else {
			binary.BigEndian.PutUint32(buf[offset:], uint32(stco[i]))
			offset += 4
		}
	}
	return offset, buf
}

type ChunkLargeOffsetBox ChunkOffsetBox

func (co64 *ChunkLargeOffsetBox) Decode(r io.Reader) (offset int, err error) {
	var fullbox FullBox
	if _, err = fullbox.Decode(r); err != nil {
		return
	}
	tmp := make([]byte, 4)
	if _, err = io.ReadFull(r, tmp); err != nil {
		return
	}
	offset = 8
	l := binary.BigEndian.Uint32(tmp)
	*co64 = make([]uint64, l)
	buf := make([]byte, l*8)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	idx := 0
	for i := 0; i < int(l); i++ {
		(*co64)[i] = binary.BigEndian.Uint64(buf[idx:])
		idx += 8
	}
	offset += idx
	return
}
