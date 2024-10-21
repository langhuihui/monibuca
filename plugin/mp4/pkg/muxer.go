package mp4

import (
	"encoding/binary"
	"errors"
	"io"
	"os"

	. "m7s.live/v5/plugin/mp4/pkg/box"
)

const (
	FLAG_FRAGMENT Flag = (1 << 1)
	FLAG_KEYFRAME Flag = (1 << 3)
	FLAG_CUSTOM   Flag = (1 << 5)
	FLAG_DASH     Flag = (1 << 11)
)

type (
	Flag uint32

	Muxer struct {
		nextTrackId    uint32
		nextFragmentId uint32
		CurrentOffset  int64
		Tracks         map[uint32]*Track
		Flag
		fragDuration uint32
		moov         *BasicBox
		mdatOffset   uint64
		mdatSize     uint64
	}
	FileMuxer struct {
		*Muxer
		*os.File
	}
	FMP4Muxer struct {
		*Muxer
		writer io.WriteSeeker
	}
)

func (m Muxer) isFragment() bool {
	return (m.Flag & FLAG_FRAGMENT) != 0
}

func (m Muxer) isDash() bool {
	return (m.Flag & FLAG_DASH) != 0
}

func (m Muxer) has(flag Flag) bool {
	return (m.Flag & flag) != 0
}

func NewFileMuxer(f *os.File) (muxer *FileMuxer, err error) {
	muxer = &FileMuxer{
		File:  f,
		Muxer: NewMuxer(0),
	}
	err = muxer.WriteInitSegment(f)
	if err != nil {
		return nil, err
	}
	err = muxer.WriteEmptyMdat(f)
	if err != nil {
		return nil, err
	}
	return
}

func NewFMP4Muxer(w io.WriteSeeker) *FMP4Muxer {
	muxer := &FMP4Muxer{
		writer: w,
		Muxer:  NewMuxer(FLAG_FRAGMENT),
	}
	return muxer
}

func NewMuxer(flag Flag) *Muxer {
	return &Muxer{
		nextTrackId:    1,
		nextFragmentId: 1,
		Tracks:         make(map[uint32]*Track),
		Flag:           flag,
	}
}

func (m *Muxer) WriteInitSegment(w io.Writer) (err error) {
	var n int
	n, err = w.Write(MakeFtypBox(TypeISOM, 0x200, TypeISOM, TypeISO2, TypeAVC1, TypeMP41))
	if err != nil {
		return
	}
	m.CurrentOffset = int64(n)
	n, err = w.Write((new(FreeBox)).Encode())
	if err != nil {
		return
	}
	m.CurrentOffset += int64(n)
	return
}

func (m *Muxer) WriteEmptyMdat(w io.Writer) (err error) {
	mdat := BasicBox{Type: TypeMDAT, Size: 8}
	mdatlen, mdatBox := mdat.Encode()
	m.mdatOffset = uint64(m.CurrentOffset + 8)
	var n int
	n, err = w.Write(mdatBox[0:mdatlen])
	if err != nil {
		return
	}
	m.CurrentOffset += int64(n)
	return
}

// func (d *Muxer) WriteInitSegment(w io.Writer) error {
// 	_, err := w.Write(MakeFtypBox(TypeISO5, 0x200, TypeISO5, TypeISO6, TypeMP41))
// 	if err != nil {
// 		return err
// 	}
// 	return d.writeMoov(w)
// }

func (m *Muxer) AddTrack(cid MP4_CODEC_TYPE) *Track {
	track := &Track{
		Cid:       cid,
		TrackId:   m.nextTrackId,
		Timescale: 1000,
	}
	if m.isFragment() || m.isDash() {
		track.writer = NewFmp4WriterSeeker(1024 * 1024)
	}
	m.Tracks[m.nextTrackId] = track
	m.nextTrackId++
	return track
}

func (m *FMP4Muxer) WriteSample(t *Track, sample Sample) (err error) {
	if sample.Offset, err = t.writer.Seek(0, io.SeekCurrent); err != nil {
		return
	}
	if sample.Size, err = t.writer.Write(sample.Data); err != nil {
		return
	}
	sample.Data = nil
	t.AddSampleEntry(sample)

	// isKeyFrag := muxer.movFlag.has(MP4_FLAG_KEYFRAME)
	// if isKeyFrag {
	// 	if data.KeyFrame && track.duration > 0 {
	// 		err = muxer.flushFragment()
	// 		if err != nil {
	// 			return
	// 		}
	// 		if muxer.onNewFragment != nil {
	// 			muxer.onNewFragment(track.duration, track.startPts, track.startDts)
	// 		}
	// 	}
	// }
	return
}

func (m *FileMuxer) WriteSample(t *Track, sample Sample) (err error) {
	return m.Muxer.WriteSample(m.File, t, sample)
}

func (m *Muxer) WriteSample(w io.Writer, t *Track, sample Sample) (err error) {
	if len(sample.Data) == 0 {
		return errors.New("sample data is empty")
	}
	sample.Offset = m.CurrentOffset
	sample.Size, err = w.Write(sample.Data)
	if err != nil {
		return
	}
	m.CurrentOffset += int64(sample.Size)
	sample.Data = nil
	t.AddSampleEntry(sample)
	return
}

func (m *FileMuxer) reWriteMdatSize() (err error) {
	m.mdatSize = uint64(m.CurrentOffset) - (m.mdatOffset)
	if m.mdatSize+BasicBoxLen > 0xFFFFFFFF {
		_, mdatBox := MediaDataBox(m.mdatSize).Encode()
		if _, err = m.Seek(int64(m.mdatOffset-16), io.SeekStart); err != nil {
			return
		}
		if _, err = m.Write(mdatBox); err != nil {
			return
		}
		if _, err = m.Seek(m.CurrentOffset, io.SeekStart); err != nil {
			return
		}
	} else {
		if _, err = m.Seek(int64(m.mdatOffset-8), io.SeekStart); err != nil {
			return
		}
		tmpdata := make([]byte, 4)
		binary.BigEndian.PutUint32(tmpdata, uint32(m.mdatSize)+BasicBoxLen)
		if _, err = m.Write(tmpdata); err != nil {
			return
		}
		if _, err = m.Seek(m.CurrentOffset, io.SeekStart); err != nil {
			return
		}
	}
	return
}

func (m *FileMuxer) ReWriteWithMoov(f *os.File) (err error) {
	_, err = m.Seek(0, io.SeekStart)
	if err != nil {
		return
	}
	_, err = io.CopyN(f, m, int64(m.mdatOffset)-16)
	if err != nil {
		return
	}
	for _, track := range m.Tracks {
		for i := range len(track.Samplelist) {
			track.Samplelist[i].Offset += int64(m.moov.Size)
		}
	}
	err = m.WriteMoov(f)
	if err != nil {
		return
	}
	_, err = io.CopyN(f, m, int64(m.mdatSize)+16)
	return
}

func (m *Muxer) makeMvex() []byte {
	trexs := make([]byte, 0, 64)
	for i := uint32(1); i < m.nextTrackId; i++ {
		trex := NewTrackExtendsBox(m.Tracks[i].TrackId)
		trex.DefaultSampleDescriptionIndex = 1
		_, boxData := trex.Encode()
		trexs = append(trexs, boxData...)
	}
	return trexs
}

func (m *Muxer) makeTrak(track *Track) []byte {
	edts := []byte{}
	if m.isDash() || m.isFragment() {
		// track.makeEmptyStblTable()
	} else {
		if len(track.Samplelist) > 0 {
			track.makeStblTable()
			edts = track.makeEdtsBox()
		}
	}

	tkhd := track.makeTkhdBox()
	mdia := track.makeMdiaBox()

	trak := BasicBox{Type: TypeTRAK}
	trak.Size = 8 + uint64(len(tkhd)+len(edts)+len(mdia))
	offset, trakBox := trak.Encode()
	copy(trakBox[offset:], tkhd)
	offset += len(tkhd)
	copy(trakBox[offset:], edts)
	offset += len(edts)
	copy(trakBox[offset:], mdia)
	return trakBox
}

func (m *Muxer) GetMoovSize() int {
	moovsize := FullBoxLen + 96
	if m.isDash() || m.isFragment() {
		moovsize += 64
	}
	traks := make([][]byte, len(m.Tracks))
	for i := uint32(1); i < m.nextTrackId; i++ {
		traks[i-1] = m.makeTrak(m.Tracks[i])
		moovsize += len(traks[i-1])
	}
	return int(8 + uint64(moovsize))
}

func (m *Muxer) WriteMoov(w io.Writer) (err error) {
	var mvhd []byte
	var mvex []byte
	if m.isDash() || m.isFragment() {
		mvhd = MakeMvhdBox(m.nextTrackId, 0)
		mvex = m.makeMvex()
	} else {
		maxdurtaion := uint32(0)
		for _, track := range m.Tracks {
			if maxdurtaion < track.Duration {
				maxdurtaion = track.Duration
			}
		}
		mvhd = MakeMvhdBox(m.nextTrackId, maxdurtaion)
	}
	moovsize := len(mvhd) + len(mvex)
	traks := make([][]byte, len(m.Tracks))
	for i := uint32(1); i < m.nextTrackId; i++ {
		traks[i-1] = m.makeTrak(m.Tracks[i])
		moovsize += len(traks[i-1])
	}

	moov := BasicBox{Type: TypeMOOV}
	moov.Size = 8 + uint64(moovsize)
	offset, moovBox := moov.Encode()
	copy(moovBox[offset:], mvhd)
	offset += len(mvhd)
	for _, trak := range traks {
		copy(moovBox[offset:], trak)
		offset += len(trak)
	}
	copy(moovBox[offset:], mvex)
	_, err = w.Write(moovBox)
	m.moov = &moov
	return
}

func (m *FMP4Muxer) WriteTrailer() (err error) {
	err = m.flushFragment()
	if err != nil {
		return err
	}
	//for _, track := range m.Tracks {
	//	if track.Cid.IsAudio() {
	//		continue
	//	}
	//}
	return m.writeMfra()
}

func (m *FileMuxer) WriteTrailer() (err error) {
	if err = m.reWriteMdatSize(); err != nil {
		return err
	}
	return m.WriteMoov(m.File)
}

func (m *FMP4Muxer) writeMfra() (err error) {
	mfraSize := 0
	tfras := make([][]byte, len(m.Tracks))
	for i := uint32(1); i < m.nextTrackId; i++ {
		tfras[i-1] = m.Tracks[i].makeTfraBox()
		mfraSize += len(tfras[i-1])
	}

	mfro := MakeMfroBox(uint32(mfraSize) + 16)
	mfraSize += len(mfro)
	mfra := BasicBox{Type: TypeMFRA}
	mfra.Size = 8 + uint64(mfraSize)
	offset, mfraBox := mfra.Encode()
	for _, tfra := range tfras {
		copy(mfraBox[offset:], tfra)
		offset += len(tfra)
	}
	copy(mfraBox[offset:], mfro)
	_, err = m.writer.Write(mfraBox)
	return
}

func (m *FMP4Muxer) flushFragment() (err error) {

	if m.isFragment() {
		if m.nextFragmentId == 1 { //first fragment ,write moov
			_, err := m.writer.Write(MakeFtypBox(TypeISO5, 0x200, TypeISO5, TypeISO6, TypeMP41))
			if err != nil {
				return err
			}
			m.WriteMoov(m.writer)
		}
	}

	var moofOffset int64
	if moofOffset, err = m.writer.Seek(0, io.SeekCurrent); err != nil {
		return err
	}
	var mdatlen uint64 = 0
	for i := uint32(1); i < m.nextTrackId; i++ {
		if len(m.Tracks[i].Samplelist) == 0 {
			continue
		}
		for j := 0; j < len(m.Tracks[i].Samplelist); j++ {
			m.Tracks[i].Samplelist[j].Offset += int64(mdatlen)
		}
		ws := m.Tracks[i].writer.(*Fmp4WriterSeeker)
		mdatlen += uint64(len(ws.Buffer))
	}
	mdatlen += 8

	moofSize := 0
	mfhd := MakeMfhdBox(m.nextFragmentId)

	moofSize += len(mfhd)
	trafs := make([][]byte, len(m.Tracks))
	for i := uint32(1); i < m.nextTrackId; i++ {
		traf := m.Tracks[i].makeTraf(moofOffset, 0)
		moofSize += len(traf)
		trafs[i-1] = traf
	}

	moofSize += 8 //moof box
	mfhd = MakeMfhdBox(m.nextFragmentId)
	trafs = make([][]byte, len(m.Tracks))
	for i := uint32(1); i < m.nextTrackId; i++ {
		traf := m.Tracks[i].makeTraf(moofOffset, int64(moofSize)+8) //moofSize + 8(mdat box)
		trafs[i-1] = traf
	}
	m.nextFragmentId++

	moof := BasicBox{Type: TypeMOOF}
	moof.Size = uint64(moofSize)
	offset, moofBox := moof.Encode()
	copy(moofBox[offset:], mfhd)
	offset += len(mfhd)
	for i := range trafs {
		copy(moofBox[offset:], trafs[i])
		offset += len(trafs[i])
	}

	mdat := BasicBox{Type: TypeMDAT}
	mdat.Size = 8
	_, mdatBox := mdat.Encode()

	if m.isDash() {
		_, err := m.writer.Write(MakeStypBox(TypeMSDH, 0, TypeMSDH, TypeMSIX))
		if err != nil {
			return err
		}

		for i := uint32(1); i < m.nextTrackId; i++ {
			sidx := m.Tracks[i].makeSidxBox(52*(m.nextTrackId-1-i), uint32(mdatlen)+uint32(len(moofBox))+52*(m.nextTrackId-i-1))
			_, err := m.writer.Write(sidx)
			if err != nil {
				return err
			}
		}
	}

	_, err = m.writer.Write(moofBox)
	if err != nil {
		return err
	}
	binary.BigEndian.PutUint32(mdatBox, uint32(mdatlen))
	_, err = m.writer.Write(mdatBox)
	if err != nil {
		return err
	}

	for i := uint32(1); i < m.nextTrackId; i++ {
		if len(m.Tracks[i].Samplelist) > 0 {
			firstPts := m.Tracks[i].Samplelist[0].PTS
			firstDts := m.Tracks[i].Samplelist[0].DTS
			lastPts := m.Tracks[i].Samplelist[len(m.Tracks[i].Samplelist)-1].PTS
			lastDts := m.Tracks[i].Samplelist[len(m.Tracks[i].Samplelist)-1].DTS
			frag := Fragment{
				Offset:   uint64(moofOffset),
				Duration: m.Tracks[i].Duration,
				FirstDts: firstDts,
				FirstPts: firstPts,
				LastPts:  lastPts,
				LastDts:  lastDts,
			}
			m.Tracks[i].fragments = append(m.Tracks[i].fragments, frag)
		}
		ws := m.Tracks[i].writer.(*Fmp4WriterSeeker)
		_, err = m.writer.Write(ws.Buffer)
		if err != nil {
			return err
		}
		ws.Buffer = ws.Buffer[:0]
		ws.Offset = 0
		m.Tracks[i].Samplelist = m.Tracks[i].Samplelist[:0]
	}
	return nil
}
