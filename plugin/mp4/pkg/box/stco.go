package box

import (
	"encoding/binary"
	"io"
	"net"
)

// aligned(8) class ChunkOffsetBox
//     extends FullBox('stco', version = 0, 0) {
//         unsigned int(32) entry_count;
//         for (i=1; i <= entry_count; i++) {
//             unsigned int(32) chunk_offset;
//     }
// }
// aligned(8) class ChunkLargeOffsetBox
//     extends FullBox('co64', version = 0, 0) {
//         unsigned int(32) entry_count;
//         for (i=1; i <= entry_count; i++) {
//             unsigned int(64) chunk_offset;
//         }
// }

type STCOBox struct {
	FullBox
	Entries []uint64
}

type CO64Box STCOBox

func CreateSTCOBox(entries []uint64) *STCOBox {
	return &STCOBox{

		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeSTCO,
				size: uint32(FullBoxLen + 4 + len(entries)*4),
			},
			Version: 0,
			Flags:   [3]byte{0, 0, 0},
		},

		Entries: entries,
	}
}

func CreateCO64Box(entries []uint64) *CO64Box {
	return &CO64Box{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeCO64,
				size: uint32(FullBoxLen + 4 + len(entries)*8),
			},
		},

		Entries: entries,
	}
}

func (box *STCOBox) WriteTo(w io.Writer) (n int64, err error) {
	buf := make([]byte, 4+len(box.Entries)*4)

	// Write entry count
	binary.BigEndian.PutUint32(buf[:4], uint32(len(box.Entries)))

	// Write entries
	for i, chunkOffset := range box.Entries {
		binary.BigEndian.PutUint32(buf[4+i*4:], uint32(chunkOffset))
	}

	_, err = w.Write(buf)
	return int64(len(buf)), err
}

func (box *CO64Box) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [8]byte
	buffers := make(net.Buffers, 0, len(box.Entries)+1)

	// Write entry count
	binary.BigEndian.PutUint32(tmp[:], uint32(len(box.Entries)))
	buffers = append(buffers, tmp[:])

	// Write entries
	for _, chunkOffset := range box.Entries {
		binary.BigEndian.PutUint64(tmp[:], chunkOffset)
		buffers = append(buffers, tmp[:])
	}

	return buffers.WriteTo(w)
}

func (box *STCOBox) Unmarshal(buf []byte) (IBox, error) {
	entryCount := binary.BigEndian.Uint32(buf[:4])
	box.Entries = make([]uint64, entryCount)

	if len(buf) < 4+int(entryCount)*4 {
		return nil, io.ErrShortBuffer
	}

	idx := 4
	for i := 0; i < int(entryCount); i++ {
		box.Entries[i] = uint64(binary.BigEndian.Uint32(buf[idx:]))
		idx += 4
	}
	return box, nil
}

func (box *CO64Box) Unmarshal(buf []byte) (IBox, error) {
	entryCount := binary.BigEndian.Uint32(buf[:4])
	box.Entries = make([]uint64, entryCount)

	if len(buf) < 4+int(entryCount)*8 {
		return nil, io.ErrShortBuffer
	}

	idx := 4
	for i := 0; i < int(entryCount); i++ {
		box.Entries[i] = binary.BigEndian.Uint64(buf[idx:])
		idx += 8
	}
	return box, nil
}

func init() {
	RegisterBox[*STCOBox](TypeSTCO)
	RegisterBox[*CO64Box](TypeCO64)
}
