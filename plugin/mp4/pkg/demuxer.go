package mp4

import (
	"errors"
	"io"
	"slices"

	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/plugin/mp4/pkg/box"
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

	movchunk struct {
		chunknum    uint32
		samplenum   uint32
		chunkoffset uint64
	}

	Demuxer struct {
		reader        io.ReadSeeker
		Tracks        []*Track
		ReadSampleIdx []uint32
		IsFragment    bool
		// pssh          []*PsshBox
		moov       *MoovBox
		mdat       *MediaDataBox
		mdatOffset uint64
		QuicTime   bool
	}
)

func NewDemuxer(r io.ReadSeeker) *Demuxer {
	return &Demuxer{
		reader: r,
	}
}

func (d *Demuxer) Demux() (err error) {

	// decodeVisualSampleEntry := func() (offset int, err error) {
	// 	var encv VisualSampleEntry
	// 	encv.SampleEntry = new(SampleEntry)
	// 	_, err = encv.Decode(d.reader)
	// 	offset = int(encv.Size() - BasicBoxLen)
	// 	lastTrack.Width = uint32(encv.Width)
	// 	lastTrack.Height = uint32(encv.Height)
	// 	return
	// }
	// decodeAudioSampleEntry := func() (offset int, err error) {
	// 	var enca AudioSampleEntry
	// 	enca.SampleEntry = new(SampleEntry)
	// 	_, err = enca.Decode(d.reader)
	// 	lastTrack.ChannelCount = uint8(enca.ChannelCount)
	// 	lastTrack.SampleSize = enca.SampleSize
	// 	lastTrack.SampleRate = enca.Samplerate
	// 	offset = int(enca.Size() - BasicBoxLen)
	// 	if slices.Contains(d.Info.CompatibleBrands, [4]byte{'q', 't', ' ', ' '}) {
	// 		if enca.Version == 1 {
	// 			if _, err = io.ReadFull(d.reader, make([]byte, 16)); err != nil {
	// 				return
	// 			}
	// 			offset += 16
	// 		} else if enca.Version == 2 {
	// 			if _, err = io.ReadFull(d.reader, make([]byte, 36)); err != nil {
	// 				return
	// 			}
	// 			offset += 36
	// 		}
	// 	}
	// 	return
	// }
	var b IBox
	var offset uint64
	for {
		b, err = box.ReadFrom(d.reader)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		offset += b.Size()
		switch box := b.(type) {
		case *FileTypeBox:
			if slices.Contains(box.CompatibleBrands, [4]byte{'q', 't', ' ', ' '}) {
				d.QuicTime = true
			}
		case *FreeBox:
		case *MediaDataBox:
			d.mdat = box
			d.mdatOffset = offset - b.Size() + uint64(box.HeaderSize())
		case *MoovBox:
			if box.MVEX != nil {
				d.IsFragment = true
			}
			for _, trak := range box.Tracks {
				track := &Track{}
				track.TrackId = trak.TKHD.TrackID
				track.Duration = uint32(trak.TKHD.Duration)
				track.Timescale = trak.MDIA.MDHD.Timescale
				track.Samplelist = trak.ParseSamples()
				if len(trak.MDIA.MINF.STBL.STSD.Entries) > 0 {
					entryBox := trak.MDIA.MINF.STBL.STSD.Entries[0]
					switch entry := entryBox.(type) {
					case *AudioSampleEntry:
						switch entry.Type() {
						case TypeMP4A:
							track.Cid = MP4_CODEC_AAC
							switch extra := entry.ExtraData.(type) {
							case *ESDSBox:
								var extraData []byte
								track.Cid, extraData = DecodeESDescriptor(extra.Data)
								if aacCtx, err := codec.NewAACCtxFromRecord(extraData); err == nil {
									track.ICodecCtx = aacCtx
								}
							}
						case TypeALAW:
							track.Cid = MP4_CODEC_G711A
							track.ICodecCtx = &codec.PCMACtx{
								AudioCtx: codec.AudioCtx{
									SampleRate: int(entry.Samplerate),
									Channels:   int(entry.ChannelCount),
									SampleSize: int(entry.SampleSize),
								},
							}
						case TypeULAW:
							track.Cid = MP4_CODEC_G711U
							track.ICodecCtx = &codec.PCMUCtx{
								AudioCtx: codec.AudioCtx{
									SampleRate: int(entry.Samplerate),
									Channels:   int(entry.ChannelCount),
									SampleSize: int(entry.SampleSize),
								},
							}
						case TypeOPUS:
							track.Cid = MP4_CODEC_OPUS
							// TODO: 需要实现 OPUS 的 codec context 创建
							track.ICodecCtx = &codec.OPUSCtx{}
						}
					case *VisualSampleEntry:
						extraData := entry.ExtraData.(*DataBox).Data
						switch entry.Type() {
						case TypeAVC1:
							track.Cid = MP4_CODEC_H264
							if h264Ctx, err := codec.NewH264CtxFromRecord(extraData); err == nil {
								track.ICodecCtx = h264Ctx
							}
						case TypeHVC1, TypeHEV1:
							track.Cid = MP4_CODEC_H265
							if h265Ctx, err := codec.NewH265CtxFromRecord(extraData); err == nil {
								track.ICodecCtx = h265Ctx
							}
						}
					}
				}
				d.Tracks = append(d.Tracks, track)
			}
			d.moov = box
		case *MovieFragmentBox:
			for _, traf := range box.TRAFs {
				track := d.Tracks[traf.TFHD.TrackID-1]
				track.defaultSize = traf.TFHD.DefaultSampleSize
				track.defaultDuration = traf.TFHD.DefaultSampleDuration
			}
		}
	}
	d.ReadSampleIdx = make([]uint32, len(d.Tracks))
	// for _, track := range d.Tracks {
	// 	if len(track.Samplelist) > 0 {
	// 		track.StartDts = uint64(track.Samplelist[0].DTS) * 1000 / uint64(track.Timescale)
	// 		track.EndDts = uint64(track.Samplelist[len(track.Samplelist)-1].DTS) * 1000 / uint64(track.Timescale)
	// 	}
	// }
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
	} else {
		return nil, pkg.ErrNoTrack
	}
	return
}

// func (d *Demuxer) decodeTRUN(trun *TrackRunBox) {
// 	dataOffset := trun.Dataoffset
// 	nextDts := d.currentTrack.StartDts
// 	delta := 0
// 	var cts int64 = 0
// 	for _, entry := range trun.EntryList {
// 		sample := Sample{}
// 		sample.Offset = int64(dataOffset) + int64(d.currentTrack.baseDataOffset)
// 		sample.DTS = (nextDts)
// 		if entry.SampleSize == 0 {
// 			dataOffset += int32(d.currentTrack.defaultSize)
// 			sample.Size = int(d.currentTrack.defaultSize)
// 		} else {
// 			dataOffset += int32(entry.SampleSize)
// 			sample.Size = int(entry.SampleSize)
// 		}

// 		if entry.SampleDuration == 0 {
// 			delta = int(d.currentTrack.defaultDuration)
// 		} else {
// 			delta = int(entry.SampleDuration)
// 		}
// 		cts = int64(entry.SampleCompositionTimeOffset)
// 		sample.PTS = uint64(int64(sample.DTS) + cts)
// 		nextDts += uint64(delta)
// 		d.currentTrack.Samplelist = append(d.currentTrack.Samplelist, sample)
// 	}
// 	d.dataOffset = uint32(dataOffset)
// }

// func (d *Demuxer) decodeSaioBox(saio *SaioBox) (err error) {
// 	if len(saio.Offset) > 0 && len(d.currentTrack.subSamples) == 0 {
// 		var currentOffset int64
// 		currentOffset, err = d.reader.Seek(0, io.SeekCurrent)
// 		if err != nil {
// 			return err
// 		}
// 		d.reader.Seek(d.moofOffset+saio.Offset[0], io.SeekStart)
// 		saiz := d.currentTrack.lastSaiz
// 		for i := uint32(0); i < saiz.SampleCount; i++ {
// 			sampleSize := saiz.DefaultSampleInfoSize
// 			if saiz.DefaultSampleInfoSize == 0 {
// 				sampleSize = saiz.SampleInfo[i]
// 			}
// 			buf := make([]byte, sampleSize)
// 			d.reader.Read(buf)
// 			var se SencEntry
// 			se.IV = make([]byte, 16)
// 			copy(se.IV, buf[:8])
// 			if sampleSize == 8 {
// 				d.currentTrack.subSamples = append(d.currentTrack.subSamples, se)
// 				continue
// 			}
// 			n := 8
// 			sampleCount := binary.BigEndian.Uint16(buf[n:])
// 			n += 2

// 			se.SubSamples = make([]SubSampleEntry, sampleCount)
// 			for j := 0; j < int(sampleCount); j++ {
// 				se.SubSamples[j].BytesOfClearData = binary.BigEndian.Uint16(buf[n:])
// 				n += 2
// 				se.SubSamples[j].BytesOfProtectedData = binary.BigEndian.Uint32(buf[n:])
// 				n += 4
// 			}
// 			d.currentTrack.subSamples = append(d.currentTrack.subSamples, se)
// 		}
// 		d.reader.Seek(currentOffset, io.SeekStart)
// 	}
// 	return nil
// }

// func (d *Demuxer) decodeSgpdBox(size uint32) (err error) {
// 	buf := make([]byte, size-BasicBoxLen)
// 	if _, err = io.ReadFull(d.reader, buf); err != nil {
// 		return
// 	}
// 	n := 0
// 	versionAndFlags := binary.BigEndian.Uint32(buf[n:])
// 	n += 4
// 	version := byte(versionAndFlags >> 24)

// 	b := &SgpdBox{
// 		Version: version,
// 		Flags:   versionAndFlags & 0x00ffffff,
// 	}
// 	b.GroupingType = string(buf[n : n+4])
// 	n += 4

// 	if b.Version >= 1 {
// 		b.DefaultLength = binary.BigEndian.Uint32(buf[n:])
// 		n += 4
// 	}
// 	if b.Version >= 2 {
// 		b.DefaultGroupDescriptionIndex = binary.BigEndian.Uint32(buf[n:])
// 		n += 4
// 	}
// 	entryCount := int(binary.BigEndian.Uint32(buf[n:]))
// 	n += 4

// 	track := d.Tracks[len(d.Tracks)-1]
// 	for i := 0; i < entryCount; i++ {
// 		var descriptionLength = b.DefaultLength
// 		if b.Version >= 1 && b.DefaultLength == 0 {
// 			descriptionLength = binary.BigEndian.Uint32(buf[n:])
// 			n += 4
// 			b.DescriptionLengths = append(b.DescriptionLengths, descriptionLength)
// 		}
// 		var (
// 			sgEntry interface{}
// 			offset  int
// 		)
// 		sgEntry, offset, err = DecodeSampleGroupEntry(b.GroupingType, descriptionLength, buf[n:])
// 		n += offset
// 		if err != nil {
// 			return err
// 		}
// 		if sgEntry == nil {
// 			continue
// 		}
// 		if seig, ok := sgEntry.(*SeigSampleGroupEntry); ok {
// 			track.lastSeig = seig
// 		}
// 		b.SampleGroupEntries = append(b.SampleGroupEntries, sgEntry)
// 	}

// 	return nil
// }

// func (d *Demuxer) readSubSample(idx uint32, track *Track) (subSample *SubSample) {
// 	if int(idx) < len(track.subSamples) {
// 		subSample = new(SubSample)
// 		subSample.Number = idx
// 		if len(track.subSamples[idx].IV) > 0 {
// 			copy(subSample.IV[:], track.subSamples[idx].IV)
// 		} else {
// 			copy(subSample.IV[:], track.defaultConstantIV)
// 		}
// 		if track.lastSeig != nil {
// 			copy(subSample.KID[:], track.lastSeig.KID[:])
// 			subSample.CryptByteBlock = track.lastSeig.CryptByteBlock
// 			subSample.SkipByteBlock = track.lastSeig.SkipByteBlock
// 		} else {
// 			copy(subSample.KID[:], track.defaultKID[:])
// 			subSample.CryptByteBlock = track.defaultCryptByteBlock
// 			subSample.SkipByteBlock = track.defaultSkipByteBlock
// 		}
// 		subSample.PsshBoxes = append(subSample.PsshBoxes, d.pssh...)
// 		if len(track.subSamples[idx].SubSamples) > 0 {
// 			subSample.Patterns = make([]SubSamplePattern, len(track.subSamples[idx].SubSamples))
// 			for ei, e := range track.subSamples[idx].SubSamples {
// 				subSample.Patterns[ei].BytesClear = e.BytesOfClearData
// 				subSample.Patterns[ei].BytesProtected = e.BytesOfProtectedData
// 			}
// 		}
// 		return subSample
// 	}
// 	return nil
// }

func (d *Demuxer) ReadSample(yield func(*Track, Sample) bool) {
	for {
		maxdts := int64(-1)
		minTsSample := Sample{Timestamp: uint32(maxdts)}
		var whichTrack *Track
		for _, track := range d.Tracks {
			idx := d.ReadSampleIdx[track.TrackId-1]
			if int(idx) == len(track.Samplelist) {
				continue
			}
			if whichTrack == nil {
				minTsSample = track.Samplelist[idx]
				whichTrack = track
			} else {
				dts1 := uint64(minTsSample.Timestamp) * uint64(d.moov.MVHD.Timescale) / uint64(whichTrack.Timescale)
				dts2 := uint64(track.Samplelist[idx].Timestamp) * uint64(d.moov.MVHD.Timescale) / uint64(track.Timescale)
				if dts1 > dts2 {
					minTsSample = track.Samplelist[idx]
					whichTrack = track
				}
			}
			// subSample := d.readSubSample(idx, whichTrack)
		}
		if minTsSample.Timestamp == uint32(maxdts) {
			return
		}

		d.ReadSampleIdx[whichTrack.TrackId-1]++
		if !yield(whichTrack, minTsSample) {
			return
		}
	}
}

func (d *Demuxer) RangeSample(yield func(*Track, *Sample) bool) {
	for {
		var minTsSample *Sample
		var whichTrack *Track
		for _, track := range d.Tracks {
			idx := d.ReadSampleIdx[track.TrackId-1]
			if int(idx) == len(track.Samplelist) {
				continue
			}
			if whichTrack == nil {
				minTsSample = &track.Samplelist[idx]
				whichTrack = track
			} else {
				if minTsSample.Offset > track.Samplelist[idx].Offset {
					minTsSample = &track.Samplelist[idx]
					whichTrack = track
				}
			}
			// subSample := d.readSubSample(idx, whichTrack)
		}
		if minTsSample == nil {
			return
		}
		d.ReadSampleIdx[whichTrack.TrackId-1]++
		if !yield(whichTrack, minTsSample) {
			return
		}
	}
}

// GetMoovBox returns the Movie Box from the demuxer
func (d *Demuxer) GetMoovBox() *MoovBox {
	return d.moov
}
