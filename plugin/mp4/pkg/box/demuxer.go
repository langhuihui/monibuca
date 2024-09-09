package box

import (
	"errors"
	"io"
	"m7s.live/m7s/v5/pkg/util"
)

type AVPacket struct {
	Cid     MP4_CODEC_TYPE
	Data    []byte
	TrackId int
	Pts     uint64
	Dts     uint64
}

type SyncSample struct {
	Pts    uint64
	Dts    uint64
	Size   uint32
	Offset uint32
}

type SubSample struct {
	KID            [16]byte
	IV             [16]byte
	Patterns       []SubSamplePattern
	Number         uint32
	CryptByteBlock uint8
	SkipByteBlock  uint8
	PsshBoxes      []PsshBox
}

type SubSamplePattern struct {
	BytesClear     uint16
	BytesProtected uint32
}

type TrackInfo struct {
	Duration     uint32
	TrackId      int
	Cid          MP4_CODEC_TYPE
	ExtraData    []byte
	Height       uint32
	Width        uint32
	SampleRate   uint32
	SampleSize   uint16
	SampleCount  uint32
	ChannelCount uint8
	Timescale    uint32
	StartDts     uint64
	EndDts       uint64
}

type Mp4Info struct {
	MajorBrand       uint32
	MinorVersion     uint32
	CompatibleBrands []uint32
	Duration         uint32
	Timescale        uint32
	CreateTime       uint64
	ModifyTime       uint64
}

type MovDemuxer struct {
	reader        io.ReadSeeker
	mdatOffset    []uint64 //一个mp4文件可能存在多个mdatbox
	tracks        []*mp4track
	readSampleIdx []uint32
	mp4out        []byte
	mp4Info       Mp4Info

	//for demux fmp4
	isFragement  bool
	currentTrack *mp4track
	pssh         []PsshBox
	moofOffset   int64
	dataOffset   uint32

	OnRawSample func(cid MP4_CODEC_TYPE, sample []byte, subSample *SubSample) error
}

// how to demux mp4 file
// 1. CreateMovDemuxer
// 2. ReadHead()
// 3. ReadPacket

func CreateMp4Demuxer(r io.ReadSeeker) *MovDemuxer {
	return &MovDemuxer{
		reader: r,
	}
}

func (demuxer *MovDemuxer) ReadHead() ([]TrackInfo, error) {
	infos := make([]TrackInfo, 0, 2)
	var err error
	for {
		fullbox := FullBox{}
		basebox := BasicBox{}
		_, err = basebox.Decode(demuxer.reader)
		if err != nil {
			break
		}
		if basebox.Size < BasicBoxLen {
			err = errors.New("mp4 Parser error")
			break
		}
		switch basebox.Type {
		case TypeFTYP:
			err = decodeFtypBox(demuxer, uint32(basebox.Size))
		case TypeFREE:
			err = decodeFreeBox(demuxer, uint32(basebox.Size))
		case TypeMDAT:
			var currentOffset int64
			if currentOffset, err = demuxer.reader.Seek(0, io.SeekCurrent); err != nil {
				break
			}
			demuxer.mdatOffset = append(demuxer.mdatOffset, uint64(currentOffset))
			_, err = demuxer.reader.Seek(int64(basebox.Size)-BasicBoxLen, io.SeekCurrent)
		case TypeMOOV:
			var currentOffset int64
			if currentOffset, err = demuxer.reader.Seek(0, io.SeekCurrent); err != nil {
				break
			}
			offset := int64(0)
			if offset, err = demuxer.reader.Seek(0, io.SeekEnd); err != nil {
				break
			}
			if offset-currentOffset < int64(basebox.Size)-BasicBoxLen {
				err = errors.New("incomplete mp4 file")
				break
			}
			_, err = demuxer.reader.Seek(currentOffset, io.SeekStart)
		case TypeMVHD:
			err = decodeMvhd(demuxer)
		case TypePSSH:
			err = decodePsshBox(demuxer, uint32(basebox.Size))
		case TypeTRAK:
			demuxer.tracks = append(demuxer.tracks, &mp4track{})
		case TypeTKHD:
			err = decodeTkhdBox(demuxer)
		case TypeMDHD:
			err = decodeMdhdBox(demuxer)
		case TypeHDLR:
			err = decodeHdlrBox(demuxer, basebox.Size)
		case TypeMDIA:
		case TypeMINF:
		case TypeVMHD:
			err = decodeVmhdBox(demuxer)
		case TypeSMHD:
			err = decodeSmhdBox(demuxer)
		case TypeHMHD:
			_, err = fullbox.Decode(demuxer.reader)
		case TypeNMHD:
			_, err = fullbox.Decode(demuxer.reader)
		case TypeSTBL:
			demuxer.tracks[len(demuxer.tracks)-1].stbltable = new(movstbl)
		case TypeSTSD:
			err = decodeStsdBox(demuxer)
		case TypeSTTS:
			err = decodeSttsBox(demuxer)
		case TypeCTTS:
			err = decodeCttsBox(demuxer)
		case TypeSTSC:
			err = decodeStscBox(demuxer)
		case TypeSTSZ:
			err = decodeStszBox(demuxer)
		case TypeSTCO:
			err = decodeStcoBox(demuxer)
		case TypeCO64:
			err = decodeCo64Box(demuxer)
		case TypeSTSS:
			err = decodeStssBox(demuxer)
		case TypeENCv:
			err = decodeVisualSampleEntry(demuxer)
		case TypeSINF:
		case TypeFRMA:
			err = decodeFrmaBox(demuxer, uint32(basebox.Size))
		case TypeSCHI:
		case TypeTENC:
			err = decodeTencBox(demuxer, uint32(basebox.Size))
		case TypeAVC1:
			demuxer.tracks[len(demuxer.tracks)-1].cid = MP4_CODEC_H264
			demuxer.tracks[len(demuxer.tracks)-1].extra = new(h264ExtraData)
			err = decodeVisualSampleEntry(demuxer)
		case TypeHVC1, TypeHEV1:
			demuxer.tracks[len(demuxer.tracks)-1].cid = MP4_CODEC_H265
			demuxer.tracks[len(demuxer.tracks)-1].extra = newh265ExtraData()
			err = decodeVisualSampleEntry(demuxer)
		case TypeENCA:
			err = decodeAudioSampleEntry(demuxer)
		case TypeMP4A:
			demuxer.tracks[len(demuxer.tracks)-1].cid = MP4_CODEC_AAC
			demuxer.tracks[len(demuxer.tracks)-1].extra = new(aacExtraData)
			err = decodeAudioSampleEntry(demuxer)
		case TypeULAW:
			demuxer.tracks[len(demuxer.tracks)-1].cid = MP4_CODEC_G711U
			err = decodeAudioSampleEntry(demuxer)
		case TypeALAW:
			demuxer.tracks[len(demuxer.tracks)-1].cid = MP4_CODEC_G711A
			err = decodeAudioSampleEntry(demuxer)
		case TypeOPUS:
			demuxer.tracks[len(demuxer.tracks)-1].cid = MP4_CODEC_OPUS
		case TypeAVCC:
			err = decodeAvccBox(demuxer, uint32(basebox.Size))
		case TypeHVCC:
			err = decodeHvccBox(demuxer, uint32(basebox.Size))
		case TypeESDS:
			err = decodeEsdsBox(demuxer, uint32(basebox.Size))
		case TypeEDTS:
		case TypeELST:
			err = decodeElstBox(demuxer)
		case TypeMVEX:
			demuxer.isFragement = true
		case TypeMOOF:
			if demuxer.moofOffset, err = demuxer.reader.Seek(0, io.SeekCurrent); err != nil {
				break
			}
			demuxer.moofOffset -= 8
			demuxer.dataOffset = uint32(basebox.Size) + 8
		case TypeMFHD:
			err = decodeMfhdBox(demuxer)
		case TypeTRAF:
		case TypeTFHD:
			err = decodeTfhdBox(demuxer, uint32(basebox.Size))
		case TypeTFDT:
			err = decodeTfdtBox(demuxer, uint32(basebox.Size))
		case TypeTRUN:
			err = decodeTrunBox(demuxer, uint32(basebox.Size))
		case TypeSENC:
			err = decodeSencBox(demuxer, uint32(basebox.Size))
		case TypeSAIZ:
			err = decodeSaizBox(demuxer, uint32(basebox.Size))
		case TypeSAIO:
			err = decodeSaioBox(demuxer, uint32(basebox.Size))
		case TypeUUID:
			_, err = demuxer.reader.Seek(int64(basebox.Size)-BasicBoxLen-16, io.SeekCurrent)
		case TypeSGPD:
			err = decodeSgpdBox(demuxer, uint32(basebox.Size))
		case TypeWAVE:
			err = decodeWaveBox(demuxer)
		default:
			_, err = demuxer.reader.Seek(int64(basebox.Size)-BasicBoxLen, io.SeekCurrent)
		}
		if err != nil {
			break
		}
	}
	if err != nil && err != io.EOF {
		return nil, err
	}
	if !demuxer.isFragement {
		demuxer.buildSampleList()
	}
	demuxer.readSampleIdx = make([]uint32, len(demuxer.tracks))
	for _, track := range demuxer.tracks {
		info := TrackInfo{}
		info.Cid = track.cid
		info.Duration = track.duration
		info.ChannelCount = track.chanelCount
		info.SampleRate = track.sampleRate
		info.SampleCount = uint32(len(track.samplelist))
		info.SampleSize = uint16(track.sampleBits)
		info.TrackId = int(track.trackId)
		info.Width = track.width
		info.Height = track.height
		info.Timescale = track.timescale
		switch e := track.extra.(type) {
		case *h264ExtraData:
			//data, err := h264parser.NewCodecDataFromSPSAndPPS(e.spss[0], e.ppss[0])
			//if err != nil {
			//	return nil, err
			//}
			//info.ExtraData = data.Record
			info.ExtraData = e.export()
		case *h265ExtraData:
			info.ExtraData = e.export()
		case *aacExtraData:
			info.ExtraData = e.export()
		}
		if len(track.samplelist) > 0 {
			info.StartDts = track.samplelist[0].dts * 1000 / uint64(track.timescale)
			info.EndDts = track.samplelist[len(track.samplelist)-1].dts * 1000 / uint64(track.timescale)
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (demuxer *MovDemuxer) GetMp4Info() Mp4Info {
	return demuxer.mp4Info
}

// /return error == io.EOF, means read mp4 file completed
func (demuxer *MovDemuxer) ReadPacket(allocator *util.ScalableMemoryAllocator) (*AVPacket, error) {
	for {
		maxdts := int64(-1)
		minTsSample := sampleEntry{dts: uint64(maxdts)}
		var (
			subSample  *SubSample = nil
			whichTrack *mp4track  = nil
		)
		whichTracki := 0
		for i, track := range demuxer.tracks {
			idx := demuxer.readSampleIdx[i]
			if int(idx) == len(track.samplelist) {
				continue
			}
			if whichTrack == nil {
				minTsSample = track.samplelist[idx]
				whichTrack = track
				whichTracki = i
			} else {
				dts1 := minTsSample.dts * uint64(demuxer.mp4Info.Timescale) / uint64(whichTrack.timescale)
				dts2 := track.samplelist[idx].dts * uint64(demuxer.mp4Info.Timescale) / uint64(track.timescale)
				if dts1 > dts2 {
					minTsSample = track.samplelist[idx]
					whichTrack = track
					whichTracki = i
				}
			}
			if int(idx) < len(track.subSamples) {
				subSample = new(SubSample)
				subSample.Number = idx
				if len(track.subSamples[idx].iv) > 0 {
					copy(subSample.IV[:], track.subSamples[idx].iv)
				} else {
					copy(subSample.IV[:], track.defaultConstantIV)
				}
				if track.lastSeig != nil {
					copy(subSample.KID[:], track.lastSeig.KID[:])
					subSample.CryptByteBlock = track.lastSeig.CryptByteBlock
					subSample.SkipByteBlock = track.lastSeig.SkipByteBlock
				} else {
					copy(subSample.KID[:], track.defaultKID[:])
					subSample.CryptByteBlock = track.defaultCryptByteBlock
					subSample.SkipByteBlock = track.defaultSkipByteBlock
				}
				subSample.PsshBoxes = append(subSample.PsshBoxes, demuxer.pssh...)
				if len(track.subSamples[idx].subSamples) > 0 {
					subSample.Patterns = make([]SubSamplePattern, len(track.subSamples[idx].subSamples))
					for ei, e := range track.subSamples[idx].subSamples {
						subSample.Patterns[ei].BytesClear = e.bytesOfClearData
						subSample.Patterns[ei].BytesProtected = e.bytesOfProtectedData
					}
				}
			}
		}

		if minTsSample.dts == uint64(maxdts) {
			return nil, io.EOF
		}
		if _, err := demuxer.reader.Seek(int64(minTsSample.offset), io.SeekStart); err != nil {
			return nil, err
		}
		//sample := make([]byte, minTsSample.size)
		sample := allocator.Malloc(int(minTsSample.size))
		if _, err := io.ReadFull(demuxer.reader, sample); err != nil {
			return nil, err
		}
		demuxer.readSampleIdx[whichTracki]++
		avpkg := &AVPacket{
			Cid:     whichTrack.cid,
			TrackId: int(whichTrack.trackId),
			Pts:     minTsSample.pts * 1000 / uint64(whichTrack.timescale),
			Dts:     minTsSample.dts * 1000 / uint64(whichTrack.timescale),
			Data:    sample,
		}
		if demuxer.OnRawSample != nil {
			err := demuxer.OnRawSample(whichTrack.cid, sample, subSample)
			if err != nil {
				return nil, err
			}
		}
		if len(avpkg.Data) > 0 {
			return avpkg, nil
		}
	}
}

func (demuxer *MovDemuxer) GetSyncTable(trackId uint32) ([]SyncSample, error) {
	var track *mp4track = nil
	for i := 0; i < len(demuxer.tracks); i++ {
		if demuxer.tracks[i].trackId != trackId {
			continue
		}
		track = demuxer.tracks[i]
	}
	if track == nil {
		return nil, errors.New("not found track")
	}

	if track.stbltable == nil || track.stbltable.stss == nil {
		return nil, errors.New("not found stss box")
	}

	syncTable := make([]SyncSample, len(track.stbltable.stss.sampleNumber))

	for i := 0; i < len(syncTable); i++ {
		idx := track.stbltable.stss.sampleNumber[i] - 1
		syncTable[i] = SyncSample{
			Pts:    track.samplelist[idx].pts * 1000 / uint64(track.timescale),
			Dts:    track.samplelist[idx].dts * 1000 / uint64(track.timescale),
			Offset: uint32(track.samplelist[idx].offset),
			Size:   uint32(track.samplelist[idx].size),
		}
	}
	return syncTable, nil
}

func (demuxer *MovDemuxer) SeekTime(dts uint64) error {
	for i, track := range demuxer.tracks {
		for j := 0; j < len(track.samplelist); j++ {
			if track.samplelist[j].dts*1000/uint64(track.timescale) < dts {
				continue
			}
			demuxer.readSampleIdx[i] = uint32(j)
			break
		}
	}
	return nil
}

func (demuxer *MovDemuxer) buildSampleList() {
	for _, track := range demuxer.tracks {
		stbl := track.stbltable
		chunks := make([]movchunk, stbl.stco.entryCount)
		iterator := 0
		for i := 0; i < int(stbl.stco.entryCount); i++ {
			chunks[i].chunknum = uint32(i + 1)
			chunks[i].chunkoffset = stbl.stco.chunkOffsetlist[i]
			for iterator+1 < int(stbl.stsc.entryCount) && stbl.stsc.entrys[iterator+1].firstChunk <= chunks[i].chunknum {
				iterator++
			}
			chunks[i].samplenum = stbl.stsc.entrys[iterator].samplesPerChunk
		}
		track.samplelist = make([]sampleEntry, stbl.stsz.sampleCount)
		for i := range track.samplelist {
			if stbl.stsz.sampleSize == 0 {
				track.samplelist[i].size = uint64(stbl.stsz.entrySizelist[i])
			} else {
				track.samplelist[i].size = uint64(stbl.stsz.sampleSize)
			}
		}
		iterator = 0
		for i := range chunks {
			for j := 0; j < int(chunks[i].samplenum); j++ {
				if iterator >= len(track.samplelist) {
					break
				}
				if j == 0 {
					track.samplelist[iterator].offset = chunks[i].chunkoffset
				} else {
					track.samplelist[iterator].offset = track.samplelist[iterator-1].offset + track.samplelist[iterator-1].size
				}
				iterator++
			}
		}
		iterator = 0
		track.samplelist[iterator].dts = 0
		if track.elst != nil {
			for _, entry := range track.elst.entrys {
				if entry.mediaTime == -1 {
					track.samplelist[iterator].dts = entry.segmentDuration
				}
			}
		}
		iterator++
		for i := range stbl.stts.entrys {
			for j := 0; j < int(stbl.stts.entrys[i].sampleCount); j++ {
				if iterator == len(track.samplelist) {
					break
				}
				track.samplelist[iterator].dts = uint64(stbl.stts.entrys[i].sampleDelta) + track.samplelist[iterator-1].dts
				iterator++
			}
		}

		// no ctts table, so pts == dts
		if stbl.ctts == nil || stbl.ctts.entryCount == 0 {
			for i := range track.samplelist {
				track.samplelist[i].pts = track.samplelist[i].dts
			}
		} else {
			iterator = 0
			for i := range stbl.ctts.entrys {
				for j := 0; j < int(stbl.ctts.entrys[i].sampleCount); j++ {
					track.samplelist[iterator].pts = track.samplelist[iterator].dts + uint64(stbl.ctts.entrys[i].sampleOffset)
					iterator++
				}
			}
		}
	}
}
