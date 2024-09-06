package box

import (
	"encoding/binary"
	"fmt"
	"io"
)

// aligned(8) abstract class SampleEntry (unsigned int(32) format) extends Box(format){
// 	const unsigned int(8)[6] reserved = 0;
// 	unsigned int(16) data_reference_index;
// 	}

type SampleEntry struct {
	box                  *BasicBox
	data_reference_index uint16
}

func NewSampleEntry(format [4]byte) *SampleEntry {
	return &SampleEntry{
		box:                  NewBasicBox(format),
		data_reference_index: 1,
	}
}

func (entry *SampleEntry) Size() uint64 {
	return 8 + 8
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

func (entry *SampleEntry) Encode() (int, []byte) {
	offset, buf := entry.box.Encode()
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
	entry        *SampleEntry
	version      uint16 // ffmpeg mov.c mov_parse_stsd_audio
	channelcount uint16
	samplesize   uint16
	samplerate   uint32
}

func NewAudioSampleEntry(format [4]byte) *AudioSampleEntry {
	return &AudioSampleEntry{
		entry: NewSampleEntry(format),
	}
}

func (entry *AudioSampleEntry) Decode(r io.Reader) (offset int, err error) {
	if _, err = entry.entry.Decode(r); err != nil {
		return
	}
	buf := make([]byte, 20)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	offset = 0
	entry.version = binary.BigEndian.Uint16(buf[offset:])
	offset = 8
	entry.channelcount = binary.BigEndian.Uint16(buf[offset:])
	offset += 2
	entry.samplesize = binary.BigEndian.Uint16(buf[offset:])
	offset += 2
	offset += 4
	entry.samplerate = binary.BigEndian.Uint32(buf[offset:])
	entry.samplerate = entry.samplerate >> 16
	offset += 4
	return
}

func (entry *AudioSampleEntry) Size() uint64 {
	if entry.entry.box.Size > 0 {
		return entry.entry.box.Size
	} else {
		return entry.entry.Size() + 20
	}
}

func (entry *AudioSampleEntry) Encode() (int, []byte) {
	entry.entry.box.Size = entry.Size()
	offset, buf := entry.entry.Encode()
	offset += 8
	binary.BigEndian.PutUint16(buf[offset:], entry.channelcount)
	offset += 2
	binary.BigEndian.PutUint16(buf[offset:], entry.samplesize)
	offset += 2
	offset += 4
	binary.BigEndian.PutUint32(buf[offset:], entry.samplerate<<16)
	offset += 4
	return offset, buf
}

func decodeAudioSampleEntry(demuxer *MovDemuxer) (err error) {
	entry := AudioSampleEntry{entry: new(SampleEntry)}
	_, err = entry.Decode(demuxer.reader)
	track := demuxer.tracks[len(demuxer.tracks)-1]
	track.chanelCount = uint8(entry.channelcount)
	track.sampleBits = uint8(entry.samplesize)
	track.sampleRate = entry.samplerate
	quickTime := false
	for _, brand := range demuxer.mp4Info.CompatibleBrands {
		if brand == mov_tag([4]byte{'q', 't', ' ', ' '}) {
			quickTime = true
			break
		}
	}
	//ffmpeg mov.c mov_parse_stsd_audio
	if quickTime && entry.version == 1 {
		if _, err = io.ReadFull(demuxer.reader, make([]byte, 16)); err != nil {
			return
		}
	} else if quickTime && entry.version == 2 {
		if _, err = io.ReadFull(demuxer.reader, make([]byte, 36)); err != nil {
			return
		}
	}
	return
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
	entry           *SampleEntry
	width           uint16
	height          uint16
	horizresolution uint32
	vertresolution  uint32
	frame_count     uint16
	compressorname  [32]byte
}

func NewVisualSampleEntry(format [4]byte) *VisualSampleEntry {
	return &VisualSampleEntry{
		entry:           NewSampleEntry(format),
		horizresolution: 0x00480000,
		vertresolution:  0x00480000,
		frame_count:     1,
	}
}

func (entry *VisualSampleEntry) Size() uint64 {
	return entry.entry.Size() + 70
}

func (entry *VisualSampleEntry) Decode(r io.Reader) (offset int, err error) {
	if _, err = entry.entry.Decode(r); err != nil {
		return 0, err
	}
	buf := make([]byte, 70)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	offset = 16
	entry.width = binary.BigEndian.Uint16(buf[offset:])
	offset += 2
	entry.height = binary.BigEndian.Uint16(buf[offset:])
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

func (entry *VisualSampleEntry) Encode() (int, []byte) {
	offset, buf := entry.entry.Encode()
	offset += 16
	binary.BigEndian.PutUint16(buf[offset:], entry.width)
	offset += 2
	binary.BigEndian.PutUint16(buf[offset:], entry.height)
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

func decodeVisualSampleEntry(demuxer *MovDemuxer) (err error) {
	entry := VisualSampleEntry{entry: new(SampleEntry)}
	_, err = entry.Decode(demuxer.reader)
	track := demuxer.tracks[len(demuxer.tracks)-1]
	track.width = uint32(entry.width)
	track.height = uint32(entry.height)
	return
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

type SampleDescriptionBox struct {
	box         *FullBox
	entry_count uint32
}

func NewSampleDescriptionBox() *SampleDescriptionBox {
	return &SampleDescriptionBox{
		box: NewFullBox([4]byte{'s', 't', 's', 'd'}, 0),
	}
}

func (stsd *SampleDescriptionBox) Size() uint64 {
	return stsd.box.Size() + 4
}

func (stsd *SampleDescriptionBox) Decode(r io.Reader) (offset int, err error) {
	if _, err = stsd.box.Decode(r); err != nil {
		return
	}
	buf := make([]byte, 4)
	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}
	stsd.entry_count = binary.BigEndian.Uint32(buf)
	return
}

func (entry *SampleDescriptionBox) Encode() (int, []byte) {
	offset, buf := entry.box.Encode()
	binary.BigEndian.PutUint32(buf[offset:], entry.entry_count)
	offset += 4
	return offset, buf
}

func makeStsd(track *mp4track, handler_type HandlerType) []byte {

	var avbox []byte
	var extraData []byte
	if len(track.extraData) == 0 {
		if track.cid == MP4_CODEC_AAC || track.cid == MP4_CODEC_H264 || track.cid == MP4_CODEC_H265 {
			if track.extra == nil {
				panic(fmt.Sprintf("track %d:extra is nil", track.trackId))
			}
			extraData = track.extra.export()
		}
	} else {
		extraData = track.extraData
	}

	if track.cid == MP4_CODEC_H264 {
		avbox = makeAvcCBox(extraData)
	} else if track.cid == MP4_CODEC_H265 {
		avbox = makeHvcCBox(extraData)
	} else if track.cid == MP4_CODEC_AAC || track.cid == MP4_CODEC_MP2 || track.cid == MP4_CODEC_MP3 {
		avbox = makeEsdsBox(track.trackId, track.cid, extraData)
	} else if track.cid == MP4_CODEC_OPUS {
		avbox = makeOpusSpecificBox(extraData)
	}

	var se []byte
	var offset int
	if handler_type.equal(vide) {
		entry := NewVisualSampleEntry(getCodecNameWithCodecId(track.cid))
		entry.width = uint16(track.width)
		entry.height = uint16(track.height)
		entry.entry.box.Size = entry.Size() + uint64(len(avbox))
		offset, se = entry.Encode()
	} else if handler_type.equal(soun) {
		entry := NewAudioSampleEntry(getCodecNameWithCodecId(track.cid))
		entry.channelcount = uint16(track.chanelCount)
		entry.samplerate = track.sampleRate
		entry.samplesize = uint16(track.sampleBits)
		entry.entry.box.Size = entry.Size() + uint64(len(avbox))
		offset, se = entry.Encode()
	}
	copy(se[offset:], avbox)

	stsd := NewSampleDescriptionBox()
	stsd.box.Box.Size = stsd.Size() + uint64(len(se))
	stsd.entry_count = 1
	offset2, stsdbox := stsd.Encode()
	copy(stsdbox[offset2:], se)
	return stsdbox
}

func decodeStsdBox(demuxer *MovDemuxer) (err error) {
	stsd := SampleDescriptionBox{box: new(FullBox)}
	_, err = stsd.Decode(demuxer.reader)
	return
}

func makeAvcCBox(extraData []byte) []byte {
	avcc := BasicBox{Type: [4]byte{'a', 'v', 'c', 'C'}}
	avcc.Size = 8 + uint64(len(extraData))
	offset, boxdata := avcc.Encode()
	copy(boxdata[offset:], extraData)
	return boxdata
}

func decodeAvccBox(demuxer *MovDemuxer, size uint32) (err error) {
	buf := make([]byte, size-BasicBoxLen)
	if _, err = io.ReadFull(demuxer.reader, buf); err != nil {
		return
	}
	track := demuxer.tracks[len(demuxer.tracks)-1]
	if track.extra == nil {
		track.extra = new(h264ExtraData)
	}
	track.extra.load(buf)
	return
}

func makeHvcCBox(extraData []byte) []byte {
	hvcc := BasicBox{Type: [4]byte{'h', 'v', 'c', 'C'}}
	hvcc.Size = 8 + uint64(len(extraData))
	offset, boxdata := hvcc.Encode()
	copy(boxdata[offset:], extraData)
	return boxdata
}

func decodeHvccBox(demuxer *MovDemuxer, size uint32) (err error) {
	buf := make([]byte, size-BasicBoxLen)
	if _, err = io.ReadFull(demuxer.reader, buf); err != nil {
		return
	}
	track := demuxer.tracks[len(demuxer.tracks)-1]
	track.extra.load(buf)
	return
}

func makeEsdsBox(tid uint32, cid MP4_CODEC_TYPE, extraData []byte) []byte {
	esd := makeESDescriptor(uint16(tid), cid, extraData)
	esds := FullBox{Box: NewBasicBox([4]byte{'e', 's', 'd', 's'}), Version: 0}
	esds.Box.Size = esds.Size() + uint64(len(esd))
	offset, esdsBox := esds.Encode()
	copy(esdsBox[offset:], esd)
	return esdsBox
}

func decodeEsdsBox(demuxer *MovDemuxer, size uint32) (err error) {
	esds := FullBox{}
	if _, err = esds.Decode(demuxer.reader); err != nil {
		return
	}
	buf := make([]byte, size-FullBoxLen)
	if _, err = io.ReadFull(demuxer.reader, buf); err != nil {
		return
	}
	track := demuxer.tracks[len(demuxer.tracks)-1]
	vosdata := decodeESDescriptor(buf, track)
	if track.extra != nil {
		track.extra.load(vosdata)
	}
	return nil
}

//ffmpeg mov_write_wave_tag
//  avio_wb32(pb, 12);    /* size */
//  ffio_wfourcc(pb, "frma");
//  avio_wl32(pb, track->tag);

// avio_wb32(pb, 12); /* size */
// ffio_wfourcc(pb, "mp4a");
// avio_wb32(pb, 0);
func decodeWaveBox(demuxer *MovDemuxer) (err error) {
	if _, err = io.ReadFull(demuxer.reader, make([]byte, 24)); err != nil {
		return
	}
	return
}
