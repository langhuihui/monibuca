package mp4

import (
	"io"

	. "m7s.live/v5/plugin/mp4/pkg/box"
)

type (
	Track struct {
		Cid     MP4_CODEC_TYPE
		TrackId uint32
		SampleTable
		Duration           uint32
		Height             uint32
		Width              uint32
		SampleRate         uint32
		SampleSize         uint16
		SampleCount        uint32
		ChannelCount       uint8
		Timescale          uint32
		StartDts           uint64
		EndDts             uint64
		StartPts           uint64
		EndPts             uint64
		Samplelist         []Sample
		ELST               *EditListBox
		ExtraData          []byte
		writer             io.WriteSeeker
		fragments          []Fragment
		defaultSize        uint32
		defaultDuration    uint32
		defaultSampleFlags uint32
		baseDataOffset     uint64

		//for subsample
		defaultIsProtected     uint8
		defaultPerSampleIVSize uint8
		defaultCryptByteBlock  uint8
		defaultSkipByteBlock   uint8
		defaultConstantIV      []byte
		defaultKID             [16]byte
		lastSeig               *SeigSampleGroupEntry
		lastSaiz               *SaizBox
		subSamples             []SencEntry
	}
	Fragment struct {
		Offset   uint64
		Duration uint32
		FirstDts uint64
		FirstPts uint64
		LastPts  uint64
		LastDts  uint64
	}
)

func (track *Track) makeElstBox() []byte {
	delay := track.Samplelist[0].PTS * 1000 / uint64(track.Timescale)
	entryCount := 1
	version := byte(0)
	boxSize := 12
	entrySize := 12
	if delay > 0xFFFFFFFF {
		version = 1
		entrySize = 20
	}
	// if delay > 0 {
	// 	entryCount += 1
	// }
	boxSize += 4 + entrySize*entryCount
	elst := NewEditListBox(version)
	elst.Entrys = make([]ELSTEntry, entryCount)
	// if entryCount > 1 {
	// 	elst.entrys.entrys[0].segmentDuration = startCt
	// 	elst.entrys.entrys[0].mediaTime = -1
	// 	elst.entrys.entrys[0].mediaRateInteger = 0x0001
	// 	elst.entrys.entrys[0].mediaRateFraction = 0
	// }

	//简单起见，mediaTime先固定为0,即不延迟播放
	elst.Entrys[entryCount-1].SegmentDuration = uint64(track.Duration)
	elst.Entrys[entryCount-1].MediaTime = 0
	elst.Entrys[entryCount-1].MediaRateInteger = 0x0001
	elst.Entrys[entryCount-1].MediaRateFraction = 0

	_, boxdata := elst.Encode(boxSize)
	return boxdata

}

func (track *Track) Seek(dts uint64) int {
	for i, sample := range track.Samplelist {
		if sample.DTS*1000/uint64(track.Timescale) < dts {
			continue
		} else if track.Cid.IsVideo() {
			if sample.KeyFrame {
				return i
			}
		} else {
			return i
		}
	}
	return -1
}

func (track *Track) makeEdtsBox() []byte {
	elst := track.makeElstBox()
	edts := BasicBox{Type: TypeEDTS, Size: 8 + uint64(len(elst))}
	offset, edtsbox := edts.Encode()
	copy(edtsbox[offset:], elst)
	return edtsbox
}

func (track *Track) AddSampleEntry(entry Sample) {
	if len(track.Samplelist) <= 1 {
		track.Duration = 0
	} else {
		delta := int64(entry.DTS - track.Samplelist[len(track.Samplelist)-1].DTS)
		if delta < 0 {
			track.Duration += 1
		} else {
			track.Duration += uint32(delta)
		}
	}
	track.Samplelist = append(track.Samplelist, entry)
}

func (track *Track) makeTkhdBox() []byte {
	tkhd := NewTrackHeaderBox()
	tkhd.Duration = uint64(track.Duration)
	tkhd.Track_ID = track.TrackId
	if track.Cid == MP4_CODEC_AAC || track.Cid == MP4_CODEC_G711A || track.Cid == MP4_CODEC_G711U || track.Cid == MP4_CODEC_OPUS {
		tkhd.Volume = 0x0100
	} else {
		tkhd.Width = track.Width << 16
		tkhd.Height = track.Height << 16
	}
	_, tkhdbox := tkhd.Encode()
	return tkhdbox
}

func (track *Track) makeMinfBox() []byte {
	var mhdbox []byte
	switch track.Cid {
	case MP4_CODEC_H264, MP4_CODEC_H265:
		mhdbox = MakeVmhdBox()
	case MP4_CODEC_G711A, MP4_CODEC_G711U, MP4_CODEC_AAC,
		MP4_CODEC_MP2, MP4_CODEC_MP3, MP4_CODEC_OPUS:
		mhdbox = MakeSmhdBox()
	default:
		panic("unsupport codec id")
	}
	dinfbox := MakeDefaultDinfBox()
	stblbox := track.makeStblBox()

	minf := BasicBox{Type: TypeMINF, Size: 8 + uint64(len(mhdbox)+len(dinfbox)+len(stblbox))}
	offset, minfbox := minf.Encode()
	copy(minfbox[offset:], mhdbox)
	offset += len(mhdbox)
	copy(minfbox[offset:], dinfbox)
	offset += len(dinfbox)
	copy(minfbox[offset:], stblbox)
	offset += len(stblbox)
	return minfbox
}

func (track *Track) makeMdiaBox() []byte {
	mdhdbox := MakeMdhdBox(track.Duration)
	hdlrbox := MakeHdlrBox(GetHandlerType(track.Cid))
	minfbox := track.makeMinfBox()
	mdia := BasicBox{Type: TypeMDIA, Size: 8 + uint64(len(mdhdbox)+len(hdlrbox)+len(minfbox))}
	offset, mdiabox := mdia.Encode()
	copy(mdiabox[offset:], mdhdbox)
	offset += len(mdhdbox)
	copy(mdiabox[offset:], hdlrbox)
	offset += len(hdlrbox)
	copy(mdiabox[offset:], minfbox)
	offset += len(minfbox)
	return mdiabox
}

func (track *Track) makeStblBox() []byte {
	var stsdbox, sttsbox, cttsbox, stscbox, stszbox, stcobox, stssbox []byte
	stsdbox = track.makeStsd(GetHandlerType(track.Cid))
	if track.SampleTable.STTS != nil {
		_, sttsbox = track.SampleTable.STTS.Encode()
	}
	if track.SampleTable.CTTS != nil {
		_, cttsbox = track.SampleTable.CTTS.Encode()
	}
	if track.SampleTable.STSC != nil {
		_, stscbox = track.SampleTable.STSC.Encode()
	}
	if track.SampleTable.STSZ != nil {
		_, stszbox = track.SampleTable.STSZ.Encode()
	}
	if track.SampleTable.STCO != nil {
		_, stcobox = track.SampleTable.STCO.Encode()
	}
	if track.Cid == MP4_CODEC_H264 || track.Cid == MP4_CODEC_H265 {
		stssbox = track.makeStssBox()
	}

	stbl := BasicBox{Type: TypeSTBL, Size: uint64(8 + len(stsdbox) + len(sttsbox) + len(cttsbox) + len(stscbox) + len(stszbox) + len(stcobox) + len(stssbox))}
	offset, stblbox := stbl.Encode()
	copy(stblbox[offset:], stsdbox)
	offset += len(stsdbox)
	copy(stblbox[offset:], sttsbox)
	offset += len(sttsbox)
	copy(stblbox[offset:], cttsbox)
	offset += len(cttsbox)
	copy(stblbox[offset:], stscbox)
	offset += len(stscbox)
	copy(stblbox[offset:], stszbox)
	offset += len(stszbox)
	copy(stblbox[offset:], stcobox)
	offset += len(stcobox)
	copy(stblbox[offset:], stssbox)
	offset += len(stssbox)
	return stblbox
}

func (track *Track) makeStsd(handler_type HandlerType) []byte {
	var avbox []byte
	if track.Cid == MP4_CODEC_H264 {
		avbox = MakeAvcCBox(track.ExtraData)
	} else if track.Cid == MP4_CODEC_H265 {
		avbox = MakeHvcCBox(track.ExtraData)
	} else if track.Cid == MP4_CODEC_AAC || track.Cid == MP4_CODEC_MP2 || track.Cid == MP4_CODEC_MP3 {
		avbox = MakeEsdsBox(track.TrackId, track.Cid, track.ExtraData)
	} else if track.Cid == MP4_CODEC_OPUS {
		avbox = MakeOpusSpecificBox(track.ExtraData)
	}

	var se []byte
	var offset int
	if handler_type == TypeVIDE {
		entry := NewVisualSampleEntry(GetCodecNameWithCodecId(track.Cid))
		entry.Width = uint16(track.Width)
		entry.Height = uint16(track.Height)
		offset, se = entry.Encode(entry.Size() + uint64(len(avbox)))
	} else if handler_type == TypeSOUN {
		entry := NewAudioSampleEntry(GetCodecNameWithCodecId(track.Cid))
		entry.ChannelCount = uint16(track.ChannelCount)
		entry.Samplerate = track.SampleRate
		entry.SampleSize = track.SampleSize
		offset, se = entry.Encode(entry.Size() + uint64(len(avbox)))
	}
	copy(se[offset:], avbox)

	var stsd SampleDescriptionBox = 1
	offset2, stsdbox := stsd.Encode(FullBoxLen + 4 + uint64(len(se)))
	copy(stsdbox[offset2:], se)
	return stsdbox
}

// fmp4
func (track *Track) makeTraf(moofOffset int64, moofSize int64) []byte {
	tfhd := track.makeTfhdBox(uint64(moofOffset))
	tfdt := track.makeTfdtBox()
	trun := track.makeTrunBoxes(moofSize)

	traf := BasicBox{Type: TypeTRAF, Size: 8 + uint64(len(tfhd)+len(tfdt)+len(trun))}
	offset, boxData := traf.Encode()
	copy(boxData[offset:], tfhd)
	offset += len(tfhd)
	copy(boxData[offset:], tfdt)
	offset += len(tfdt)
	copy(boxData[offset:], trun)
	offset += len(trun)
	return boxData
}

func (track *Track) makeTfhdBox(offset uint64) []byte {
	tfFlags := TF_FLAG_SAMPLE_DESCRIPTION_INDEX_PRESENT
	tfFlags |= TF_FLAG_DEAAULT_BASE_IS_MOOF
	tfhd := NewTrackFragmentHeaderBox(track.TrackId)
	tfhd.BaseDataOffset = offset
	if len(track.Samplelist) > 1 {
		tfhd.DefaultSampleDuration = uint32(track.Samplelist[1].DTS - track.Samplelist[0].DTS)
	} else if len(track.Samplelist) == 1 && len(track.fragments) > 0 {
		tfhd.DefaultSampleDuration = uint32(track.Samplelist[0].DTS - track.fragments[len(track.fragments)-1].LastDts)
	} else {
		tfhd.DefaultSampleDuration = 0
		tfFlags |= TF_FLAG_DURATION_IS_EMPTY
	}
	if len(track.Samplelist) > 0 {
		tfFlags |= TF_FLAG_DEAAULT_SAMPLE_FLAGS_PRESENT
		tfFlags |= TF_FLAG_DEFAULT_SAMPLE_DURATION_PRESENT
		tfFlags |= TF_FLAG_DEFAULT_SAMPLE_SIZE_PRESENT
		tfhd.DefaultSampleSize = uint32(track.Samplelist[0].Size)
	} else {
		tfhd.DefaultSampleSize = 0
	}
	//ffmpeg movenc.c mov_write_tfhd_tag
	if track.Cid.IsVideo() {
		tfhd.DefaultSampleFlags = MOV_FRAG_SAMPLE_FLAG_DEPENDS_YES | MOV_FRAG_SAMPLE_FLAG_IS_NON_SYNC
	} else {
		tfhd.DefaultSampleFlags = MOV_FRAG_SAMPLE_FLAG_DEPENDS_NO
	}
	track.defaultDuration = tfhd.DefaultSampleDuration
	track.defaultSize = tfhd.DefaultSampleSize
	track.defaultSampleFlags = tfhd.DefaultSampleFlags
	_, boxData := tfhd.Encode(tfFlags)
	return boxData
}

func (track *Track) makeTfdtBox() []byte {
	tfdt := NewTrackFragmentBaseMediaDecodeTimeBox(uint64(track.Samplelist[0].DTS))
	_, boxData := tfdt.Encode()
	return boxData
}

func (track *Track) makeTrunBoxes(moofSize int64) []byte {
	boxes := make([]byte, 0, 128)
	start := 0
	end := 0
	for i := 1; i < len(track.Samplelist); i++ {
		if track.Samplelist[i].Offset == track.Samplelist[i-1].Offset+int64(track.Samplelist[i-1].Size) {
			continue
		}
		end = i
		boxes = append(boxes, track.makeTrunBox(start, end, moofSize)...)
		start = end
	}

	if start < len(track.Samplelist) {
		boxes = append(boxes, track.makeTrunBox(start, len(track.Samplelist), moofSize)...)
	}
	return boxes
}

func (track *Track) makeStssBox() (boxdata []byte) {
	var stss SyncSampleBox
	for i, sample := range track.Samplelist {
		if sample.KeyFrame {
			stss = append(stss, uint32(i+1))
		}
	}
	_, boxdata = stss.Encode()
	return
}

func (track *Track) makeTfraBox() []byte {
	tfra := NewTrackFragmentRandomAccessBox(track.TrackId)
	tfra.LengthSizeOfSampleNum = 0
	tfra.LengthSizeOfTrafNum = 0
	tfra.LengthSizeOfTrunNum = 0
	for _, f := range track.fragments {
		tfra.FragEntrys = append(tfra.FragEntrys, FragEntry{
			Time:       f.FirstPts,
			MoofOffset: f.Offset,
		})
	}
	_, tfraData := tfra.Encode()
	return tfraData
}

func (track *Track) makeTrunBox(start, end int, moofSize int64) []byte {
	flag := TR_FLAG_DATA_OFFSET
	if track.Cid.IsVideo() && track.Samplelist[start].KeyFrame {
		flag |= TR_FLAG_DATA_FIRST_SAMPLE_FLAGS
	}

	for j := start; j < end; j++ {
		if track.Samplelist[j].Size != int(track.defaultSize) {
			flag |= TR_FLAG_DATA_SAMPLE_SIZE
		}
		if j+1 < end {
			if track.Samplelist[j+1].DTS-track.Samplelist[j].DTS != uint64(track.defaultDuration) {
				flag |= TR_FLAG_DATA_SAMPLE_DURATION
			}
		} else {
			// if track.lastSample.DTS-track.Samplelist[j].DTS != uint64(track.defaultDuration) {
			// 	flag |= TR_FLAG_DATA_SAMPLE_DURATION
			// }
		}
		if track.Samplelist[j].PTS != track.Samplelist[j].DTS {
			flag |= TR_FLAG_DATA_SAMPLE_COMPOSITION_TIME
		}
	}

	trun := NewTrackRunBox()
	trun.SampleCount = uint32(end - start)

	trun.Dataoffset = int32(moofSize + track.Samplelist[start].Offset)
	trun.FirstSampleFlags = MOV_FRAG_SAMPLE_FLAG_DEPENDS_NO
	for i := start; i < end; i++ {
		sampleDuration := uint32(0)
		if i == len(track.Samplelist)-1 {
			sampleDuration = track.defaultDuration
		} else {
			sampleDuration = uint32(track.Samplelist[i+1].DTS - track.Samplelist[i].DTS)
		}

		entry := TrunEntry{
			SampleDuration:              sampleDuration,
			SampleSize:                  uint32(track.Samplelist[i].Size),
			SampleCompositionTimeOffset: uint32(track.Samplelist[i].PTS - track.Samplelist[i].DTS),
		}
		trun.EntryList = append(trun.EntryList, entry)
	}
	_, boxData := trun.Encode(flag)
	return boxData
}

func (track *Track) makeStblTable() {
	sameSize := true
	movchunks := make([]movchunk, 0)
	ckn := uint32(0)
	var stts TimeToSampleBox
	var ctts CompositionOffsetBox
	var stco ChunkOffsetBox
	for i, sample := range track.Samplelist {
		sttsEntry := STTSEntry{SampleCount: 1, SampleDelta: 1}
		cttsEntry := CTTSEntry{SampleCount: 1, SampleOffset: uint32(sample.PTS) - uint32(sample.DTS)}
		if i == len(track.Samplelist)-1 {
			stts = append(stts, sttsEntry)
		} else {
			var delta uint64 = 1
			if track.Samplelist[i+1].PTS >= sample.PTS {
				delta = track.Samplelist[i+1].PTS - sample.PTS
			}

			if len(stts) > 0 && delta == uint64(stts[len(stts)-1].SampleDelta) {
				stts[len(stts)-1].SampleCount++
			} else {
				sttsEntry.SampleDelta = uint32(delta)
				stts = append(stts, sttsEntry)
			}
		}

		if len(ctts) == 0 {
			ctts = append(ctts, cttsEntry)
		} else {
			if ctts[len(ctts)-1].SampleOffset == cttsEntry.SampleOffset {
				ctts[len(ctts)-1].SampleCount++
			} else {
				ctts = append(ctts, cttsEntry)
			}
		}
		if sameSize && i < len(track.Samplelist)-1 && track.Samplelist[i+1].Size != track.Samplelist[i].Size {
			sameSize = false
		}
		if i > 0 && sample.Offset == track.Samplelist[i-1].Offset+int64(track.Samplelist[i-1].Size) {
			movchunks[ckn-1].samplenum++
		} else {
			ck := movchunk{chunknum: ckn, samplenum: 1, chunkoffset: uint64(sample.Offset)}
			movchunks = append(movchunks, ck)
			stco = append(stco, uint64(sample.Offset))
			ckn++
		}
	}
	stsz := &SampleSizeBox{
		SampleSize:  0,
		SampleCount: uint32(len(track.Samplelist)),
	}
	if sameSize {
		stsz.SampleSize = uint32(track.Samplelist[0].Size)
	} else {
		stsz.EntrySizelist = make([]uint32, stsz.SampleCount)
		for i := 0; i < len(stsz.EntrySizelist); i++ {
			stsz.EntrySizelist[i] = uint32(track.Samplelist[i].Size)
		}
	}

	var stsc SampleToChunkBox
	for i, chunk := range movchunks {
		if i == 0 || chunk.samplenum != movchunks[i-1].samplenum {
			stsc = append(stsc, STSCEntry{FirstChunk: chunk.chunknum + 1, SampleDescriptionIndex: 1, SamplesPerChunk: chunk.samplenum})
		}
	}

	track.SampleTable.STTS = &stts
	track.SampleTable.STSC = &stsc
	track.SampleTable.STCO = &stco
	track.SampleTable.STSZ = stsz
	if track.Cid == MP4_CODEC_H264 || track.Cid == MP4_CODEC_H265 {
		track.SampleTable.CTTS = &ctts
	}
}

func (track *Track) makeSidxBox(totalSidxSize uint32, refsize uint32) []byte {
	sidx := NewSegmentIndexBox()
	sidx.ReferenceID = track.TrackId
	sidx.TimeScale = track.Timescale
	sidx.EarliestPresentationTime = track.StartPts
	sidx.ReferenceCount = 1
	sidx.FirstOffset = 52 + uint64(totalSidxSize)
	entry := SidxEntry{
		ReferenceType:      0,
		ReferencedSize:     refsize,
		SubsegmentDuration: 0,
		StartsWithSAP:      1,
		SAPType:            0,
		SAPDeltaTime:       0,
	}

	if len(track.Samplelist) > 0 {
		entry.SubsegmentDuration = uint32(track.Samplelist[len(track.Samplelist)-1].DTS) - uint32(track.StartDts)
	}
	sidx.Entrys = append(sidx.Entrys, entry)
	sidx.Box.Box.Size = sidx.Size()
	_, boxData := sidx.Encode()
	return boxData
}
