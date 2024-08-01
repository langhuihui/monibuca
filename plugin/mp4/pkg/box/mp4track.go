package box

import (
	"encoding/binary"
	"github.com/yapingcat/gomedia/go-codec"
	"io"
)

type sampleCache struct {
	pts    uint64
	dts    uint64
	hasVcl bool
	isKey  bool
	cache  []byte
}

type sampleEntry struct {
	pts                    uint64
	dts                    uint64
	offset                 uint64
	size                   uint64
	isKeyFrame             bool
	SampleDescriptionIndex uint32 //always should be 1
}

type movchunk struct {
	chunknum    uint32
	samplenum   uint32
	chunkoffset uint64
}

type extraData interface {
	export() []byte
	load(data []byte)
}

type h264ExtraData struct {
	spss [][]byte
	ppss [][]byte
}

func (extra *h264ExtraData) export() []byte {
	data, _ := codec.CreateH264AVCCExtradata(extra.spss, extra.ppss)
	return data
}

func (extra *h264ExtraData) load(data []byte) {
	extra.spss, extra.ppss = codec.CovertExtradata(data)
}

type h265ExtraData struct {
	hvccExtra *codec.HEVCRecordConfiguration
}

func newh265ExtraData() *h265ExtraData {
	return &h265ExtraData{
		hvccExtra: codec.NewHEVCRecordConfiguration(),
	}
}

func (extra *h265ExtraData) export() []byte {
	if extra.hvccExtra == nil {
		panic("extra.hvccExtra must init")
	}
	data, _ := extra.hvccExtra.Encode()
	return data
}

func (extra *h265ExtraData) load(data []byte) {
	if extra.hvccExtra == nil {
		panic("extra.hvccExtra must init")
	}
	extra.hvccExtra.Decode(data)
}

type aacExtraData struct {
	asc []byte
}

func (extra *aacExtraData) export() []byte {
	return extra.asc
}

func (extra *aacExtraData) load(data []byte) {
	extra.asc = make([]byte, len(data))
	copy(extra.asc, data)
}

type movFragment struct {
	offset   uint64
	duration uint32
	firstDts uint64
	firstPts uint64
	lastPts  uint64
	lastDts  uint64
}

type mp4track struct {
	cid         MP4_CODEC_TYPE
	trackId     uint32
	stbltable   *movstbl
	duration    uint32
	timescale   uint32
	width       uint32
	height      uint32
	sampleRate  uint32
	sampleBits  uint8
	chanelCount uint8
	samplelist  []sampleEntry
	elst        *movelst
	extra       extraData
	lastSample  *sampleCache
	writer      io.WriteSeeker
	fragments   []movFragment

	//for fmp4
	extraData          []byte
	startDts           uint64
	startPts           uint64
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
	subSamples             []sencEntry
}

func newmp4track(cid MP4_CODEC_TYPE, writer io.WriteSeeker) *mp4track {
	track := &mp4track{
		cid:        cid,
		timescale:  1000,
		stbltable:  nil,
		samplelist: make([]sampleEntry, 0),
		lastSample: &sampleCache{
			hasVcl: false,
			cache:  make([]byte, 0, 128),
		},
		writer:    writer,
		fragments: make([]movFragment, 0, 32),
		startDts:  0,
	}

	if cid == MP4_CODEC_H264 {
		track.extra = new(h264ExtraData)
	} else if cid == MP4_CODEC_H265 {
		track.extra = newh265ExtraData()
	} else if cid == MP4_CODEC_AAC {
		track.extra = new(aacExtraData)
	}
	return track
}

func (track *mp4track) addSampleEntry(entry sampleEntry) {
	if len(track.samplelist) <= 1 {
		track.duration = 0
	} else {
		delta := int64(entry.dts - track.samplelist[len(track.samplelist)-1].dts)
		if delta < 0 {
			track.duration += 1
		} else {
			track.duration += uint32(delta)
		}
	}
	track.samplelist = append(track.samplelist, entry)
}

func (track *mp4track) makeStblTable() {
	if track.stbltable == nil {
		track.stbltable = new(movstbl)
	}
	sameSize := true
	stts := new(movstts)
	stts.entrys = make([]sttsEntry, 0)
	movchunks := make([]movchunk, 0)
	ctts := new(movctts)
	ctts.entrys = make([]cttsEntry, 0)
	ckn := uint32(0)
	for i, sample := range track.samplelist {
		sttsEntry := sttsEntry{sampleCount: 1, sampleDelta: 1}
		cttsEntry := cttsEntry{sampleCount: 1, sampleOffset: uint32(sample.pts) - uint32(sample.dts)}
		if i == len(track.samplelist)-1 {
			stts.entrys = append(stts.entrys, sttsEntry)
			stts.entryCount++
		} else {
			var delta uint64 = 1
			if track.samplelist[i+1].dts >= sample.dts {
				delta = track.samplelist[i+1].dts - sample.dts
			}

			if len(stts.entrys) > 0 && delta == uint64(stts.entrys[len(stts.entrys)-1].sampleDelta) {
				stts.entrys[len(stts.entrys)-1].sampleCount++
			} else {
				sttsEntry.sampleDelta = uint32(delta)
				stts.entrys = append(stts.entrys, sttsEntry)
				stts.entryCount++
			}
		}

		if len(ctts.entrys) == 0 {
			ctts.entrys = append(ctts.entrys, cttsEntry)
			ctts.entryCount++
		} else {
			if ctts.entrys[len(ctts.entrys)-1].sampleOffset == cttsEntry.sampleOffset {
				ctts.entrys[len(ctts.entrys)-1].sampleCount++
			} else {
				ctts.entrys = append(ctts.entrys, cttsEntry)
				ctts.entryCount++
			}
		}
		if sameSize && i < len(track.samplelist)-1 && track.samplelist[i+1].size != track.samplelist[i].size {
			sameSize = false
		}
		if i > 0 && sample.offset == track.samplelist[i-1].offset+track.samplelist[i-1].size {
			movchunks[ckn-1].samplenum++
		} else {
			ck := movchunk{chunknum: ckn, samplenum: 1, chunkoffset: sample.offset}
			movchunks = append(movchunks, ck)
			ckn++
		}
	}
	stsz := &movstsz{
		sampleSize:  0,
		sampleCount: uint32(len(track.samplelist)),
	}
	if sameSize {
		stsz.sampleSize = uint32(track.samplelist[0].size)
	} else {
		stsz.entrySizelist = make([]uint32, stsz.sampleCount)
		for i := 0; i < len(stsz.entrySizelist); i++ {
			stsz.entrySizelist[i] = uint32(track.samplelist[i].size)
		}
	}

	stsc := &movstsc{
		entrys:     make([]stscEntry, len(movchunks)),
		entryCount: 0,
	}
	for i, chunk := range movchunks {
		if i == 0 || chunk.samplenum != movchunks[i-1].samplenum {
			stsc.entrys[stsc.entryCount].firstChunk = chunk.chunknum + 1
			stsc.entrys[stsc.entryCount].sampleDescriptionIndex = 1
			stsc.entrys[stsc.entryCount].samplesPerChunk = chunk.samplenum
			stsc.entryCount++
		}
	}
	stco := &movstco{entryCount: ckn, chunkOffsetlist: make([]uint64, ckn)}
	for i := 0; i < int(stco.entryCount); i++ {
		stco.chunkOffsetlist[i] = movchunks[i].chunkoffset
	}
	track.stbltable.stts = stts
	track.stbltable.stsc = stsc
	track.stbltable.stco = stco
	track.stbltable.stsz = stsz
	if track.cid == MP4_CODEC_H264 || track.cid == MP4_CODEC_H265 {
		track.stbltable.ctts = ctts
	}
}

func (track *mp4track) makeEmptyStblTable() {
	track.stbltable = new(movstbl)
	track.stbltable.stts = &movstts{}
	track.stbltable.stsc = &movstsc{}
	track.stbltable.stco = &movstco{}
	track.stbltable.stsz = &movstsz{}
	track.stbltable.stss = &movstss{}
}

func (track *mp4track) writeH264(nalus [][]byte, pts, dts uint64) (err error) {
	//h264extra, ok := track.extra.(*h264ExtraData)
	//if !ok {
	//	panic("must init h264ExtraData first")
	//}
	for _, nalu := range nalus {
		nalu_type := codec.H264_NAL_TYPE(nalu[0] & 0x1F)
		//aud/sps/pps/sei 为帧间隔
		//通过first_slice_in_mb来判断，改nalu是否为一帧的开头
		if track.lastSample.hasVcl && isH264NewAccessUnit(nalu) {
			var currentOffset int64
			if currentOffset, err = track.writer.Seek(0, io.SeekCurrent); err != nil {
				return
			}
			entry := sampleEntry{
				pts:                    track.lastSample.pts,
				dts:                    track.lastSample.dts,
				size:                   0,
				isKeyFrame:             track.lastSample.isKey,
				SampleDescriptionIndex: 1,
				offset:                 uint64(currentOffset),
			}
			n := 0
			if n, err = track.writer.Write(track.lastSample.cache); err != nil {
				return
			}
			entry.size = uint64(n)
			track.addSampleEntry(entry)
			track.lastSample.cache = track.lastSample.cache[:0]
			track.lastSample.hasVcl = false
		}
		if codec.IsH264VCLNaluType(nalu_type) {
			track.lastSample.pts = pts
			track.lastSample.dts = dts
			track.lastSample.hasVcl = true
			track.lastSample.isKey = false
			if nalu_type == codec.H264_NAL_I_SLICE {
				track.lastSample.isKey = true
			}
		}
		naluLen := uint32(len(nalu))
		var lenBytes [4]byte
		binary.BigEndian.PutUint32(lenBytes[:], naluLen)
		track.lastSample.cache = append(append(track.lastSample.cache, lenBytes[:]...), nalu...)
	}
	return
}

func (track *mp4track) writeH265(nalus [][]byte, pts, dts uint64) (err error) {
	//h265extra, ok := track.extra.(*h265ExtraData)
	//if !ok {
	//	panic("must init h265ExtraData first")
	//}
	for _, nalu := range nalus {
		nalu_type := codec.H265_NAL_TYPE((nalu[0] >> 1) & 0x3F)
		if track.lastSample.hasVcl && isH265NewAccessUnit(nalu) {
			var currentOffset int64
			if currentOffset, err = track.writer.Seek(0, io.SeekCurrent); err != nil {
				return
			}
			entry := sampleEntry{
				pts:                    track.lastSample.pts,
				dts:                    track.lastSample.dts,
				size:                   0,
				isKeyFrame:             track.lastSample.isKey,
				SampleDescriptionIndex: 1,
				offset:                 uint64(currentOffset),
			}
			n := 0
			if n, err = track.writer.Write(track.lastSample.cache); err != nil {
				return
			}
			entry.size = uint64(n)
			track.addSampleEntry(entry)
			track.lastSample.cache = track.lastSample.cache[:0]
			track.lastSample.hasVcl = false
		}
		if codec.IsH265VCLNaluType(nalu_type) {
			track.lastSample.pts = pts
			track.lastSample.dts = dts
			track.lastSample.hasVcl = true
			track.lastSample.isKey = false
			if nalu_type >= codec.H265_NAL_SLICE_BLA_W_LP && nalu_type <= codec.H265_NAL_SLICE_CRA {
				track.lastSample.isKey = true
			}
		}
		naluLen := uint32(len(nalu))
		var lenBytes [4]byte
		binary.BigEndian.PutUint32(lenBytes[:], naluLen)
		track.lastSample.cache = append(append(track.lastSample.cache, lenBytes[:]...), nalu...)
	}

	return
}

func (track *mp4track) writeAAC(aacframes []byte, pts, dts uint64) (err error) {
	var currentOffset int64
	if currentOffset, err = track.writer.Seek(0, io.SeekCurrent); err != nil {
		return
	}
	entry := sampleEntry{
		pts:                    pts,
		dts:                    dts,
		size:                   0,
		SampleDescriptionIndex: 1,
		offset:                 uint64(currentOffset),
	}
	n := 0
	n, err = track.writer.Write(aacframes)
	if err != nil {
		return
	}
	currentOffset += int64(n)
	entry.size = uint64(n)
	track.addSampleEntry(entry)
	return
}

func (track *mp4track) writeG711(g711 []byte, pts, dts uint64) (err error) {
	var currentOffset int64
	if currentOffset, err = track.writer.Seek(0, io.SeekCurrent); err != nil {
		return
	}
	entry := sampleEntry{
		pts:                    pts,
		dts:                    dts,
		size:                   0,
		SampleDescriptionIndex: 1,
		offset:                 uint64(currentOffset),
	}
	n := 0
	n, err = track.writer.Write(g711)
	entry.size = uint64(n)
	track.addSampleEntry(entry)
	return
}

func (track *mp4track) writeMP3(mp3 []byte, pts, dts uint64) (err error) {
	if track.sampleRate == 0 {
		codec.SplitMp3Frames(mp3, func(head *codec.MP3FrameHead, frame []byte) {
			track.sampleRate = uint32(head.GetSampleRate())
			track.chanelCount = uint8(head.GetChannelCount())
			track.sampleBits = 16
		})
		if track.sampleRate > 24000 {
			track.cid = MP4_CODEC_MP2
		} else {
			track.cid = MP4_CODEC_MP3
		}
	}
	return track.writeG711(mp3, pts, dts)
}

func (track *mp4track) writeOPUS(opus []byte, pts, dts uint64) (err error) {
	if track.sampleRate == 0 {
		opusPacket := codec.DecodeOpusPacket(opus)
		track.sampleRate = 48000 // TODO: fixed?
		if opusPacket.Stereo != 0 {
			track.chanelCount = 1
		} else {
			track.chanelCount = 2
		}
		track.sampleBits = 16 // TODO: fixed
	}

	return track.writeG711(opus, pts, dts)
}

func (track *mp4track) flush() (err error) {
	var currentOffset int64
	if track.lastSample != nil && len(track.lastSample.cache) > 0 {
		if currentOffset, err = track.writer.Seek(0, io.SeekCurrent); err != nil {
			return err
		}
		entry := sampleEntry{
			pts:                    track.lastSample.pts,
			dts:                    track.lastSample.dts,
			isKeyFrame:             track.lastSample.isKey,
			size:                   0,
			SampleDescriptionIndex: 1,
			offset:                 uint64(currentOffset),
		}
		n := 0
		if n, err = track.writer.Write(track.lastSample.cache); err != nil {
			return err
		}
		entry.size = uint64(n)
		track.addSampleEntry(entry)
		track.lastSample.cache = track.lastSample.cache[:0]
		track.lastSample.hasVcl = false
		track.lastSample.isKey = false
		track.lastSample.dts = 0
		track.lastSample.pts = 0
	}
	return nil
}

func (track *mp4track) clearSamples() {
	track.samplelist = track.samplelist[:0]
}
