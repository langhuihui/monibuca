package mp4

import (
	"encoding/binary"
	"io"
	"net"
	"os"

	"m7s.live/v5/pkg"
	"m7s.live/v5/plugin/mp4/pkg/box"
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
		moov         IBox
		mdatOffset   uint64
		mdatSize     uint64
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

func NewMuxer(flag Flag) *Muxer {
	return &Muxer{
		nextTrackId:    1,
		nextFragmentId: 1,
		Tracks:         make(map[uint32]*Track),
		Flag:           flag,
		fragDuration:   2000,
	}
}

func (m *Muxer) WriteInitSegment(w io.Writer) (err error) {
	var ftypBox *FileTypeBox
	if m.isFragment() {
		// 对于 FMP4,使用 iso5 作为主品牌,兼容 iso5, iso6, mp41
		ftypBox = CreateFTYPBox(TypeISO5, 0x200, TypeISO5, TypeISO6, TypeMP41)
	} else {
		// 对于普通 MP4,使用 isom 作为主品牌
		ftypBox = CreateFTYPBox(TypeISOM, 0x200, TypeISOM, TypeISO2, TypeAVC1, TypeMP41)
	}
	m.CurrentOffset, err = box.WriteTo(w, ftypBox)
	if err != nil {
		return
	}
	if !m.isFragment() {
		var n int64
		freeBox := CreateFreeBox(nil)
		n, err = box.WriteTo(w, freeBox)
		if err != nil {
			return
		}
		m.CurrentOffset += n
		mdat := CreateDataBox(TypeMDAT, nil)
		n, err = box.WriteTo(w, mdat)
		if err != nil {
			return
		}
		m.mdatOffset = uint64(m.CurrentOffset + 8)
		m.mdatSize = 0
		m.CurrentOffset += n
	}
	return
}

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

func (m *Muxer) WriteSample(w io.Writer, t *Track, sample Sample) (err error) {
	if m.isFragment() {
		// For fragmented MP4, write to track's buffer
		if sample.Offset, err = t.writer.Seek(0, io.SeekCurrent); err != nil {
			return
		}
		if sample.Size, err = t.writer.Write(sample.Data); err != nil {
			return
		}
		defer func() {
			// For fragmented MP4, check if we should create a new fragment
			if sample.KeyFrame && t.Duration >= m.fragDuration {
				err = m.flushFragment(w)
			}
		}()
	} else {
		// For regular MP4, write directly to output
		sample.Offset = m.CurrentOffset
		sample.Size, err = w.Write(sample.Data)
		if err != nil {
			return
		}
		m.CurrentOffset += int64(sample.Size)
	}
	sample.Data = nil
	t.AddSampleEntry(sample)
	return
}

func (m *Muxer) reWriteMdatSize(w io.WriteSeeker) (err error) {
	m.mdatSize = uint64(m.CurrentOffset) - (m.mdatOffset)
	if m.mdatSize+BasicBoxLen > 0xFFFFFFFF {
		mdat := CreateBaseBox(TypeMDAT, m.mdatSize+BasicBoxLen)
		// 覆盖FreeBox
		if _, err = w.Seek(int64(m.mdatOffset-16), io.SeekStart); err != nil {
			return
		}
		if _, err = box.WriteTo(w, mdat); err != nil {
			return
		}
		if _, err = w.Seek(m.CurrentOffset, io.SeekStart); err != nil {
			return
		}
	} else {
		if _, err = w.Seek(int64(m.mdatOffset-8), io.SeekStart); err != nil {
			return
		}
		tmpdata := make([]byte, 4)
		binary.BigEndian.PutUint32(tmpdata, uint32(m.mdatSize)+BasicBoxLen)
		if _, err = w.Write(tmpdata); err != nil {
			return
		}
		if _, err = w.Seek(m.CurrentOffset, io.SeekStart); err != nil {
			return
		}
	}
	return
}

func (m *Muxer) ReWriteWithMoov(f io.WriteSeeker, r io.Reader) (err error) {
	if m.isFragment() {
		return pkg.ErrSkip
	}
	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		return
	}
	_, err = io.CopyN(f, r, int64(m.mdatOffset)-16)
	if err != nil {
		return
	}
	for _, track := range m.Tracks {
		for i := range len(track.Samplelist) {
			track.Samplelist[i].Offset += int64(m.moov.Size())
		}
	}
	err = m.WriteMoov(f)
	if err != nil {
		return
	}
	_, err = io.CopyN(f, r, int64(m.mdatSize)+16)
	return
}

func (m *Muxer) makeMvex() *box.MovieExtendsBox {
	trexs := make([]*box.TrackExtendsBox, 0, m.nextTrackId-1)
	for i := uint32(1); i < m.nextTrackId; i++ {
		if track := m.Tracks[i]; track != nil {

			trex := box.CreateTrackExtendsBox(track.TrackId)
			trex.DefaultSampleDescriptionIndex = 1
			if track.Cid.IsVideo() {
				trex.DefaultSampleFlags = 0x00010000 // NonSyncSampleFlags in mp4ff
			} else {
				trex.DefaultSampleFlags = 0x02000000 // SyncSampleFlags in mp4ff
			}
			trexs = append(trexs, trex)
		}
	}
	return box.CreateMovieExtendsBox(trexs)
}

func (m *Muxer) makeTrak(track *Track) *ContainerBox {
	var edts *ContainerBox
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
	return CreateContainerBox(TypeTRAK, tkhd, mdia, edts)
}

func (m *Muxer) GetMoovSize() int {
	moovsize := uint64(FullBoxLen + 96)
	if m.isDash() || m.isFragment() {
		moovsize += 64
	}
	for _, track := range m.Tracks {
		moovsize += uint64(m.makeTrak(track).Size())
	}
	return int(8 + moovsize)
}

func (m *Muxer) WriteMoov(w io.Writer) (err error) {
	var mvhd *box.MovieHeaderBox
	var mvex *box.MovieExtendsBox
	var children []IBox
	maxdurtaion := uint32(0)
	for _, track := range m.Tracks {
		children = append(children, m.makeTrak(track))
		if maxdurtaion < track.Duration {
			maxdurtaion = track.Duration
		}
	}
	if m.isDash() || m.isFragment() {
		mvhd = box.CreateMovieHeaderBox(m.nextTrackId, 0)
		mvex = m.makeMvex()
		children = append(children, mvex)
	} else {
		mvhd = box.CreateMovieHeaderBox(m.nextTrackId, maxdurtaion)
	}
	children = append(children, mvhd)
	m.moov = box.CreateContainerBox(TypeMOOV, children...)
	var n int64
	n, err = box.WriteTo(w, m.moov)
	m.CurrentOffset += n
	return
}

func (m *Muxer) WriteTrailer(file *os.File) (err error) {
	if m.isFragment() {
		// Flush any remaining samples
		if err = m.flushFragment(file); err != nil {
			return err
		}
		var mfraChildren []box.IBox
		var mfraSize uint32 = 0
		// Write mfra box
		tfras := make([]*box.TrackFragmentRandomAccessBox, len(m.Tracks))
		for i := uint32(1); i < m.nextTrackId; i++ {
			if track := m.Tracks[i]; track != nil && len(track.fragments) > 0 {
				tfras[i-1] = track.makeTfraBox()
				mfraChildren = append(mfraChildren, tfras[i-1])
				mfraSize += uint32(tfras[i-1].Size())
			}
		}

		// Only write mfra if we have fragments
		if mfraSize > 0 {
			mfraChildren = append(mfraChildren, box.CreateMfroBox(uint32(mfraSize)+16))
			mfra := box.CreateContainerBox(TypeMFRA, mfraChildren...)
			_, err = box.WriteTo(file, mfra)
			if err != nil {
				return err
			}
		}
		// Clean up any remaining buffers
		for i := uint32(1); i < m.nextTrackId; i++ {
			if track := m.Tracks[i]; track != nil && track.writer != nil {
				if ws, ok := track.writer.(*Fmp4WriterSeeker); ok {
					ws.Buffer = nil
				}
			}
		}
	} else {
		if err = m.reWriteMdatSize(file); err != nil {
			return err
		}
		return m.WriteMoov(file)
	}
	return nil
}

func (m *Muxer) flushFragment(w io.Writer) (err error) {
	// Check if there are any samples to write
	hasSamples := false
	for i := uint32(1); i < m.nextTrackId; i++ {
		if len(m.Tracks[i].Samplelist) > 0 {
			hasSamples = true
			break
		}
	}
	if !hasSamples {
		return nil
	}

	// Write moov box if not written yet
	if m.moov == nil {
		if err = m.WriteMoov(w); err != nil {
			return err
		}
	}
	// Calculate mdat size first
	var mdatSize uint64 = 8 // mdat box header
	for i := uint32(1); i < m.nextTrackId; i++ {
		if len(m.Tracks[i].Samplelist) == 0 {
			continue
		}
		ws := m.Tracks[i].writer.(*Fmp4WriterSeeker)
		mdatSize += uint64(len(ws.Buffer))
	}

	// Write moof box
	mfhdBox := box.CreateMovieFragmentHeaderBox(m.nextFragmentId)
	trafs := make([]*box.TrackFragmentBox, len(m.Tracks))
	moofChildren := make([]box.IBox, 0, len(m.Tracks)+1)
	moofChildren = append(moofChildren, mfhdBox)
	for i := uint32(1); i < m.nextTrackId; i++ {
		if len(m.Tracks[i].Samplelist) == 0 {
			continue
		}
		track := m.Tracks[i]
		// 传递 moof 偏移和 mdat 大小
		traf := track.makeTraf() // +8 for moof box header
		// Record trun data_offset position: current offset + 16 (after trun header)
		trafs[i-1] = traf
		moofChildren = append(moofChildren, traf)
	}

	moof := CreateContainerBox(TypeMOOF, moofChildren...)

	sampleData := make(net.Buffers, len(m.Tracks))
	// Write sample data
	var sampleOffset int64 = 0
	for i := uint32(1); i < m.nextTrackId; i++ {
		if len(m.Tracks[i].Samplelist) == 0 {
			continue
		}

		track := m.Tracks[i]
		ws := track.writer.(*Fmp4WriterSeeker)

		// Update sample offsets relative to mdat start
		for j := range track.Samplelist {
			track.Samplelist[j].Offset = sampleOffset
			sampleOffset += int64(track.Samplelist[j].Size)
		}

		sampleData[i-1] = ws.Buffer

		// Record fragment info
		if len(track.Samplelist) > 0 {
			firstPts := track.Samplelist[0].PTS
			firstDts := track.Samplelist[0].DTS
			lastPts := track.Samplelist[len(track.Samplelist)-1].PTS
			lastDts := track.Samplelist[len(track.Samplelist)-1].DTS
			frag := Fragment{
				Offset:   uint64(m.CurrentOffset),
				Duration: track.Duration,
				FirstDts: firstDts,
				FirstPts: firstPts,
				LastPts:  lastPts,
				LastDts:  lastDts,
			}
			track.fragments = append(track.fragments, frag)
		}

		// Clear track buffers
		ws.Buffer = ws.Buffer[:0]
		ws.Offset = 0
		track.Samplelist = track.Samplelist[:0]
		track.Duration = 0
	}

	// Write mdat box
	mdat := CreateBaseBox(TypeMDAT, uint64(sampleOffset)+BasicBoxLen)

	for _, traf := range trafs {
		traf.TRUN.DataOffset = int32(moof.Size()) + int32(mdat.HeaderSize())
	}

	var n int64
	n, err = box.WriteTo(w, moof)
	if err != nil {
		return err
	}
	m.CurrentOffset += n
	n, err = mdat.HeaderWriteTo(w)
	if err != nil {
		return err
	}
	m.CurrentOffset += n
	n, err = sampleData.WriteTo(w)
	if err != nil {
		return err
	}
	m.CurrentOffset += n
	m.nextFragmentId++
	return nil
}

// SetFragmentDuration sets the target duration for each fragment in milliseconds
func (m *Muxer) SetFragmentDuration(duration uint32) {
	m.fragDuration = duration
}
