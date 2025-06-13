package mp4

import (
	"errors"
	"io"
	"slices"

	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
	"m7s.live/v5/plugin/mp4/pkg/box"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
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
		PsshBoxes      []*box.PsshBox
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

	RTMPFrame struct {
		Frame any // 可以是 *rtmp.RTMPVideo 或 *rtmp.RTMPAudio
	}

	Demuxer struct {
		reader        io.ReadSeeker
		Tracks        []*Track
		ReadSampleIdx []uint32
		IsFragment    bool
		// pssh          []*box.PsshBox
		moov       *box.MoovBox
		mdat       *box.MediaDataBox
		mdatOffset uint64
		QuicTime   bool

		// 预生成的 RTMP 帧
		RTMPVideoSequence *rtmp.RTMPVideo
		RTMPAudioSequence *rtmp.RTMPAudio
		RTMPFrames        []RTMPFrame

		// RTMP 帧生成配置
		RTMPAllocator *util.ScalableMemoryAllocator
	}
)

func NewDemuxer(r io.ReadSeeker) *Demuxer {
	return &Demuxer{
		reader: r,
	}
}

func (d *Demuxer) Demux() (err error) {
	return d.DemuxWithAllocator(nil)
}

func (d *Demuxer) DemuxWithAllocator(allocator *util.ScalableMemoryAllocator) (err error) {

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
	var b box.IBox
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
		switch boxData := b.(type) {
		case *box.FileTypeBox:
			if slices.Contains(boxData.CompatibleBrands, [4]byte{'q', 't', ' ', ' '}) {
				d.QuicTime = true
			}
		case *box.FreeBox:
		case *box.MediaDataBox:
			d.mdat = boxData
			d.mdatOffset = offset - b.Size() + uint64(boxData.HeaderSize())
		case *box.MoovBox:
			if boxData.MVEX != nil {
				d.IsFragment = true
			}
			for _, trak := range boxData.Tracks {
				track := &Track{}
				track.TrackId = trak.TKHD.TrackID
				track.Duration = uint32(trak.TKHD.Duration)
				track.Timescale = trak.MDIA.MDHD.Timescale
				// 创建RTMP样本处理回调
				var sampleCallback box.SampleCallback
				if d.RTMPAllocator != nil {
					sampleCallback = d.createRTMPSampleCallback(track, trak)
				}

				track.Samplelist = trak.ParseSamplesWithCallback(sampleCallback)
				if len(trak.MDIA.MINF.STBL.STSD.Entries) > 0 {
					entryBox := trak.MDIA.MINF.STBL.STSD.Entries[0]
					switch entry := entryBox.(type) {
					case *box.AudioSampleEntry:
						switch entry.Type() {
						case box.TypeMP4A:
							track.Cid = box.MP4_CODEC_AAC
						case box.TypeALAW:
							track.Cid = box.MP4_CODEC_G711A
						case box.TypeULAW:
							track.Cid = box.MP4_CODEC_G711U
						case box.TypeOPUS:
							track.Cid = box.MP4_CODEC_OPUS
						}
						track.SampleRate = entry.Samplerate
						track.ChannelCount = uint8(entry.ChannelCount)
						track.SampleSize = entry.SampleSize
						switch extra := entry.ExtraData.(type) {
						case *box.ESDSBox:
							track.Cid, track.ExtraData = box.DecodeESDescriptor(extra.Data)
						}
					case *box.VisualSampleEntry:
						track.ExtraData = entry.ExtraData.(*box.DataBox).Data
						switch entry.Type() {
						case box.TypeAVC1:
							track.Cid = box.MP4_CODEC_H264
						case box.TypeHVC1, box.TypeHEV1:
							track.Cid = box.MP4_CODEC_H265
						}
						track.Width = uint32(entry.Width)
						track.Height = uint32(entry.Height)
					}
				}
				d.Tracks = append(d.Tracks, track)
			}
			d.moov = boxData
		case *box.MovieFragmentBox:
			for _, traf := range boxData.TRAFs {
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

func (d *Demuxer) SeekTime(dts uint64) (sample *box.Sample, err error) {
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

/**
* @brief 函数跳帧到dts 前面的第一个关键帧位置
*
* @param 参数名dts 跳帧位置
*
* @todo 待实现的功能或改进点   audioTrack 没有同步改进
* @author erroot
* @date 250614
*
**/
func (d *Demuxer) SeekTimePreIDR(dts uint64) (sample *Sample, err error) {
	var audioTrack, videoTrack *Track
	for _, track := range d.Tracks {
		if track.Cid.IsAudio() {
			audioTrack = track
		} else if track.Cid.IsVideo() {
			videoTrack = track
		}
	}
	if videoTrack != nil {
		idx := videoTrack.SeekPreIDR(dts)
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

func (d *Demuxer) ReadSample(yield func(*Track, box.Sample) bool) {
	for {
		maxdts := int64(-1)
		minTsSample := box.Sample{Timestamp: uint32(maxdts)}
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
				dts1 := uint64(minTsSample.Timestamp) * uint64(d.moov.MVHD.Timescale) / uint64(whichTrack.Timescale)
				dts2 := uint64(track.Samplelist[idx].Timestamp) * uint64(d.moov.MVHD.Timescale) / uint64(track.Timescale)
				if dts1 > dts2 {
					minTsSample = track.Samplelist[idx]
					whichTrack = track
					whichTracki = i
				}
			}
			// subSample := d.readSubSample(idx, whichTrack)
		}
		if minTsSample.Timestamp == uint32(maxdts) {
			return
		}

		d.ReadSampleIdx[whichTracki]++
		if !yield(whichTrack, minTsSample) {
			return
		}
	}
}

func (d *Demuxer) RangeSample(yield func(*Track, *box.Sample) bool) {
	for {
		var minTsSample *box.Sample
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

// GetMoovBox returns the Movie Box from the demuxer
func (d *Demuxer) GetMoovBox() *box.MoovBox {
	return d.moov
}

// CreateRTMPSequenceFrame 创建 RTMP 序列帧
func (d *Demuxer) CreateRTMPSequenceFrame(track *Track, allocator *util.ScalableMemoryAllocator) (videoSeq *rtmp.RTMPVideo, audioSeq *rtmp.RTMPAudio, err error) {
	switch track.Cid {
	case box.MP4_CODEC_H264:
		videoSeq = &rtmp.RTMPVideo{}
		videoSeq.SetAllocator(allocator)
		videoSeq.Append([]byte{0x17, 0x00, 0x00, 0x00, 0x00}, track.ExtraData)
	case box.MP4_CODEC_H265:
		videoSeq = &rtmp.RTMPVideo{}
		videoSeq.SetAllocator(allocator)
		videoSeq.Append([]byte{0b1001_0000 | rtmp.PacketTypeSequenceStart}, codec.FourCC_H265[:], track.ExtraData)
	case box.MP4_CODEC_AAC:
		audioSeq = &rtmp.RTMPAudio{}
		audioSeq.SetAllocator(allocator)
		audioSeq.Append([]byte{0xaf, 0x00}, track.ExtraData)
	}
	return
}

// ConvertSampleToRTMP 将 MP4 sample 转换为 RTMP 格式
func (d *Demuxer) ConvertSampleToRTMP(track *Track, sample box.Sample, allocator *util.ScalableMemoryAllocator, timestampOffset uint64) (videoFrame *rtmp.RTMPVideo, audioFrame *rtmp.RTMPAudio, err error) {
	switch track.Cid {
	case box.MP4_CODEC_H264:
		videoFrame = &rtmp.RTMPVideo{}
		videoFrame.SetAllocator(allocator)
		videoFrame.CTS = sample.CTS
		videoFrame.Timestamp = uint32(uint64(sample.Timestamp)*1000/uint64(track.Timescale) + timestampOffset)
		videoFrame.AppendOne([]byte{util.Conditional[byte](sample.KeyFrame, 0x17, 0x27), 0x01, byte(videoFrame.CTS >> 24), byte(videoFrame.CTS >> 8), byte(videoFrame.CTS)})
		videoFrame.AddRecycleBytes(sample.Data)
	case box.MP4_CODEC_H265:
		videoFrame = &rtmp.RTMPVideo{}
		videoFrame.SetAllocator(allocator)
		videoFrame.CTS = uint32(sample.CTS)
		videoFrame.Timestamp = uint32(uint64(sample.Timestamp)*1000/uint64(track.Timescale) + timestampOffset)
		var head []byte
		var b0 byte = 0b1010_0000
		if sample.KeyFrame {
			b0 = 0b1001_0000
		}
		if videoFrame.CTS == 0 {
			head = videoFrame.NextN(5)
			head[0] = b0 | rtmp.PacketTypeCodedFramesX
		} else {
			head = videoFrame.NextN(8)
			head[0] = b0 | rtmp.PacketTypeCodedFrames
			util.PutBE(head[5:8], videoFrame.CTS) // cts
		}
		copy(head[1:], codec.FourCC_H265[:])
		videoFrame.AddRecycleBytes(sample.Data)
	case box.MP4_CODEC_AAC:
		audioFrame = &rtmp.RTMPAudio{}
		audioFrame.SetAllocator(allocator)
		audioFrame.Timestamp = uint32(uint64(sample.Timestamp)*1000/uint64(track.Timescale) + timestampOffset)
		audioFrame.AppendOne([]byte{0xaf, 0x01})
		audioFrame.AddRecycleBytes(sample.Data)
	case box.MP4_CODEC_G711A:
		audioFrame = &rtmp.RTMPAudio{}
		audioFrame.SetAllocator(allocator)
		audioFrame.Timestamp = uint32(uint64(sample.Timestamp)*1000/uint64(track.Timescale) + timestampOffset)
		audioFrame.AppendOne([]byte{0x72})
		audioFrame.AddRecycleBytes(sample.Data)
	case box.MP4_CODEC_G711U:
		audioFrame = &rtmp.RTMPAudio{}
		audioFrame.SetAllocator(allocator)
		audioFrame.Timestamp = uint32(uint64(sample.Timestamp)*1000/uint64(track.Timescale) + timestampOffset)
		audioFrame.AppendOne([]byte{0x82})
		audioFrame.AddRecycleBytes(sample.Data)
	}
	return
}

// GetRTMPSequenceFrames 获取预生成的 RTMP 序列帧
func (d *Demuxer) GetRTMPSequenceFrames() (videoSeq *rtmp.RTMPVideo, audioSeq *rtmp.RTMPAudio) {
	return d.RTMPVideoSequence, d.RTMPAudioSequence
}

// IterateRTMPFrames 迭代预生成的 RTMP 帧
func (d *Demuxer) IterateRTMPFrames(timestampOffset uint64, yield func(*RTMPFrame) bool) {
	for i := range d.RTMPFrames {
		frame := &d.RTMPFrames[i]

		// 应用时间戳偏移
		switch f := frame.Frame.(type) {
		case *rtmp.RTMPVideo:
			f.Timestamp += uint32(timestampOffset)
		case *rtmp.RTMPAudio:
			f.Timestamp += uint32(timestampOffset)
		}

		if !yield(frame) {
			return
		}
	}
}

// GetMaxTimestamp 获取所有帧中的最大时间戳
func (d *Demuxer) GetMaxTimestamp() uint64 {
	var maxTimestamp uint64
	for _, frame := range d.RTMPFrames {
		var timestamp uint64
		switch f := frame.Frame.(type) {
		case *rtmp.RTMPVideo:
			timestamp = uint64(f.Timestamp)
		case *rtmp.RTMPAudio:
			timestamp = uint64(f.Timestamp)
		}
		if timestamp > maxTimestamp {
			maxTimestamp = timestamp
		}
	}
	return maxTimestamp
}

// generateRTMPFrames 生成RTMP序列帧和所有帧数据
func (d *Demuxer) generateRTMPFrames(allocator *util.ScalableMemoryAllocator) (err error) {
	// 生成序列帧
	for _, track := range d.Tracks {
		if track.Cid.IsVideo() && d.RTMPVideoSequence == nil {
			d.RTMPVideoSequence, _, err = d.CreateRTMPSequenceFrame(track, allocator)
			if err != nil {
				return err
			}
		} else if track.Cid.IsAudio() && d.RTMPAudioSequence == nil {
			_, d.RTMPAudioSequence, err = d.CreateRTMPSequenceFrame(track, allocator)
			if err != nil {
				return err
			}
		}
	}

	// 预生成所有 RTMP 帧
	d.RTMPFrames = make([]RTMPFrame, 0)

	// 收集所有样本并按时间戳排序
	type sampleInfo struct {
		track       *Track
		sample      box.Sample
		sampleIndex uint32
		trackIndex  int
	}

	var allSamples []sampleInfo
	for trackIdx, track := range d.Tracks {
		for sampleIdx, sample := range track.Samplelist {
			// 读取样本数据
			if _, err = d.reader.Seek(sample.Offset, io.SeekStart); err != nil {
				return err
			}
			sample.Data = allocator.Malloc(sample.Size)
			if _, err = io.ReadFull(d.reader, sample.Data); err != nil {
				allocator.Free(sample.Data)
				return err
			}

			allSamples = append(allSamples, sampleInfo{
				track:       track,
				sample:      sample,
				sampleIndex: uint32(sampleIdx),
				trackIndex:  trackIdx,
			})
		}
	}

	// 按时间戳排序样本
	slices.SortFunc(allSamples, func(a, b sampleInfo) int {
		timeA := uint64(a.sample.Timestamp) * uint64(d.moov.MVHD.Timescale) / uint64(a.track.Timescale)
		timeB := uint64(b.sample.Timestamp) * uint64(d.moov.MVHD.Timescale) / uint64(b.track.Timescale)
		if timeA < timeB {
			return -1
		} else if timeA > timeB {
			return 1
		}
		return 0
	})

	// 预生成 RTMP 帧
	for _, sampleInfo := range allSamples {
		videoFrame, audioFrame, err := d.ConvertSampleToRTMP(sampleInfo.track, sampleInfo.sample, allocator, 0)
		if err != nil {
			return err
		}

		if videoFrame != nil {
			d.RTMPFrames = append(d.RTMPFrames, RTMPFrame{Frame: videoFrame})
		}

		if audioFrame != nil {
			d.RTMPFrames = append(d.RTMPFrames, RTMPFrame{Frame: audioFrame})
		}
	}

	return nil
}

// createRTMPSampleCallback 创建RTMP样本处理回调函数
func (d *Demuxer) createRTMPSampleCallback(track *Track, trak *box.TrakBox) box.SampleCallback {
	// 首先生成序列帧
	if track.Cid.IsVideo() && d.RTMPVideoSequence == nil {
		videoSeq, _, err := d.CreateRTMPSequenceFrame(track, d.RTMPAllocator)
		if err == nil {
			d.RTMPVideoSequence = videoSeq
		}
	} else if track.Cid.IsAudio() && d.RTMPAudioSequence == nil {
		_, audioSeq, err := d.CreateRTMPSequenceFrame(track, d.RTMPAllocator)
		if err == nil {
			d.RTMPAudioSequence = audioSeq
		}
	}

	return func(sample *box.Sample, sampleIndex int) error {
		// 读取样本数据
		if _, err := d.reader.Seek(sample.Offset, io.SeekStart); err != nil {
			return err
		}
		sample.Data = d.RTMPAllocator.Malloc(sample.Size)
		if _, err := io.ReadFull(d.reader, sample.Data); err != nil {
			d.RTMPAllocator.Free(sample.Data)
			return err
		}

		// 转换为 RTMP 格式
		videoFrame, audioFrame, err := d.ConvertSampleToRTMP(track, *sample, d.RTMPAllocator, 0)
		if err != nil {
			return err
		}

		// 内部收集RTMP帧
		if videoFrame != nil {
			d.RTMPFrames = append(d.RTMPFrames, RTMPFrame{Frame: videoFrame})
		}
		if audioFrame != nil {
			d.RTMPFrames = append(d.RTMPFrames, RTMPFrame{Frame: audioFrame})
		}

		return nil
	}
}
