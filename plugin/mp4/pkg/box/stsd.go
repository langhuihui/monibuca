package box

import (
	"bytes"
	"encoding/binary"
	"io"
)

// aligned(8) abstract class SampleEntry (unsigned int(32) format) extends Box(format){
// 	const unsigned int(8)[6] reserved = 0;
// 	unsigned int(16) data_reference_index;
// 	}

type SampleEntry struct {
	BaseBox
	DataReferenceIndex uint16
}

// class HintSampleEntry() extends SampleEntry (protocol) {
// 		unsigned int(8) data [];
// }

type HintSampleEntry struct {
	SampleEntry

	Data []byte
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
	SampleEntry
	Version      uint16 // ffmpeg mov.c mov_parse_stsd_audio
	ChannelCount uint16
	SampleSize   uint16
	Samplerate   uint32
	ExtraData    IBox
}

func (s *SampleEntry) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [8]byte
	binary.BigEndian.PutUint16(tmp[6:], s.DataReferenceIndex)
	_, err = w.Write(tmp[:])
	return 8, err
}

func (s *SampleEntry) Unmarshal(buf []byte) {
	s.DataReferenceIndex = binary.BigEndian.Uint16(buf[6:])
}

func CreateAudioSampleEntry(codecName BoxType, channelCount uint16, sampleSize uint16, samplerate uint32, extraData IBox) *AudioSampleEntry {
	size := 28 + BasicBoxLen
	if extraData != nil {
		size += int(extraData.Size())
	}
	return &AudioSampleEntry{
		SampleEntry: SampleEntry{
			BaseBox: BaseBox{
				typ:  codecName,
				size: uint32(size),
			},
			DataReferenceIndex: 1,
		},
		Version:      0,
		ChannelCount: channelCount,
		SampleSize:   sampleSize,
		Samplerate:   samplerate,
		ExtraData:    extraData,
	}
}

func (audio *AudioSampleEntry) WriteTo(w io.Writer) (n int64, err error) {
	n, err = audio.SampleEntry.WriteTo(w)
	if err != nil {
		return
	}
	var buf [20]byte
	binary.BigEndian.PutUint16(buf[8:], audio.Version)
	binary.BigEndian.PutUint16(buf[10:], audio.ChannelCount)
	binary.BigEndian.PutUint16(buf[12:], audio.SampleSize)
	binary.BigEndian.PutUint32(buf[16:], audio.Samplerate<<16)
	_, err = w.Write(buf[:])
	n += 20
	var nn int64
	if audio.ExtraData != nil {
		nn, err = WriteTo(w, audio.ExtraData)
		if err != nil {
			return
		}
		n += nn
	}
	return

}

func (audio *AudioSampleEntry) Unmarshal(buf []byte) (IBox, error) {
	audio.SampleEntry.Unmarshal(buf)
	buf = buf[8:]
	audio.Version = binary.BigEndian.Uint16(buf[0:])
	audio.ChannelCount = binary.BigEndian.Uint16(buf[8:])
	audio.SampleSize = binary.BigEndian.Uint16(buf[10:])

	audio.Samplerate = binary.BigEndian.Uint32(buf[16:]) >> 16
	if len(buf) > 20 {
		box, err := ReadFrom(bytes.NewReader(buf[20:]))
		if err != nil {
			return nil, err
		}
		audio.ExtraData = box
	}

	return audio, nil
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
	SampleEntry
	Width, Height                   uint16
	Horizresolution, Vertresolution uint32
	FrameCount                      uint16
	Compressorname                  [32]byte
	Depth                           uint16
	ExtraData                       IBox
}

func CreateVisualSampleEntry(codecName BoxType, width, height uint16, extraData IBox) *VisualSampleEntry {
	size := 78 + BasicBoxLen
	if extraData != nil {
		size += int(extraData.Size())
	}
	return &VisualSampleEntry{
		SampleEntry: SampleEntry{
			BaseBox: BaseBox{
				typ:  codecName,
				size: uint32(size),
			},
			DataReferenceIndex: 1,
		},
		Width:           width,
		Height:          height,
		Horizresolution: 0x00480000,
		Vertresolution:  0x00480000,
		FrameCount:      1,
		Depth:           0x0018,
		ExtraData:       extraData,
	}
}

func (visual *VisualSampleEntry) WriteTo(w io.Writer) (n int64, err error) {
	n, err = visual.SampleEntry.WriteTo(w)
	if err != nil {
		return
	}

	var buf [70]byte // 16(pre_defined) + 2(width) + 2(height) + 4(horiz) + 4(vert) + 4(reserved) + 2(frame) + 32(compressor) + 2(depth) + 2(pre_defined)
	binary.BigEndian.PutUint16(buf[16:], visual.Width)
	binary.BigEndian.PutUint16(buf[18:], visual.Height)
	binary.BigEndian.PutUint32(buf[20:], visual.Horizresolution)
	binary.BigEndian.PutUint32(buf[24:], visual.Vertresolution)
	binary.BigEndian.PutUint16(buf[32:], visual.FrameCount)
	copy(buf[34:66], visual.Compressorname[:])
	binary.BigEndian.PutUint16(buf[66:], visual.Depth)
	binary.BigEndian.PutUint16(buf[68:], 0xFFFF) // pre_defined = -1

	_, err = w.Write(buf[:])
	n += 70
	var nn int64
	if visual.ExtraData != nil {
		nn, err = WriteTo(w, visual.ExtraData)
		if err != nil {
			return
		}
		n += nn
	}
	return
}

func (visual *VisualSampleEntry) Unmarshal(buf []byte) (IBox, error) {
	visual.SampleEntry.Unmarshal(buf)
	buf = buf[24:] // Skip 8 bytes from SampleEntry + 16 bytes pre_defined
	visual.Width = binary.BigEndian.Uint16(buf[0:])
	visual.Height = binary.BigEndian.Uint16(buf[2:])

	visual.Horizresolution = binary.BigEndian.Uint32(buf[4:])
	visual.Vertresolution = binary.BigEndian.Uint32(buf[8:])
	visual.FrameCount = binary.BigEndian.Uint16(buf[16:])
	copy(visual.Compressorname[:], buf[18:50])
	visual.Depth = binary.BigEndian.Uint16(buf[50:])

	if len(buf) > 52 {
		box, err := ReadFrom(bytes.NewReader(buf[52:]))
		if err != nil {
			return nil, err
		}
		visual.ExtraData = box
	}

	return visual, nil
}

// aligned(8) class SampleDescriptionBox (unsigned int(32) handler_type) extends FullBox('stsd', 0, 0){
// 	int i ;
// 	unsigned int(32) entry_count;
// 	   for (i = 1 ; i <= entry_count ; i++){
// 		  switch (handler_type){
// 			 case 'soun': // for audio tracks
// 				AudioSampleEntry();
// 				break;
// 			 case 'vide': // for video tracks
// 				VisualSampleEntry();
// 				break;
// 			 case 'hint': // Hint track
// 				HintSampleEntry();
// 				break;
// 			 case 'meta': // Metadata track
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

type STSDBox struct {
	FullBox
	Entries []IBox
}

func CreateSTSDBox(entries ...IBox) *STSDBox {
	childSize := 0
	for _, entry := range entries {
		childSize += int(entry.Size())
	}
	return &STSDBox{
		FullBox: FullBox{
			BaseBox: BaseBox{
				typ:  TypeSTSD,
				size: uint32(FullBoxLen + 4 + childSize),
			},
		},
		Entries: entries,
	}
}

func (stsd *STSDBox) Unmarshal(buf []byte) (IBox, error) {
	stsd.Entries = make([]IBox, 0, binary.BigEndian.Uint32(buf))
	r := bytes.NewReader(buf[4:])
	for {
		box, err := ReadFrom(r)
		if err != nil {
			break
		}
		stsd.Entries = append(stsd.Entries, box)
	}
	return stsd, nil
}

func (stsd *STSDBox) WriteTo(w io.Writer) (n int64, err error) {
	var tmp [4]byte
	var nn int64
	binary.BigEndian.PutUint32(tmp[:], uint32(len(stsd.Entries)))
	_, err = w.Write(tmp[:])
	if err != nil {
		return
	}
	n += 4
	for _, entry := range stsd.Entries {
		nn, err = WriteTo(w, entry)
		if err != nil {
			return
		}
		n += nn
	}
	return int64(n), nil
}

func (h *HintSampleEntry) Unmarshal(buf []byte) (IBox, error) {
	h.SampleEntry.Unmarshal(buf)
	h.Data = buf[8:]
	return h, nil
}

func (h *HintSampleEntry) WriteTo(w io.Writer) (n int64, err error) {
	var offset int64
	offset, err = h.SampleEntry.WriteTo(w)
	if err != nil {
		return
	}
	_, err = w.Write(h.Data)
	return offset + int64(len(h.Data)), err
}

func init() {
	RegisterBox[*STSDBox](TypeSTSD)
	RegisterBox[*AudioSampleEntry](TypeMP4A, TypeULAW, TypeALAW, TypeOPUS, TypeENCA)
	RegisterBox[*VisualSampleEntry](TypeAVC1, TypeHVC1, TypeHEV1, TypeENCV)
	RegisterBox[*HintSampleEntry](TypeHINT)
	RegisterBox[*DataBox](TypeAVCC, TypeHVCC)
	// RegisterBox[*MetadataSampleEntry](TypeMETA)
}

//ffmpeg mov_write_wave_tag
//  avio_wb32(pb, 12);    /* size */
//  ffio_wfourcc(pb, "frma");
//  avio_wl32(pb, track->tag);

// avio_wb32(pb, 12); /* size */
// ffio_wfourcc(pb, "mp4a");
// avio_wb32(pb, 0);
