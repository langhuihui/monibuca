package mp4

import (
	"slices"

	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	. "m7s.live/v5/plugin/mp4/pkg/box"
)

type (
	Track struct {
		codec.ICodecCtx
		Cid     MP4_CODEC_TYPE
		TrackId uint32
		SampleTable
		Duration  uint32
		Timescale uint32
		// StartDts        uint64
		// EndDts          uint64
		// StartPts        uint64
		// EndPts          uint64
		Samplelist      []Sample
		isFragment      bool
		fragments       []Fragment
		defaultSize     uint32
		defaultDuration uint32
		// defaultSampleFlags     uint32
		// baseDataOffset         uint64
		// stbl                   []byte
		// FragmentSequenceNumber uint32

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
		FirstTs  uint64
		LastTs   uint64
	}
)

func (track *Track) makeElstBox() *EditListBox {
	delay := track.Samplelist[0].Timestamp * 1000 / uint32(track.Timescale)
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

func (track *Track) Seek(dts uint64) (idx int) {
	idx = -1
	for i, sample := range track.Samplelist {
		if track.Cid.IsVideo() && sample.KeyFrame {
			idx = i
		}
		if sample.Timestamp*1000/uint32(track.Timescale) > uint32(dts) {
			break
		}
	}
	return
}

func (track *Track) makeEdtsBox() *ContainerBox {
	return CreateContainerBox(TypeEDTS, track.makeElstBox())
}

func (track *Track) AddSampleEntry(entry Sample) {
	if len(track.Samplelist) < 1 {
		track.Duration = 0
	} else {
		delta := int64(entry.Timestamp - track.Samplelist[len(track.Samplelist)-1].Timestamp)
		track.Samplelist[len(track.Samplelist)-1].Duration = uint32(delta)
		if delta < 0 {
			track.Duration += 1
		} else {
			track.Duration += uint32(delta)
		}
	}
	track.Samplelist = append(track.Samplelist, entry)
}

func (track *Track) makeTkhdBox() *TrackHeaderBox {
	duration := uint64(track.Duration)
	tkhd := CreateTrackHeaderBox(track.TrackId, duration)
	switch ctx := track.ICodecCtx.(type) {
	case pkg.IVideoCodecCtx:
		tkhd.Width = uint32(ctx.Width()) << 16
		tkhd.Height = uint32(ctx.Height()) << 16
	case pkg.IAudioCodecCtx:
		tkhd.Volume = 0x0100
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
	duration := uint64(track.Duration)
	if track.isFragment {
		duration = 0
	}
	mdhdbox := CreateMediaHeaderBox(track.Timescale, duration)
	hdlrbox := MakeHdlrBox(GetHandlerType(track.Cid))
	minfbox := track.makeMinfBox()
	return CreateContainerBox(TypeMDIA, mdhdbox, hdlrbox, minfbox)
}

func (track *Track) makeStblBox() IBox {
	track.STSD = track.makeStsd()
	if !track.isFragment {
		if track.Cid == MP4_CODEC_H264 || track.Cid == MP4_CODEC_H265 {
			track.STSS = track.makeStssBox()
		}
	} else {
		track.STTS = CreateSTTSBox(nil)
		track.STSC = CreateSTSCBox(nil)
		track.STSZ = CreateSTSZBox(0, nil)
		track.STCO = CreateSTCOBox(nil)
	}
	return CreateContainerBox(TypeSTBL, track.STSD, track.STSS, track.STSZ, track.STSC, track.STTS, track.CTTS, track.STCO)
}

func (track *Track) makeStsd() *STSDBox {
	var avbox IBox
	var entry IBox
	switch ctx := track.ICodecCtx.(type) {
	case pkg.IVideoCodecCtx:
		switch track.Cid {
		case MP4_CODEC_H264:
			avbox = CreateDataBox(TypeAVCC, track.GetRecord())
		case MP4_CODEC_H265:
			avbox = CreateDataBox(TypeHVCC, track.GetRecord())
		}
		entry = CreateVisualSampleEntry(GetCodecNameWithCodecId(track.Cid), uint16(ctx.Width()), uint16(ctx.Height()), avbox)
	case pkg.IAudioCodecCtx:
		if track.Cid == MP4_CODEC_OPUS {
			avbox = CreateOpusSpecificBox(track.GetRecord())
		} else {
			avbox = CreateESDSBox(uint16(track.TrackId), track.Cid, track.GetRecord())
		}
		entry = CreateAudioSampleEntry(GetCodecNameWithCodecId(track.Cid), uint16(ctx.GetChannels()), uint16(ctx.GetSampleSize()), uint32(ctx.GetSampleRate()), avbox)
	}
	return CreateSTSDBox(entry)
}

func (track *Track) MakeMoof(fragmentId uint32) *ContainerBox {
	tfhd := track.makeTfhdBox(0)
	tfdt := track.makeTfdtBox()

	turnEntries := make([]TrunEntry, 0)
	for _, sample := range track.Samplelist {
		var sampleFlags uint32
		if sample.KeyFrame {
			sampleFlags = SAMPLE_FLAG_IS_LEADING | SAMPLE_FLAG_IS_DEPENDED_ON | SAMPLE_FLAG_DEPENDS_ON_NO
		} else {
			sampleFlags = SAMPLE_FLAG_DEPENDS_ON_YES
		}
		turnEntries = append(turnEntries, TrunEntry{
			SampleSize:                  uint32(sample.Size),
			SampleDuration:              uint32(sample.Duration),
			SampleCompositionTimeOffset: int32(sample.CTS),
			SampleFlags:                 sampleFlags,
		})
	}

	trun := CreateTrackRunBox(TR_FLAG_DATA_OFFSET|TR_FLAG_DATA_SAMPLE_DURATION|TR_FLAG_DATA_SAMPLE_SIZE|TR_FLAG_DATA_SAMPLE_FLAGS|TR_FLAG_DATA_SAMPLE_COMPOSITION_TIME, turnEntries)

	traf := CreateContainerBox(TypeTRAF, tfhd, tfdt, trun)
	result := CreateContainerBox(TypeMOOF, CreateMovieFragmentHeaderBox(fragmentId), traf)
	trun.DataOffset = int32(result.Size()) + int32(BasicBoxLen) // TODO: mdat large than 4GB
	return result
}

func (track *Track) makeTfhdBox(moofOffset uint64) *TrackFragmentHeaderBox {
	tfFlags := uint32(0)
	// tfFlags |= TF_FLAG_DEFAULT_BASE_IS_MOOF
	// tfFlags |= TF_FLAG_DEFAULT_SAMPLE_FLAGS_PRESENT
	tfhd := CreateTrackFragmentHeaderBox(track.TrackId, tfFlags)
	tfhd.BaseDataOffset = moofOffset
	// Calculate default sample duration
	if len(track.Samplelist) > 1 {
		var totalDuration uint64 = 0
		var count int = 0
		for i := 1; i < len(track.Samplelist); i++ {
			duration := track.Samplelist[i].Timestamp - track.Samplelist[i-1].Timestamp
			if duration > 0 {
				totalDuration += uint64(duration)
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
		tfhd.DefaultSampleFlags = 16842752
	} else {
		tfhd.DefaultSampleFlags = 0
	}
	return tfhd
}

func (track *Track) makeTfdtBox() *TrackFragmentBaseMediaDecodeTimeBox {
	return CreateTrackFragmentBaseMediaDecodeTimeBox(uint64(track.Samplelist[0].Timestamp))
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
				Time:         f.FirstTs,
				MoofOffset:   f.Offset,
				TrafNumber:   uint32(1),
				TrunNumber:   uint32(1),
				SampleNumber: uint32(1),
			}) {
				break
			}
		}
	}))
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
		cttsEntry := CTTSEntry{SampleCount: 1, SampleOffset: uint32(sample.CTS)}
		if i == len(track.Samplelist)-1 {
			stts = append(stts, sttsEntry)
		} else {
			var delta uint32 = 1
			if track.Samplelist[i+1].Timestamp >= sample.Timestamp {
				delta = track.Samplelist[i+1].Timestamp - sample.Timestamp
			}

			if len(stts) > 0 && delta == uint32(stts[len(stts)-1].SampleDelta) {
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
	track.STSZ.SampleCount = uint32(len(track.Samplelist))
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
