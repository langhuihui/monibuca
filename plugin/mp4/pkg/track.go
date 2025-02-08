package mp4

import (
	"io"
	"slices"

	. "m7s.live/v5/plugin/mp4/pkg/box"
)

type (
	Track struct {
		Cid     MP4_CODEC_TYPE
		TrackId uint32
		SampleTable
		Duration     uint32
		Height       uint32
		Width        uint32
		SampleRate   uint32
		SampleSize   uint16
		SampleCount  uint32
		ChannelCount uint8
		Timescale    uint32
		// StartDts        uint64
		// EndDts          uint64
		// StartPts        uint64
		// EndPts          uint64
		Samplelist      []Sample
		ELST            *EditListBox
		ExtraData       []byte
		writer          io.WriteSeeker
		fragments       []Fragment
		defaultSize     uint32
		defaultDuration uint32
		// defaultSampleFlags     uint32
		// baseDataOffset         uint64
		// stbl                   []byte
		FragmentSequenceNumber uint32

		//for subsample
		// defaultIsProtected     uint8
		// defaultPerSampleIVSize uint8
		// defaultCryptByteBlock  uint8
		// defaultSkipByteBlock   uint8
		// defaultConstantIV      []byte
		// defaultKID             [16]byte
		// lastSeig               *SeigSampleGroupEntry
		// lastSaiz               *SaizBox
		// subSamples             []SencEntry
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

func (track *Track) makeElstBox() *EditListBox {
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
	entrys := make([]ELSTEntry, entryCount)

	// if entryCount > 1 {
	// 	elst.entrys.entrys[0].segmentDuration = startCt
	// 	elst.entrys.entrys[0].mediaTime = -1

	// 	elst.entrys.entrys[0].mediaRateInteger = 0x0001
	// 	elst.entrys.entrys[0].mediaRateFraction = 0
	// }

	//简单起见，mediaTime先固定为0,即不延迟播放
	entrys[entryCount-1].SegmentDuration = uint64(track.Duration)
	entrys[entryCount-1].MediaTime = 0
	entrys[entryCount-1].MediaRateInteger = 0x0001
	entrys[entryCount-1].MediaRateFraction = 0

	return CreateEditListBox(version, entrys)

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

func (track *Track) makeEdtsBox() *ContainerBox {
	return CreateContainerBox(TypeEDTS, track.makeElstBox())
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

func (track *Track) makeTkhdBox() *TrackHeaderBox {
	tkhd := CreateTrackHeaderBox(track.TrackId, uint64(track.Duration), track.Width, track.Height)
	tkhd.Duration = uint64(track.Duration)
	tkhd.TrackID = track.TrackId

	if track.Cid == MP4_CODEC_AAC || track.Cid == MP4_CODEC_G711A || track.Cid == MP4_CODEC_G711U || track.Cid == MP4_CODEC_OPUS {
		tkhd.Volume = 0x0100
	} else {
		tkhd.Width = track.Width << 16
		tkhd.Height = track.Height << 16
	}
	return tkhd
}

func (track *Track) makeMinfBox() *ContainerBox {
	var mhdbox IBox
	switch track.Cid {
	case MP4_CODEC_H264, MP4_CODEC_H265:
		mhdbox = CreateVideoMediaHeaderBox()

	case MP4_CODEC_G711A, MP4_CODEC_G711U, MP4_CODEC_AAC,
		MP4_CODEC_MP2, MP4_CODEC_MP3, MP4_CODEC_OPUS:
		mhdbox = CreateSoundMediaHeaderBox()
	default:
		panic("unsupport codec id")
	}
	dinfbox := CreateDataInformationBox()
	stblbox := track.makeStblBox()
	return CreateContainerBox(TypeMINF, mhdbox, dinfbox, stblbox)
}

func (track *Track) makeMdiaBox() *ContainerBox {
	mdhdbox := CreateMediaHeaderBox(track.Timescale, uint64(track.Duration))
	hdlrbox := MakeHdlrBox(GetHandlerType(track.Cid))
	minfbox := track.makeMinfBox()
	return CreateContainerBox(TypeMDIA, mdhdbox, hdlrbox, minfbox)
}

func (track *Track) makeStblBox() IBox {
	track.STSD = track.makeStsd(GetHandlerType(track.Cid))
	if track.Cid == MP4_CODEC_H264 || track.Cid == MP4_CODEC_H265 {
		track.STSS = track.makeStssBox()
	}
	return CreateContainerBox(TypeSTBL, track.STSD, track.STSS, track.STTS, track.CTTS, track.STSC, track.STSZ, track.STCO)
}

func (track *Track) makeStsd(handler_type HandlerType) *STSDBox {
	var avbox IBox
	if track.Cid == MP4_CODEC_H264 {
		avbox = CreateDataBox(TypeAVCC, track.ExtraData)
	} else if track.Cid == MP4_CODEC_H265 {
		avbox = CreateDataBox(TypeHVCC, track.ExtraData)
	} else if track.Cid == MP4_CODEC_AAC || track.Cid == MP4_CODEC_MP2 || track.Cid == MP4_CODEC_MP3 {
		avbox = CreateESDSBox(uint16(track.TrackId), track.Cid, track.ExtraData)
	} else if track.Cid == MP4_CODEC_OPUS {
		avbox = CreateOpusSpecificBox(track.ExtraData)
	}
	var entry IBox
	if handler_type == TypeVIDE {
		entry = CreateVisualSampleEntry(GetCodecNameWithCodecId(track.Cid), uint16(track.Width), uint16(track.Height), avbox)
	} else if handler_type == TypeSOUN {
		entry = CreateAudioSampleEntry(GetCodecNameWithCodecId(track.Cid), uint16(track.ChannelCount), uint16(track.SampleSize), track.SampleRate, avbox)
	}
	return CreateSTSDBox(entry)
}

// fmp4
func (track *Track) makeTraf() *TrackFragmentBox {
	// Create tfhd box
	tfhd := track.makeTfhdBox()

	// Create tfdt box
	tfdt := track.makeTfdtBox()

	// Create trun box with all samples
	trun := track.makeTrunBox(0, len(track.Samplelist))

	// Create track fragment box with all necessary boxes
	traf := CreateTrackFragmentBox(tfhd, tfdt, trun)

	return traf
}

func (track *Track) makeTfhdBox() *TrackFragmentHeaderBox {
	tfFlags := uint32(0)
	tfFlags |= TF_FLAG_DEFAULT_BASE_IS_MOOF
	tfFlags |= TF_FLAG_SAMPLE_DESCRIPTION_INDEX_PRESENT
	tfFlags |= TF_FLAG_DEFAULT_SAMPLE_DURATION_PRESENT
	tfFlags |= TF_FLAG_DEFAULT_SAMPLE_SIZE_PRESENT
	tfFlags |= TF_FLAG_DEFAULT_SAMPLE_FLAGS_PRESENT

	tfhd := CreateTrackFragmentHeaderBox(track.TrackId, tfFlags)

	// Calculate default sample duration
	if len(track.Samplelist) > 1 {
		var totalDuration uint64 = 0
		var count int = 0
		for i := 1; i < len(track.Samplelist); i++ {
			duration := track.Samplelist[i].DTS - track.Samplelist[i-1].DTS
			if duration > 0 {
				totalDuration += duration
				count++
			}
		}
		if count > 0 {
			tfhd.DefaultSampleDuration = uint32(totalDuration / uint64(count))
		}
	}

	// Set default sample size
	if len(track.Samplelist) > 0 {
		tfhd.DefaultSampleSize = uint32(track.Samplelist[0].Size)
	}

	// Set default sample flags
	if track.Cid.IsVideo() {
		tfhd.DefaultSampleFlags = MOV_FRAG_SAMPLE_FLAG_DEPENDS_YES | MOV_FRAG_SAMPLE_FLAG_IS_NON_SYNC
	} else {
		tfhd.DefaultSampleFlags = MOV_FRAG_SAMPLE_FLAG_DEPENDS_NO
	}
	return tfhd
}

func (track *Track) makeTfdtBox() *TrackFragmentBaseMediaDecodeTimeBox {
	return CreateTrackFragmentBaseMediaDecodeTimeBox(uint64(track.Samplelist[0].DTS))
}

func (track *Track) makeStssBox() *STSSBox {
	var stss []uint32
	for i, sample := range track.Samplelist {
		if sample.KeyFrame {
			stss = append(stss, uint32(i+1))
		}
	}
	return CreateSTSSBox(stss)
}

func (track *Track) makeTfraBox() *TrackFragmentRandomAccessBox {
	return CreateTrackFragmentRandomAccessBox(track.TrackId, slices.Collect(func(yield func(TFRAEntry) bool) {
		for _, f := range track.fragments {
			if !yield(TFRAEntry{
				Time:       f.FirstPts,
				MoofOffset: f.Offset,
			}) {
				break
			}
		}
	}))
}

func (track *Track) makeTrunBox(start, end int) *TrackRunBox {

	// Set flags in the correct order
	flag := TR_FLAG_DATA_OFFSET

	// Then set sample duration flag (0x000100)
	flag |= TR_FLAG_DATA_SAMPLE_DURATION

	// Then set sample size flag (0x000200)
	flag |= TR_FLAG_DATA_SAMPLE_SIZE

	// Then set sample flags if needed (0x000400)
	if track.Cid.IsVideo() {
		flag |= TR_FLAG_DATA_SAMPLE_FLAGS
	}
	// Create a new TrackRunBox

	// Finally set composition time offset flag if needed (0x000800)
	for i := start; i < end; i++ {
		if track.Samplelist[i].PTS != track.Samplelist[i].DTS {
			flag |= TR_FLAG_DATA_SAMPLE_COMPOSITION_TIME
			break
		}
	}
	trun := CreateTrackRunBox(flag, uint32(end-start))
	// Fill entry list
	trun.Entries = make([]TrunEntry, trun.SampleCount)
	for i := 0; i < int(trun.SampleCount); i++ {
		sample := &track.Samplelist[start+i]
		// Duration
		if i < int(trun.SampleCount)-1 {
			trun.Entries[i].SampleDuration = uint32(track.Samplelist[start+i+1].DTS - sample.DTS)
		} else {
			trun.Entries[i].SampleDuration = trun.Entries[i-1].SampleDuration
		}
		// Size
		trun.Entries[i].SampleSize = uint32(sample.Size)
		// Flags
		if flag&TR_FLAG_DATA_SAMPLE_FLAGS != 0 {
			if sample.KeyFrame {
				trun.Entries[i].SampleFlags = MOV_FRAG_SAMPLE_FLAG_DEPENDS_NO | MOV_FRAG_SAMPLE_FLAG_IS_SYNC
			} else {
				trun.Entries[i].SampleFlags = MOV_FRAG_SAMPLE_FLAG_DEPENDS_YES | MOV_FRAG_SAMPLE_FLAG_IS_NON_SYNC
			}
		}
		// Composition time offset
		if flag&TR_FLAG_DATA_SAMPLE_COMPOSITION_TIME != 0 {
			trun.Entries[i].SampleCompositionTimeOffset = int32(sample.PTS - sample.DTS)
		}
	}

	return trun
}

func (track *Track) makeStblTable() {
	sameSize := true
	movchunks := make([]movchunk, 0)
	ckn := uint32(0)
	var stts []STTSEntry
	var ctts []CTTSEntry
	var stco []uint64

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
	var sampleSize uint32
	var entrySizelist []uint32

	if sameSize {
		sampleSize = uint32(track.Samplelist[0].Size)
	} else {
		entrySizelist = make([]uint32, len(track.Samplelist))

		for i := 0; i < len(track.Samplelist); i++ {
			entrySizelist[i] = uint32(track.Samplelist[i].Size)
		}

	}

	var stsc []STSCEntry
	for i, chunk := range movchunks {
		if i == 0 || chunk.samplenum != movchunks[i-1].samplenum {
			stsc = append(stsc, STSCEntry{FirstChunk: chunk.chunknum + 1, SampleDescriptionIndex: 1, SamplesPerChunk: chunk.samplenum})
		}
	}
	if track.Cid == MP4_CODEC_H264 || track.Cid == MP4_CODEC_H265 {
		track.CTTS = CreateCTTSBox(ctts)
	}
	track.STTS = CreateSTTSBox(stts)
	track.STSC = CreateSTSCBox(stsc)
	track.STCO = CreateSTCOBox(stco)
	track.STSZ = CreateSTSZBox(sampleSize, entrySizelist)
}

// func (track *Track) makeSidxBox(totalSidxSize uint32, refsize uint32) []byte {
// 	sidx := NewSegmentIndexBox()
// 	sidx.ReferenceID = track.TrackId
// 	sidx.TimeScale = track.Timescale
// 	sidx.EarliestPresentationTime = track.StartPts
// 	sidx.ReferenceCount = 1
// 	sidx.FirstOffset = 52 + uint64(totalSidxSize)
// 	entry := SidxEntry{
// 		ReferenceType:      0,
// 		ReferencedSize:     refsize,
// 		SubsegmentDuration: 0,
// 		StartsWithSAP:      1,
// 		SAPType:            0,
// 		SAPDeltaTime:       0,
// 	}

// 	if len(track.Samplelist) > 0 {
// 		entry.SubsegmentDuration = uint32(track.Samplelist[len(track.Samplelist)-1].DTS) - uint32(track.StartDts)
// 	}
// 	sidx.Entrys = append(sidx.Entrys, entry)
// 	sidx.Box.Box.Size = sidx.Size()
// 	_, boxData := sidx.Encode()
// 	return boxData
// }
