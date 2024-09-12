package box

import (
	"encoding/binary"
	"io"
)

// aligned(8) abstract class SampleEntry (unsigned int(32) format) extends Box(format){
// 	const unsigned int(8)[6] reserved = 0;
// 	unsigned int(16) data_reference_index;
// 	}

type SampleEntry struct {
	Type                 [4]byte
	data_reference_index uint16
}

func NewSampleEntry(format [4]byte) *SampleEntry {
	return &SampleEntry{
		Type:                 format,
		data_reference_index: 1,
	}
}

func (entry *SampleEntry) Size() uint64 {
	return BasicBoxLen + 8
}

func (entry *SampleEntry) Decode(r io.Reader) (offset int, err error) {

	buf := make([]byte, 8)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	offset = 6
	entry.data_reference_index = binary.BigEndian.Uint16(buf[offset:])
	offset += 2
	return
}

func (entry *SampleEntry) Encode(size uint64) (int, []byte) {
	offset, buf := (&BasicBox{Type: entry.Type, Size: size}).Encode()
	offset += 6
	binary.BigEndian.PutUint16(buf[offset:], entry.data_reference_index)
	offset += 2
	return offset, buf
}

// class HintSampleEntry() extends SampleEntry (protocol) {
// 		unsigned int(8) data [];
// }

type HintSampleEntry struct {
	Entry *SampleEntry
	Data  byte
}

// class AudioSampleEntry(codingname) extends SampleEntry (codingname){
//  const unsigned int(32)[2] reserved = 0;
// 	template unsigned int(16) channelcount = 2;
// 	template unsigned int(16) samplesize = 16;
// 	unsigned int(16) pre_defined = 0;
// 	const unsigned int(16) reserved = 0 ;
// 	template unsigned int(32) samplerate = { default samplerate of media}<<16;
// }

type AudioSampleEntry struct {
	*SampleEntry
	Version      uint16 // ffmpeg mov.c mov_parse_stsd_audio
	ChannelCount uint16
	SampleSize   uint16
	Samplerate   uint32
}

func NewAudioSampleEntry(format [4]byte) *AudioSampleEntry {
	return &AudioSampleEntry{
		SampleEntry: NewSampleEntry(format),
	}
}

func (entry *AudioSampleEntry) Decode(r io.Reader) (offset int, err error) {
	if _, err = entry.SampleEntry.Decode(r); err != nil {
		return
	}
	buf := make([]byte, 20)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	offset = 0
	entry.Version = binary.BigEndian.Uint16(buf[offset:])
	offset = 8
	entry.ChannelCount = binary.BigEndian.Uint16(buf[offset:])
	offset += 2
	entry.SampleSize = binary.BigEndian.Uint16(buf[offset:])
	offset += 2
	offset += 4
	entry.Samplerate = binary.BigEndian.Uint32(buf[offset:])
	entry.Samplerate = entry.Samplerate >> 16
	offset += 4
	return
}

func (entry *AudioSampleEntry) Size() uint64 {
	return entry.SampleEntry.Size() + 20
}

func (entry *AudioSampleEntry) Encode(size uint64) (int, []byte) {
	offset, buf := entry.SampleEntry.Encode(size)
	offset += 8
	binary.BigEndian.PutUint16(buf[offset:], entry.ChannelCount)
	offset += 2
	binary.BigEndian.PutUint16(buf[offset:], entry.SampleSize)
	offset += 2
	offset += 4
	binary.BigEndian.PutUint32(buf[offset:], entry.Samplerate<<16)
	offset += 4
	return offset, buf
}

// class VisualSampleEntry(codingname) extends SampleEntry (codingname){
//  unsigned int(16) pre_defined = 0;
// 	const unsigned int(16) reserved = 0;
// 	unsigned int(32)[3] pre_defined = 0;
// 	unsigned int(16) width;
// 	unsigned int(16) height;
// 	template unsigned int(32) horizresolution = 0x00480000; // 72 dpi
//  template unsigned int(32) vertresolution = 0x00480000; // 72 dpi
//  const unsigned int(32) reserved = 0;
// 	template unsigned int(16) frame_count = 1;
// 	string[32] compressorname;
// 	template unsigned int(16) depth = 0x0018;
// 	int(16) pre_defined = -1;
// 	// other boxes from derived specifications
// 	CleanApertureBox clap; // optional
// 	PixelAspectRatioBox pasp; // optional
// }

type VisualSampleEntry struct {
	*SampleEntry
	Width, Height                   uint16
	horizresolution, vertresolution uint32
	frame_count                     uint16
	compressorname                  [32]byte
}

func NewVisualSampleEntry(format [4]byte) *VisualSampleEntry {
	return &VisualSampleEntry{
		SampleEntry:     NewSampleEntry(format),
		horizresolution: 0x00480000,
		vertresolution:  0x00480000,
		frame_count:     1,
	}
}

func (entry *VisualSampleEntry) Size() uint64 {
	return entry.SampleEntry.Size() + 70
}

func (entry *VisualSampleEntry) Decode(r io.Reader) (offset int, err error) {
	if _, err = entry.SampleEntry.Decode(r); err != nil {
		return 0, err
	}
	buf := make([]byte, 70)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	offset = 16
	entry.Width = binary.BigEndian.Uint16(buf[offset:])
	offset += 2
	entry.Height = binary.BigEndian.Uint16(buf[offset:])
	offset += 2
	entry.horizresolution = binary.BigEndian.Uint32(buf[offset:])
	offset += 4
	entry.vertresolution = binary.BigEndian.Uint32(buf[offset:])
	offset += 8
	entry.frame_count = binary.BigEndian.Uint16(buf[offset:])
	offset += 2
	copy(entry.compressorname[:], buf[offset:offset+32])
	offset += 32
	offset += 4
	return
}

func (entry *VisualSampleEntry) Encode(size uint64) (int, []byte) {
	offset, buf := entry.SampleEntry.Encode(size)
	offset += 16
	binary.BigEndian.PutUint16(buf[offset:], entry.Width)
	offset += 2
	binary.BigEndian.PutUint16(buf[offset:], entry.Height)
	offset += 2
	binary.BigEndian.PutUint32(buf[offset:], entry.horizresolution)
	offset += 4
	binary.BigEndian.PutUint32(buf[offset:], entry.vertresolution)
	offset += 8
	binary.BigEndian.PutUint16(buf[offset:], entry.frame_count)
	offset += 2
	copy(buf[offset:offset+32], entry.compressorname[:])
	offset += 32
	binary.BigEndian.PutUint16(buf[offset:], 0x0018)
	offset += 2
	binary.BigEndian.PutUint16(buf[offset:], 0xFFFF)
	offset += 2
	return offset, buf
}

// aligned(8) class SampleDescriptionBox (unsigned int(32) handler_type) extends FullBox('stsd', 0, 0){
// 	int i ;
// 	unsigned int(32) entry_count;
// 	   for (i = 1 ; i <= entry_count ; i++){
// 		  switch (handler_type){
// 			 case ‘soun’: // for audio tracks
// 				AudioSampleEntry();
// 				break;
// 			 case ‘vide’: // for video tracks
// 				VisualSampleEntry();
// 				break;
// 			 case ‘hint’: // Hint track
// 				HintSampleEntry();
// 				break;
// 			 case ‘meta’: // Metadata track
// 				MetadataSampleEntry();
// 				break;
// 		}
// 	}
// }

type SampleEntryType uint8

const (
	SAMPLE_AUDIO SampleEntryType = iota
	SAMPLE_VIDEO
)

type SampleDescriptionBox uint32

func (stsd *SampleDescriptionBox) Decode(r io.Reader) (offset int, err error) {
	var fullbox FullBox
	if offset, err = fullbox.Decode(r); err != nil {
		return
	}
	buf := make([]byte, 4)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	offset += 4
	*stsd = SampleDescriptionBox(binary.BigEndian.Uint32(buf))
	return
}

func (entry SampleDescriptionBox) Encode(size uint64) (int, []byte) {
	fullbox := FullBox{Box: NewBasicBox(TypeSTSD)}
	fullbox.Box.Size = size
	offset, buf := fullbox.Encode()
	binary.BigEndian.PutUint32(buf[offset:], uint32(entry))
	offset += 4
	return offset, buf
}

func MakeAvcCBox(extraData []byte) []byte {
	offset, boxdata := (&BasicBox{Type: TypeAVCC, Size: BasicBoxLen + uint64(len(extraData))}).Encode()
	copy(boxdata[offset:], extraData)
	return boxdata
}

func MakeHvcCBox(extraData []byte) []byte {
	offset, boxdata := (&BasicBox{Type: TypeHVCC, Size: BasicBoxLen + uint64(len(extraData))}).Encode()
	copy(boxdata[offset:], extraData)
	return boxdata
}

func MakeEsdsBox(tid uint32, cid MP4_CODEC_TYPE, extraData []byte) []byte {
	esd := makeESDescriptor(uint16(tid), cid, extraData)
	esds := FullBox{Box: NewBasicBox(TypeESDS), Version: 0}
	esds.Box.Size = esds.Size() + uint64(len(esd))
	offset, esdsBox := esds.Encode()
	copy(esdsBox[offset:], esd)
	return esdsBox
}

//ffmpeg mov_write_wave_tag
//  avio_wb32(pb, 12);    /* size */
//  ffio_wfourcc(pb, "frma");
//  avio_wl32(pb, track->tag);

// avio_wb32(pb, 12); /* size */
// ffio_wfourcc(pb, "mp4a");
// avio_wb32(pb, 0);
