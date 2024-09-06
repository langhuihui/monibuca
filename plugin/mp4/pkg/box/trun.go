package box

import (
	"encoding/binary"
	"errors"
	"io"
)

// aligned(8) class TrackRunBox extends FullBox(‘trun’, version, tr_flags) {
//      unsigned int(32) sample_count;
//      // the following are optional fields
//      signed int(32) data_offset;
//       unsigned int(32) first_sample_flags;
//      // all fields in the following array are optional
//      {
//          unsigned int(32) sample_duration;
//          unsigned int(32) sample_size;
//          unsigned int(32) sample_flags
//          if (version == 0)
//          {
//              unsigned int(32) sample_composition_time_offset;
//          }
//          else
//          {
//              signed int(32) sample_composition_time_offset;
//          }
//      }[ sample_count ]
// }

const (
	TR_FLAG_DATA_OFFSET                  uint32 = 0x000001
	TR_FLAG_DATA_FIRST_SAMPLE_FLAGS      uint32 = 0x000004
	TR_FLAG_DATA_SAMPLE_DURATION         uint32 = 0x000100
	TR_FLAG_DATA_SAMPLE_SIZE             uint32 = 0x000200
	TR_FLAG_DATA_SAMPLE_FLAGS            uint32 = 0x000400
	TR_FLAG_DATA_SAMPLE_COMPOSITION_TIME uint32 = 0x000800
)

type TrackRunBox struct {
	Box              *FullBox
	SampleCount      uint32
	Dataoffset       int32
	FirstSampleFlags uint32
	EntryList        *movtrun
}

func NewTrackRunBox() *TrackRunBox {
	return &TrackRunBox{
		Box: NewFullBox([4]byte{'t', 'r', 'u', 'n'}, 1),
	}
}

func (trun *TrackRunBox) Size() uint64 {
	n := trun.Box.Size()
	n += 4
	trunFlags := uint32(trun.Box.Flags[0])<<16 | uint32(trun.Box.Flags[1])<<8 | uint32(trun.Box.Flags[2])
	if trunFlags&uint32(TR_FLAG_DATA_OFFSET) > 0 {
		n += 4
	}
	if trunFlags&uint32(TR_FLAG_DATA_FIRST_SAMPLE_FLAGS) > 0 {
		n += 4
	}
	if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_DURATION) > 0 {
		n += 4 * uint64(trun.SampleCount)
	}
	if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_SIZE) > 0 {
		n += 4 * uint64(trun.SampleCount)
	}
	if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_FLAGS) > 0 {
		n += 4 * uint64(trun.SampleCount)
	}
	if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_COMPOSITION_TIME) > 0 {
		n += 4 * uint64(trun.SampleCount)
	}
	return n
}

func (trun *TrackRunBox) Decode(r io.Reader, size uint32, dataOffset uint32) (offset int, err error) {
	if offset, err = trun.Box.Decode(r); err != nil {
		return
	}
	buf := make([]byte, size-12)
	if _, err = io.ReadFull(r, buf); err != nil {
		return 0, err
	}
	n := 0
	trun.SampleCount = binary.BigEndian.Uint32(buf[n:])
	n += 4
	trunFlags := uint32(trun.Box.Flags[0])<<16 | uint32(trun.Box.Flags[1])<<8 | uint32(trun.Box.Flags[2])

	if trunFlags&uint32(TR_FLAG_DATA_OFFSET) > 0 {
		trun.Dataoffset = int32(binary.BigEndian.Uint32(buf[n:]))
		n += 4
	} else {
		trun.Dataoffset = int32(dataOffset)
	}
	if trunFlags&uint32(TR_FLAG_DATA_FIRST_SAMPLE_FLAGS) > 0 {
		trun.FirstSampleFlags = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}
	trun.EntryList = new(movtrun)
	trun.EntryList.entrys = make([]trunEntry, trun.SampleCount)
	for i := 0; i < int(trun.SampleCount); i++ {
		if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_DURATION) > 0 {
			trun.EntryList.entrys[i].sampleDuration = binary.BigEndian.Uint32(buf[n:])
			n += 4
		}
		if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_SIZE) > 0 {
			trun.EntryList.entrys[i].sampleSize = binary.BigEndian.Uint32(buf[n:])
			n += 4
		}
		if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_FLAGS) > 0 {
			trun.EntryList.entrys[i].sampleFlags = binary.BigEndian.Uint32(buf[n:])
			n += 4
		}
		if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_COMPOSITION_TIME) > 0 {
			trun.EntryList.entrys[i].sampleCompositionTimeOffset = binary.BigEndian.Uint32(buf[n:])
			n += 4
		}
	}
	offset += n
	return
}

func (trun *TrackRunBox) Encode() (int, []byte) {
	trun.Box.Box.Size = trun.Size()
	offset, buf := trun.Box.Encode()
	binary.BigEndian.PutUint32(buf[offset:], trun.SampleCount)
	offset += 4
	trunFlags := uint32(trun.Box.Flags[0])<<16 | uint32(trun.Box.Flags[1])<<8 | uint32(trun.Box.Flags[2])

	if trunFlags&uint32(TR_FLAG_DATA_OFFSET) > 0 {
		binary.BigEndian.PutUint32(buf[offset:], uint32(trun.Dataoffset))
		offset += 4
	}
	if trunFlags&uint32(TR_FLAG_DATA_FIRST_SAMPLE_FLAGS) > 0 {
		binary.BigEndian.PutUint32(buf[offset:], trun.FirstSampleFlags)
		offset += 4
	}

	for i := 0; i < int(trun.SampleCount); i++ {
		if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_DURATION) != 0 {
			binary.BigEndian.PutUint32(buf[offset:], trun.EntryList.entrys[i].sampleDuration)
			offset += 4
		}
		if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_SIZE) != 0 {
			binary.BigEndian.PutUint32(buf[offset:], trun.EntryList.entrys[i].sampleSize)
			offset += 4
		}
		if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_FLAGS) != 0 {
			binary.BigEndian.PutUint32(buf[offset:], trun.EntryList.entrys[i].sampleFlags)
			offset += 4
		}
		if trunFlags&uint32(TR_FLAG_DATA_SAMPLE_COMPOSITION_TIME) != 0 {
			binary.BigEndian.PutUint32(buf[offset:], trun.EntryList.entrys[i].sampleCompositionTimeOffset)
			offset += 4
		}
	}
	return offset, buf
}

func decodeTrunBox(demuxer *MovDemuxer, size uint32) (err error) {
	trun := TrackRunBox{Box: new(FullBox)}
	if _, err = trun.Decode(demuxer.reader, size, uint32(demuxer.dataOffset)); err != nil {
		return err
	}

	if demuxer.currentTrack == nil {
		return errors.New("current track is nil")
	}

	dataOffset := trun.Dataoffset
	nextDts := demuxer.currentTrack.startDts
	delta := 0
	var cts int64 = 0
	for _, entry := range trun.EntryList.entrys {
		sample := sampleEntry{}
		sample.offset = uint64(dataOffset) + demuxer.currentTrack.baseDataOffset
		sample.dts = nextDts
		if entry.sampleSize == 0 {
			dataOffset += int32(demuxer.currentTrack.defaultSize)
			sample.size = uint64(demuxer.currentTrack.defaultSize)
		} else {
			dataOffset += int32(entry.sampleSize)
			sample.size = uint64(entry.sampleSize)
		}

		if entry.sampleDuration == 0 {
			delta = int(demuxer.currentTrack.defaultDuration)
		} else {
			delta = int(entry.sampleDuration)
		}
		cts = int64(entry.sampleCompositionTimeOffset)
		sample.pts = uint64(int64(sample.dts) + cts)
		nextDts += uint64(delta)
		demuxer.currentTrack.samplelist = append(demuxer.currentTrack.samplelist, sample)
	}
	demuxer.dataOffset = uint32(dataOffset)
	return
}

func makeTrunBoxes(track *mp4track, moofSize uint64) []byte {
	boxes := make([]byte, 0, 128)
	start := 0
	end := 0
	for i := 1; i < len(track.samplelist); i++ {
		if track.samplelist[i].offset == track.samplelist[i-1].offset+track.samplelist[i-1].size {
			continue
		}
		end = i
		boxes = append(boxes, makeTrunBox(track, start, end, moofSize)...)
		start = end
	}

	if start < len(track.samplelist) {
		boxes = append(boxes, makeTrunBox(track, start, len(track.samplelist), moofSize)...)
	}
	return boxes
}

func makeTrunBox(track *mp4track, start, end int, moofSize uint64) []byte {
	flag := TR_FLAG_DATA_OFFSET
	if isVideo(track.cid) && track.samplelist[start].isKeyFrame {
		flag |= TR_FLAG_DATA_FIRST_SAMPLE_FLAGS
	}

	for j := start; j < end; j++ {
		if track.samplelist[j].size != uint64(track.defaultSize) {
			flag |= TR_FLAG_DATA_SAMPLE_SIZE
		}
		if j+1 < end {
			if track.samplelist[j+1].dts-track.samplelist[j].dts != uint64(track.defaultDuration) {
				flag |= TR_FLAG_DATA_SAMPLE_DURATION
			}
		} else {
			if track.lastSample.dts-track.samplelist[j].dts != uint64(track.defaultDuration) {
				flag |= TR_FLAG_DATA_SAMPLE_DURATION
			}
		}
		if track.samplelist[j].pts != track.samplelist[j].dts {
			flag |= TR_FLAG_DATA_SAMPLE_COMPOSITION_TIME
		}
	}

	trun := NewTrackRunBox()
	trun.Box.Flags[0] = uint8(flag >> 16)
	trun.Box.Flags[1] = uint8(flag >> 8)
	trun.Box.Flags[2] = uint8(flag)
	trun.SampleCount = uint32(end - start)

	trun.Dataoffset = int32(moofSize + track.samplelist[start].offset)
	trun.FirstSampleFlags = MOV_FRAG_SAMPLE_FLAG_DEPENDS_NO
	trun.EntryList = new(movtrun)
	for i := start; i < end; i++ {
		sampleDuration := uint32(0)
		if i == len(track.samplelist)-1 {
			if track.lastSample != nil && track.lastSample.dts != 0 {
				sampleDuration = uint32(track.lastSample.dts - track.samplelist[i].dts)
			} else {
				sampleDuration = track.defaultDuration
			}
		} else {
			sampleDuration = uint32(track.samplelist[i+1].dts - track.samplelist[i].dts)
		}

		entry := trunEntry{
			sampleDuration:              sampleDuration,
			sampleSize:                  uint32(track.samplelist[i].size),
			sampleCompositionTimeOffset: uint32(track.samplelist[i].pts - track.samplelist[i].dts),
		}
		trun.EntryList.entrys = append(trun.EntryList.entrys, entry)
	}
	_, boxData := trun.Encode()
	return boxData
}
