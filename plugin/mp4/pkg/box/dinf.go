package box

import "encoding/binary"

// aligned(8) class DataEntryUrlBox (bit(24) flags) extends FullBox(‘url ’, version = 0, flags) {
// 	string location;
// }
// aligned(8) class DataEntryUrnBox (bit(24) flags) extends FullBox(‘urn ’, version = 0, flags) {
// 	string name;
// 	string location;
// }
// aligned(8) class DataReferenceBox extends FullBox(‘dref’, version = 0, 0) {
// 	unsigned int(32)  entry_count;
//     for (i=1; i <= entry_count; i++) {
// 		DataEntryBox(entry_version, entry_flags) data_entry;
// 	}
// }

func MakeDefaultDinfBox() []byte {
	dinf := BasicBox{Type: TypeDINF, Size: 36}
	offset, dinfbox := dinf.Encode()
	binary.BigEndian.PutUint32(dinfbox[offset:], 28)
	offset += 4
	copy(dinfbox[offset:], TypeDREF[:])
	offset += 4
	offset += 4
	binary.BigEndian.PutUint32(dinfbox[offset:], 1)
	offset += 4
	binary.BigEndian.PutUint32(dinfbox[offset:], 0xc)
	offset += 4
	copy(dinfbox[offset:], []byte("url "))
	offset += 4
	binary.BigEndian.PutUint32(dinfbox[offset:], 1)
	return dinfbox
}
