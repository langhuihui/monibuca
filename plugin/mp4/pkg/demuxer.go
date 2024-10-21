package mp4

import (
	"encoding/binary"
	"errors"
	"io"
	"slices"

	. "m7s.live/v5/plugin/mp4/pkg/box"
)

type (
	AVPacket struct {
		Track *Track
		Data  []byte
		Pts   uint64
		Dts   uint64
	}
	SyncSample struct {
		Pts    uint64
		Dts    uint64
		Size   uint32
		Offset uint32
	}
	SubSample struct {
		KID            [16]byte
		IV             [16]byte
		Patterns       []SubSamplePattern
		Number         uint32
		CryptByteBlock uint8
		SkipByteBlock  uint8
		PsshBoxes      []*PsshBox
	}
	SubSamplePattern struct {
		BytesClear     uint16
		BytesProtected uint32
	}
	Info struct {
		MajorBrand       [4]byte
		MinorVersion     uint32
		CompatibleBrands [][4]byte
		Duration         uint32
		Timescale        uint32
		CreateTime       uint64
		ModifyTime       uint64
	}

	movchunk struct {
		chunknum    uint32
		samplenum   uint32
		chunkoffset uint64
	}

	Demuxer struct {
		Info
		reader        io.ReadSeeker
		mdatOffset    []uint64
		Tracks        []*Track
		ReadSampleIdx []uint32
		IsFragment    bool
		currentTrack  *Track
		pssh          []*PsshBox
		moofOffset    int64
		dataOffset    uint32
	}
)

func NewDemuxer(r io.ReadSeeker) *Demuxer {
	return &Demuxer{
		reader: r,
	}
}

func (d *Demuxer) Demux() (err error) {
	var offset int64
	var lastTrack *Track
	decodeVisualSampleEntry := func() (offset int, err error) {
		var encv VisualSampleEntry
		encv.SampleEntry = new(SampleEntry)
		_, err = encv.Decode(d.reader)
		offset = int(encv.Size() - BasicBoxLen)
		lastTrack.Width = uint32(encv.Width)
		lastTrack.Height = uint32(encv.Height)
		return
	}
	decodeAudioSampleEntry := func() (offset int, err error) {
		var enca AudioSampleEntry
		enca.SampleEntry = new(SampleEntry)
		_, err = enca.Decode(d.reader)
		lastTrack.ChannelCount = uint8(enca.ChannelCount)
		lastTrack.SampleSize = enca.SampleSize
		lastTrack.SampleRate = enca.Samplerate
		offset = int(enca.Size() - BasicBoxLen)
		if slices.Contains(d.Info.CompatibleBrands, [4]byte{'q', 't', ' ', ' '}) {
			if enca.Version == 1 {
				if _, err = io.ReadFull(d.reader, make([]byte, 16)); err != nil {
					return
				}
				offset += 16
			} else if enca.Version == 2 {
				if _, err = io.ReadFull(d.reader, make([]byte, 36)); err != nil {
					return
				}
				offset += 36
			}
		}
		return
	}

	for {
		var basebox BasicBox
		basebox.Offset = offset
		var headerSize, contentSize int
		var hasChild bool
		headerSize, err = basebox.Decode(d.reader)
		if err != nil {
			break
		}
		nextChildBoxOffset := offset + int64(headerSize)
		offset = offset + int64(basebox.Size)
		// if basebox.Size < BasicBoxLen {
		// 	err = errors.New("mp4 Parser error")
		// 	break
		// }
		switch basebox.Type {
		case TypeFTYP:
			var ftyp FileTypeBox
			ftyp.Decode(d.reader, &basebox)
			d.MajorBrand = ftyp.Major_brand
			d.MinorVersion = ftyp.Minor_version
			d.CompatibleBrands = ftyp.Compatible_brands
		case TypeFREE:
			var free FreeBox
			free.Decode(d.reader, &basebox)
		case TypeMOOV, TypeMDIA, TypeMINF, TypeSTBL:
			hasChild = true
		case TypeMVHD:
			var mvhd MovieHeaderBox
			mvhd.Decode(d.reader, &basebox)
			d.CreateTime = mvhd.Creation_time
			d.ModifyTime = mvhd.Modification_time
			d.Duration = uint32(mvhd.Duration)
			d.Timescale = mvhd.Timescale
		case TypeMDAT:
			d.mdatOffset = append(d.mdatOffset, uint64(basebox.Offset+BasicBoxLen))
			if _, err = d.reader.Seek(offset, io.SeekStart); err != nil {
				return
			}
		case TypePSSH:
			var pssh PsshBox
			pssh.Decode(d.reader, &basebox)
			d.pssh = append(d.pssh, &pssh)
		case TypeTRAK:
			lastTrack = &Track{}
			d.Tracks = append(d.Tracks, lastTrack)
			hasChild = true
		case TypeTKHD:
			var tkhd TrackHeaderBox
			tkhd.Decode(d.reader)
			lastTrack.TrackId = tkhd.Track_ID
			lastTrack.Duration = uint32(tkhd.Duration)
		case TypeMDHD:
			var mdhd MediaHeaderBox
			mdhd.Decode(d.reader)
			lastTrack.Timescale = mdhd.Timescale
		case TypeHDLR:
			var hdlr HandlerBox
			if _, err = hdlr.Decode(d.reader, basebox.Size); err != nil {
				return
			}
		case TypeVMHD:
			var vmhd VideoMediaHeaderBox
			vmhd.Decode(d.reader)
		case TypeSMHD:
			var smhd SoundMediaHeaderBox
			smhd.Decode(d.reader)
		case TypeHMHD:
			var hmhd HintMediaHeaderBox
			hmhd.Decode(d.reader)
		case TypeNMHD:
			var fullbox FullBox
			fullbox.Decode(d.reader)
		case TypeSTSD:
			var stsd SampleDescriptionBox
			if contentSize, err = stsd.Decode(d.reader); err != nil {
				return
			}
			hasChild = true
		case TypeSTTS:
			var stts TimeToSampleBox
			stts.Decode(d.reader)
			lastTrack.SampleTable.STTS = &stts
		case TypeCTTS:
			var ctts CompositionOffsetBox
			ctts.Decode(d.reader)
			lastTrack.SampleTable.CTTS = &ctts
		case TypeSTSC:
			var stsc SampleToChunkBox
			stsc.Decode(d.reader)
			lastTrack.SampleTable.STSC = &stsc
		case TypeSTSZ:
			var stsz SampleSizeBox
			stsz.Decode(d.reader)
			lastTrack.SampleTable.STSZ = &stsz
		case TypeSTCO:
			var stco ChunkOffsetBox
			stco.Decode(d.reader)
			lastTrack.SampleTable.STCO = &stco
		case TypeCO64:
			var co64 ChunkLargeOffsetBox
			co64.Decode(d.reader)
			lastTrack.SampleTable.STCO = (*ChunkOffsetBox)(&co64)
		case TypeSTSS:
			var stss SyncSampleBox
			stss.Decode(d.reader)
			lastTrack.SampleTable.STSS = &stss
		case TypeENCV:
			if contentSize, err = decodeVisualSampleEntry(); err != nil {
				return
			}
		case TypeFRMA:
			buf := make([]byte, basebox.Size-BasicBoxLen)
			if _, err = io.ReadFull(d.reader, buf); err != nil {
				return
			}
			switch [4]byte(buf) {
			case TypeAVC1:
				lastTrack.Cid = MP4_CODEC_H264
				return
			case TypeMP4A:
				lastTrack.Cid = MP4_CODEC_AAC
				return
			}
		case TypeTENC:
			buf := make([]byte, basebox.Size-BasicBoxLen)
			if _, err = io.ReadFull(d.reader, buf); err != nil {
				return
			}
			n := 0
			versionAndFlags := binary.BigEndian.Uint32(buf[n:])
			n += 5
			version := byte(versionAndFlags >> 24)
			track := lastTrack
			if version != 0 {
				infoByte := buf[n]
				track.defaultCryptByteBlock = infoByte >> 4
				track.defaultSkipByteBlock = infoByte & 0x0f
			}
			n += 1
			track.defaultIsProtected = buf[n]
			n += 1
			track.defaultPerSampleIVSize = buf[n]
			n += 1
			copy(track.defaultKID[:], buf[n:n+16])
			n += 16
			if track.defaultIsProtected == 1 && track.defaultPerSampleIVSize == 0 {
				defaultConstantIVSize := int(buf[n])
				n += 1
				track.defaultConstantIV = make([]byte, defaultConstantIVSize)
				copy(track.defaultConstantIV, buf[n:n+defaultConstantIVSize])
			}
		case TypeAVC1:
			lastTrack.Cid = MP4_CODEC_H264
			if contentSize, err = decodeVisualSampleEntry(); err != nil {
				return
			}
			hasChild = true
		case TypeHVC1, TypeHEV1:
			lastTrack.Cid = MP4_CODEC_H265
			if contentSize, err = decodeVisualSampleEntry(); err != nil {
				return
			}
			hasChild = true
		case TypeENCA:
			if contentSize, err = decodeAudioSampleEntry(); err != nil {
				return
			}
			hasChild = true
		case TypeMP4A:
			lastTrack.Cid = MP4_CODEC_AAC
			if contentSize, err = decodeAudioSampleEntry(); err != nil {
				return
			}
			hasChild = true
		case TypeULAW:
			lastTrack.Cid = MP4_CODEC_G711U
			if contentSize, err = decodeAudioSampleEntry(); err != nil {
				return
			}
			hasChild = true
		case TypeALAW:
			lastTrack.Cid = MP4_CODEC_G711A
			if contentSize, err = decodeAudioSampleEntry(); err != nil {
				return
			}
			hasChild = true
		case TypeOPUS:
			lastTrack.Cid = MP4_CODEC_OPUS
			if contentSize, err = decodeAudioSampleEntry(); err != nil {
				return
			}
			hasChild = true
		case TypeAVCC:
			lastTrack.ExtraData = make([]byte, basebox.Size-BasicBoxLen)
			if _, err = io.ReadFull(d.reader, lastTrack.ExtraData); err != nil {
				return
			}
		case TypeHVCC:
			lastTrack.ExtraData = make([]byte, basebox.Size-BasicBoxLen)
			if _, err = io.ReadFull(d.reader, lastTrack.ExtraData); err != nil {
				return
			}
		case TypeESDS:
			var fullbox FullBox
			fullbox.Decode(d.reader)
			esds := make([]byte, basebox.Size-FullBoxLen)
			if _, err = io.ReadFull(d.reader, esds); err != nil {
				return
			}
			//TODO: check cid changed
			lastTrack.Cid, lastTrack.ExtraData = DecodeESDescriptor(esds)
		case TypeEDTS:
			hasChild = true
		case TypeELST:
			var elst EditListBox
			elst.Decode(d.reader)
			lastTrack.ELST = &elst
		case TypeMVEX:
			d.IsFragment = true
		case TypeMOOF:
			d.moofOffset = basebox.Offset
			d.dataOffset = uint32(basebox.Size) + 8
			hasChild = true
		case TypeMFHD:
			var mfhd MovieFragmentHeaderBox
			mfhd.Decode(d.reader)
		case TypeTRAF:
			hasChild = true
		case TypeTFHD:
			var tfhd TrackFragmentHeaderBox
			tfhd.Decode(d.reader, uint32(basebox.Size), uint64(d.moofOffset))
			for i := 0; i < len(d.Tracks); i++ {
				if d.Tracks[i].TrackId != tfhd.Track_ID {
					continue
				}
				d.currentTrack = d.Tracks[i]
				d.Tracks[i].defaultDuration = tfhd.DefaultSampleDuration
				d.Tracks[i].defaultSize = tfhd.DefaultSampleSize
				d.Tracks[i].baseDataOffset = tfhd.BaseDataOffset
			}
		case TypeTFDT:
			var tfdt TrackFragmentBaseMediaDecodeTimeBox
			tfdt.Decode(d.reader, uint32(basebox.Size))
			d.currentTrack.StartDts = tfdt.BaseMediaDecodeTime
		case TypeTRUN:
			var trun TrackRunBox
			trun.Decode(d.reader, uint32(basebox.Size), uint32(d.dataOffset))
			d.decodeTRUN(&trun)
		case TypeSENC:
			var senc SencBox
			senc.Decode(d.reader, uint32(basebox.Size), lastTrack.defaultPerSampleIVSize)
			d.currentTrack.subSamples = append(d.currentTrack.subSamples, senc.EntryList...)
		case TypeSAIZ:
			var saiz SaizBox
			saiz.Decode(d.reader, uint32(basebox.Size))
			d.currentTrack.lastSaiz = &saiz
		case TypeSAIO:
			var saio SaioBox
			saio.Decode(d.reader, uint32(basebox.Size))
			if err = d.decodeSaioBox(&saio); err != nil {
				return
			}
		case TypeUUID:
			var uuid [16]byte
			if _, err = io.ReadFull(d.reader, uuid[:]); err != nil {
				return
			}
			// TODO
			// _, err = d.reader.Seek(int64(basebox.Size)-BasicBoxLen-16, io.SeekCurrent)
		case TypeSGPD:
			if err = d.decodeSgpdBox(uint32(basebox.Size)); err != nil {
				return
			}
		case TypeWAVE:
			if _, err = io.ReadFull(d.reader, make([]byte, 24)); err != nil {
				return
			}
		default:
			if basebox.Size != BasicBoxLen {
				_, err = d.reader.Seek(int64(basebox.Size)-BasicBoxLen, io.SeekCurrent)
				if err != nil {
					return
				}
			}
		}
		if hasChild {
			nextChildBoxOffset += int64(contentSize)
			offset = nextChildBoxOffset
		}
		//	n, err := d.reader.Seek(0, io.SeekCurrent)
		//	if err != nil {
		//		panic(err)
		//	}
		//	if n != offset {
		//		panic("seek error")
		//	}
		//}
	}
	if err != io.EOF {
		return err
	}
	if !d.IsFragment {
		d.buildSampleList()
	}
	d.ReadSampleIdx = make([]uint32, len(d.Tracks))
	for _, track := range d.Tracks {
		if len(track.Samplelist) > 0 {
			track.StartDts = uint64(track.Samplelist[0].DTS) * 1000 / uint64(track.Timescale)
			track.EndDts = uint64(track.Samplelist[len(track.Samplelist)-1].DTS) * 1000 / uint64(track.Timescale)
		}
	}
	return nil
}

func (d *Demuxer) SeekTime(dts uint64) (sample *Sample, err error) {
	var audioTrack, videoTrack *Track
	for _, track := range d.Tracks {
		if track.Cid.IsAudio() {
			audioTrack = track
		} else if track.Cid.IsVideo() {
			videoTrack = track
		}
	}
	if videoTrack != nil {
		idx := videoTrack.Seek(dts)
		if idx == -1 {
			return nil, errors.New("seek failed")
		}
		d.ReadSampleIdx[videoTrack.TrackId-1] = uint32(idx)
		sample = &videoTrack.Samplelist[idx]
		if audioTrack != nil {
			for i, sample := range audioTrack.Samplelist {
				if sample.Offset < int64(videoTrack.Samplelist[idx].Offset) {
					continue
				}
				d.ReadSampleIdx[audioTrack.TrackId-1] = uint32(i)
				break
			}
		}
	} else if audioTrack != nil {
		idx := audioTrack.Seek(dts)
		if idx == -1 {
			return nil, errors.New("seek failed")
		}
		d.ReadSampleIdx[audioTrack.TrackId-1] = uint32(idx)
		sample = &audioTrack.Samplelist[idx]
	}
	return
}

func (d *Demuxer) buildSampleList() {
	for _, track := range d.Tracks {
		stbl := &track.SampleTable
		l := len(*stbl.STCO)
		chunks := make([]movchunk, l)
		iterator := 0
		for i := 0; i < l; i++ {
			chunks[i].chunknum = uint32(i + 1)
			chunks[i].chunkoffset = (*stbl.STCO)[i]
			for iterator+1 < len(*stbl.STSC) && (*stbl.STSC)[iterator+1].FirstChunk <= chunks[i].chunknum {
				iterator++
			}
			chunks[i].samplenum = (*stbl.STSC)[iterator].SamplesPerChunk
		}
		track.Samplelist = make([]Sample, stbl.STSZ.SampleCount)
		for i := range track.Samplelist {
			if stbl.STSZ.SampleSize == 0 {
				track.Samplelist[i].Size = int(stbl.STSZ.EntrySizelist[i])
			} else {
				track.Samplelist[i].Size = int(stbl.STSZ.SampleSize)
			}
		}
		iterator = 0
		for i := range chunks {
			for j := 0; j < int(chunks[i].samplenum); j++ {
				if iterator >= len(track.Samplelist) {
					break
				}
				if j == 0 {
					track.Samplelist[iterator].Offset = int64(chunks[i].chunkoffset)
				} else {
					track.Samplelist[iterator].Offset = track.Samplelist[iterator-1].Offset + int64(track.Samplelist[iterator-1].Size)
				}
				iterator++
			}
		}
		iterator = 0
		track.Samplelist[iterator].DTS = 0
		if track.ELST != nil {
			for _, entry := range track.ELST.Entrys {
				if entry.MediaTime == -1 {
					track.Samplelist[iterator].DTS = (entry.SegmentDuration)
				}
			}
		}
		iterator++
		for _, entry := range *stbl.STTS {
			for j := 0; j < int(entry.SampleCount); j++ {
				if iterator == len(track.Samplelist) {
					break
				}
				track.Samplelist[iterator].DTS = uint64(entry.SampleDelta) + track.Samplelist[iterator-1].DTS
				iterator++
			}
		}

		// no ctts table, so pts == dts
		if stbl.CTTS == nil || len(*stbl.CTTS) == 0 {
			for i := range track.Samplelist {
				track.Samplelist[i].PTS = track.Samplelist[i].DTS
			}
		} else {
			iterator = 0
			for i := range *stbl.CTTS {
				for j := 0; j < int((*stbl.CTTS)[i].SampleCount); j++ {
					track.Samplelist[iterator].PTS = (track.Samplelist[iterator].DTS) + uint64((*stbl.CTTS)[i].SampleOffset)
					iterator++
				}
			}
		}
		if stbl.STSS != nil {
			for _, keyIndex := range *stbl.STSS {
				track.Samplelist[keyIndex-1].KeyFrame = true
			}
		}
	}
}

func (d *Demuxer) decodeTRUN(trun *TrackRunBox) {
	dataOffset := trun.Dataoffset
	nextDts := d.currentTrack.StartDts
	delta := 0
	var cts int64 = 0
	for _, entry := range trun.EntryList {
		sample := Sample{}
		sample.Offset = int64(dataOffset) + int64(d.currentTrack.baseDataOffset)
		sample.DTS = (nextDts)
		if entry.SampleSize == 0 {
			dataOffset += int32(d.currentTrack.defaultSize)
			sample.Size = int(d.currentTrack.defaultSize)
		} else {
			dataOffset += int32(entry.SampleSize)
			sample.Size = int(entry.SampleSize)
		}

		if entry.SampleDuration == 0 {
			delta = int(d.currentTrack.defaultDuration)
		} else {
			delta = int(entry.SampleDuration)
		}
		cts = int64(entry.SampleCompositionTimeOffset)
		sample.PTS = uint64(int64(sample.DTS) + cts)
		nextDts += uint64(delta)
		d.currentTrack.Samplelist = append(d.currentTrack.Samplelist, sample)
	}
	d.dataOffset = uint32(dataOffset)
}

func (d *Demuxer) decodeSaioBox(saio *SaioBox) (err error) {
	if len(saio.Offset) > 0 && len(d.currentTrack.subSamples) == 0 {
		var currentOffset int64
		currentOffset, err = d.reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		d.reader.Seek(d.moofOffset+saio.Offset[0], io.SeekStart)
		saiz := d.currentTrack.lastSaiz
		for i := uint32(0); i < saiz.SampleCount; i++ {
			sampleSize := saiz.DefaultSampleInfoSize
			if saiz.DefaultSampleInfoSize == 0 {
				sampleSize = saiz.SampleInfo[i]
			}
			buf := make([]byte, sampleSize)
			d.reader.Read(buf)
			var se SencEntry
			se.IV = make([]byte, 16)
			copy(se.IV, buf[:8])
			if sampleSize == 8 {
				d.currentTrack.subSamples = append(d.currentTrack.subSamples, se)
				continue
			}
			n := 8
			sampleCount := binary.BigEndian.Uint16(buf[n:])
			n += 2

			se.SubSamples = make([]SubSampleEntry, sampleCount)
			for j := 0; j < int(sampleCount); j++ {
				se.SubSamples[j].BytesOfClearData = binary.BigEndian.Uint16(buf[n:])
				n += 2
				se.SubSamples[j].BytesOfProtectedData = binary.BigEndian.Uint32(buf[n:])
				n += 4
			}
			d.currentTrack.subSamples = append(d.currentTrack.subSamples, se)
		}
		d.reader.Seek(currentOffset, io.SeekStart)
	}
	return nil
}

func (d *Demuxer) decodeSgpdBox(size uint32) (err error) {
	buf := make([]byte, size-BasicBoxLen)
	if _, err = io.ReadFull(d.reader, buf); err != nil {
		return
	}
	n := 0
	versionAndFlags := binary.BigEndian.Uint32(buf[n:])
	n += 4
	version := byte(versionAndFlags >> 24)

	b := &SgpdBox{
		Version: version,
		Flags:   versionAndFlags & 0x00ffffff,
	}
	b.GroupingType = string(buf[n : n+4])
	n += 4

	if b.Version >= 1 {
		b.DefaultLength = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}
	if b.Version >= 2 {
		b.DefaultGroupDescriptionIndex = binary.BigEndian.Uint32(buf[n:])
		n += 4
	}
	entryCount := int(binary.BigEndian.Uint32(buf[n:]))
	n += 4

	track := d.Tracks[len(d.Tracks)-1]
	for i := 0; i < entryCount; i++ {
		var descriptionLength = b.DefaultLength
		if b.Version >= 1 && b.DefaultLength == 0 {
			descriptionLength = binary.BigEndian.Uint32(buf[n:])
			n += 4
			b.DescriptionLengths = append(b.DescriptionLengths, descriptionLength)
		}
		var (
			sgEntry interface{}
			offset  int
		)
		sgEntry, offset, err = DecodeSampleGroupEntry(b.GroupingType, descriptionLength, buf[n:])
		n += offset
		if err != nil {
			return err
		}
		if sgEntry == nil {
			continue
		}
		if seig, ok := sgEntry.(*SeigSampleGroupEntry); ok {
			track.lastSeig = seig
		}
		b.SampleGroupEntries = append(b.SampleGroupEntries, sgEntry)
	}

	return nil
}

func (d *Demuxer) readSubSample(idx uint32, track *Track) (subSample *SubSample) {
	if int(idx) < len(track.subSamples) {
		subSample = new(SubSample)
		subSample.Number = idx
		if len(track.subSamples[idx].IV) > 0 {
			copy(subSample.IV[:], track.subSamples[idx].IV)
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
		subSample.PsshBoxes = append(subSample.PsshBoxes, d.pssh...)
		if len(track.subSamples[idx].SubSamples) > 0 {
			subSample.Patterns = make([]SubSamplePattern, len(track.subSamples[idx].SubSamples))
			for ei, e := range track.subSamples[idx].SubSamples {
				subSample.Patterns[ei].BytesClear = e.BytesOfClearData
				subSample.Patterns[ei].BytesProtected = e.BytesOfProtectedData
			}
		}
		return subSample
	}
	return nil
}

func (d *Demuxer) ReadSample(yield func(*Track, Sample) bool) {
	for {
		maxdts := int64(-1)
		minTsSample := Sample{DTS: uint64(maxdts)}
		var whichTrack *Track
		whichTracki := 0
		for i, track := range d.Tracks {
			idx := d.ReadSampleIdx[i]
			if int(idx) == len(track.Samplelist) {
				continue
			}
			if whichTrack == nil {
				minTsSample = track.Samplelist[idx]
				whichTrack = track
				whichTracki = i
			} else {
				dts1 := minTsSample.DTS * uint64(d.Timescale) / uint64(whichTrack.Timescale)
				dts2 := track.Samplelist[idx].DTS * uint64(d.Timescale) / uint64(track.Timescale)
				if dts1 > dts2 {
					minTsSample = track.Samplelist[idx]
					whichTrack = track
					whichTracki = i
				}
			}
			// subSample := d.readSubSample(idx, whichTrack)
		}
		if minTsSample.DTS == uint64(maxdts) {
			return
		}

		d.ReadSampleIdx[whichTracki]++
		if !yield(whichTrack, minTsSample) {
			return
		}
	}
}

func (d *Demuxer) RangeSample(yield func(*Track, *Sample) bool) {
	for {
		var minTsSample *Sample
		var whichTrack *Track
		whichTracki := 0
		for i, track := range d.Tracks {
			idx := d.ReadSampleIdx[i]
			if int(idx) == len(track.Samplelist) {
				continue
			}
			if whichTrack == nil {
				minTsSample = &track.Samplelist[idx]
				whichTrack = track
				whichTracki = i
			} else {
				if minTsSample.Offset > track.Samplelist[idx].Offset {
					minTsSample = &track.Samplelist[idx]
					whichTrack = track
					whichTracki = i
				}
			}
			// subSample := d.readSubSample(idx, whichTrack)
		}
		if minTsSample == nil {
			return
		}
		d.ReadSampleIdx[whichTracki]++
		if !yield(whichTrack, minTsSample) {
			return
		}
	}
}
