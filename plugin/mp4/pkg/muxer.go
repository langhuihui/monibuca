package mp4

import (
	"encoding/binary"
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
		maxdurtaion  uint32
		moov         IBox
		mdatOffset   uint64
		mdatSize     uint64
		StreamPath   string    // Added to store the stream path
		Metadata     *Metadata // 添加元数据支持
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
		Metadata:       &Metadata{Custom: make(map[string]string)},
	}
}

// NewMuxerWithStreamPath creates a new muxer with the specified stream path
func NewMuxerWithStreamPath(flag Flag, streamPath string) *Muxer {
	muxer := NewMuxer(flag)
	muxer.StreamPath = streamPath
	muxer.Metadata.Producer = "M7S Live"
	muxer.Metadata.Album = streamPath
	return muxer
}

func (m *Muxer) CreateFTYPBox() *FileTypeBox {
	if m.isFragment() {
		return CreateFTYPBox(TypeISOM, 1, TypeISOM, TypeAVC1)
	}
	return CreateFTYPBox(TypeISOM, 0x200, TypeISOM, TypeISO2, TypeAVC1, TypeMP41)
}

func (m *Muxer) WriteInitSegment(w io.Writer) (err error) {
	m.CurrentOffset, err = WriteTo(w, m.CreateFTYPBox())
	if err != nil {
		return
	}
	if !m.isFragment() {
		var n int64
		freeBox := CreateFreeBox(nil)
		n, err = WriteTo(w, freeBox)
		if err != nil {
			return
		}
		m.CurrentOffset += n
		mdat := CreateDataBox(TypeMDAT, nil)
		n, err = WriteTo(w, mdat)
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
		// track.writer = NewFmp4WriterSeeker(1024 * 1024)
		track.isFragment = true
	}
	m.Tracks[m.nextTrackId] = track
	m.nextTrackId++
	return track
}

func (m *Muxer) CreateFlagment(t *Track, sample Sample) (moof IBox, mdat IBox) {
	if len(t.Samplelist) > 0 {
		lastSample := &t.Samplelist[0]
		lastSample.Duration = sample.Timestamp - lastSample.Timestamp
		m.nextFragmentId++
		// Create moof box for this track
		moof = t.MakeMoof(m.nextFragmentId)
		// Create mdat box for this track
		mdat = CreateMemoryBox(TypeMDAT, lastSample.Memory)

		moofOffset := m.CurrentOffset
		m.CurrentOffset += int64(moof.Size() + mdat.Size())
		t.fragments = append(t.fragments, Fragment{
			Offset:   uint64(moofOffset),
			Duration: lastSample.Duration,
			FirstTs:  uint64(lastSample.Timestamp),
			LastTs:   uint64(sample.Timestamp),
		})
		t.Samplelist[0] = sample
	} else {
		t.Samplelist = append(t.Samplelist, sample)
	}
	return
}

func (m *Muxer) WriteSample(w io.Writer, t *Track, sample Sample) (err error) {
	if m.isFragment() {
		moof, mdat := m.CreateFlagment(t, sample)
		_, err = WriteTo(w, moof, mdat)
		return
	}
	// For regular MP4, write directly to output
	sample.Offset = m.CurrentOffset
	_, err = sample.WriteTo(w)
	if err != nil {
		return
	}
	m.CurrentOffset += int64(sample.Size)
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
		if _, err = WriteTo(w, mdat); err != nil {
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

func (m *Muxer) makeMvex() *MovieExtendsBox {
	trexs := make([]*TrackExtendsBox, 0, m.nextTrackId-1)
	for i := uint32(1); i < m.nextTrackId; i++ {
		if track := m.Tracks[i]; track != nil {
			trex := CreateTrackExtendsBox(track.TrackId)
			trex.DefaultSampleDescriptionIndex = 1
			// if track.Cid.IsVideo() {
			// 	trex.DefaultSampleFlags = 0x01010000
			// } else {
			// 	trex.DefaultSampleFlags = 0x02000000
			// }
			trexs = append(trexs, trex)
		}
	}
	// mehd := CreateMovieExtendsHeaderBox(m.maxdurtaion)
	var mehd *MovieExtendsHeaderBox
	return CreateMovieExtendsBox(mehd, trexs)
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

func (m *Muxer) MakeMoov() IBox {
	mvhd := CreateMovieHeaderBox(m.nextTrackId, 0)
	children := []IBox{mvhd}
	for _, track := range m.Tracks {
		children = append(children, m.makeTrak(track))
		if m.maxdurtaion < track.Duration {
			m.maxdurtaion = track.Duration
		}
	}
	mvhd.Duration = uint64(m.maxdurtaion)
	if m.isDash() || m.isFragment() {
		children = append(children, m.makeMvex())
	}

	// Add user data box with metadata if available
	metadataEntries := CreateMetadataEntries(m.Metadata)
	if len(metadataEntries) > 0 {
		udta := CreateUserDataBox(metadataEntries...)
		children = append(children, udta)
	}

	m.moov = CreateContainerBox(TypeMOOV, children...)
	return m.moov
}

func (m *Muxer) WriteMoov(w io.Writer) (err error) {
	var n int64
	n, err = WriteTo(w, m.MakeMoov())
	m.CurrentOffset += n
	return
}

func (m *Muxer) WriteTrailer(file *os.File) (err error) {
	if m.isFragment() {
		// Flush any remaining samples
		// if err = m.flushFragment(file); err != nil {
		// 	return err
		// }
		var mfraChildren []IBox
		var mfraSize uint32 = 0
		// Write mfra box
		tfras := make([]*TrackFragmentRandomAccessBox, len(m.Tracks))
		for i := uint32(1); i < m.nextTrackId; i++ {
			if track := m.Tracks[i]; track != nil && len(track.fragments) > 0 {
				tfras[i-1] = track.makeTfraBox()
				mfraChildren = append(mfraChildren, tfras[i-1])
				mfraSize += uint32(tfras[i-1].Size())
			}
		}

		// Only write mfra if we have fragments
		if mfraSize > 0 {
			mfraChildren = append(mfraChildren, CreateMfroBox(uint32(mfraSize)+16))
			mfra := CreateContainerBox(TypeMFRA, mfraChildren...)
			_, err = WriteTo(file, mfra)
			if err != nil {
				return err
			}
		}
		// Clean up any remaining buffers
		// for i := uint32(1); i < m.nextTrackId; i++ {
		// 	if track := m.Tracks[i]; track != nil && track.writer != nil {
		// 		if ws, ok := track.writer.(*Fmp4WriterSeeker); ok {
		// 			ws.Buffer = nil
		// 		}
		// 	}
		// }
	} else {
		if err = m.reWriteMdatSize(file); err != nil {
			return err
		}
		return m.WriteMoov(file)
	}
	return nil
}

// func (m *Muxer) flushFragment(w io.Writer) (err error) {
// 	// Check if there are any samples to write
// 	hasSamples := false
// 	for i := uint32(1); i < m.nextTrackId; i++ {
// 		if len(m.Tracks[i].Samplelist) > 0 {
// 			hasSamples = true
// 			break
// 		}
// 	}
// 	if !hasSamples {
// 		return nil
// 	}

// 	// Write moov box if not written yet
// 	if m.moov == nil {
// 		if err = m.WriteMoov(w); err != nil {
// 			return err
// 		}
// 	}

// 	// Process each track separately
// 	for i := uint32(1); i < m.nextTrackId; i++ {
// 		track := m.Tracks[i]
// 		if len(track.Samplelist) == 0 {
// 			continue
// 		}

// 		ws := track.writer.(*Fmp4WriterSeeker)

// 		// Create moof box for this track
// 		moof := track.MakeMoof(m.nextFragmentId)

// 		// Create mdat box for this track
// 		mdat := CreateDataBox(TypeMDAT, ws.Buffer)

// 		// Write moof box
// 		var n int64
// 		n, err = WriteTo(w, moof, mdat)
// 		if err != nil {
// 			return err
// 		}
// 		m.CurrentOffset += n

// 		// Record fragment info
// 		if len(track.Samplelist) > 0 {
// 			firstTs := track.Samplelist[0].Timestamp
// 			lastTs := track.Samplelist[len(track.Samplelist)-1].Timestamp
// 			frag := Fragment{
// 				Offset:   uint64(int64(moof.Size()) + int64(mdat.HeaderSize())), // Start of moof
// 				Duration: track.Duration,
// 				FirstTs:  uint64(firstTs),
// 				LastTs:   uint64(lastTs),
// 			}
// 			track.fragments = append(track.fragments, frag)
// 		}

// 		// Clear track buffers
// 		ws.Buffer = ws.Buffer[:0]
// 		ws.Offset = 0
// 		track.Samplelist = track.Samplelist[:0]
// 		track.Duration = 0
// 	}

// 	m.nextFragmentId++
// 	return nil
// }

// SetFragmentDuration sets the target duration for each fragment in milliseconds
func (m *Muxer) SetFragmentDuration(duration uint32) {
	m.fragDuration = duration
}

// SetMetadata sets the metadata for the MP4 file
func (m *Muxer) SetMetadata(metadata *Metadata) {
	m.Metadata = metadata
	if metadata.Custom == nil {
		metadata.Custom = make(map[string]string)
	}
}

// SetTitle sets the title metadata
func (m *Muxer) SetTitle(title string) {
	m.Metadata.Title = title
}

// SetArtist sets the artist/author metadata
func (m *Muxer) SetArtist(artist string) {
	m.Metadata.Artist = artist
}

// SetAlbum sets the album metadata
func (m *Muxer) SetAlbum(album string) {
	m.Metadata.Album = album
}

// SetComment sets the comment/description metadata
func (m *Muxer) SetComment(comment string) {
	m.Metadata.Comment = comment
}

// SetGenre sets the genre metadata
func (m *Muxer) SetGenre(genre string) {
	m.Metadata.Genre = genre
}

// SetCopyright sets the copyright metadata
func (m *Muxer) SetCopyright(copyright string) {
	m.Metadata.Copyright = copyright
}

// SetEncoder sets the encoder metadata
func (m *Muxer) SetEncoder(encoder string) {
	m.Metadata.Encoder = encoder
}

// SetDate sets the date metadata (format: YYYY-MM-DD)
func (m *Muxer) SetDate(date string) {
	m.Metadata.Date = date
}

// SetCurrentDate sets the date metadata to current date
func (m *Muxer) SetCurrentDate() {
	m.Metadata.Date = GetCurrentDateString()
}

// AddCustomMetadata adds custom key-value metadata
func (m *Muxer) AddCustomMetadata(key, value string) {
	if m.Metadata.Custom == nil {
		m.Metadata.Custom = make(map[string]string)
	}
	m.Metadata.Custom[key] = value
}

// SetKeywords sets the keywords metadata
func (m *Muxer) SetKeywords(keywords string) {
	m.Metadata.Keywords = keywords
}

// SetLocation sets the location metadata
func (m *Muxer) SetLocation(location string) {
	m.Metadata.Location = location
}

// SetRating sets the rating metadata (0-5)
func (m *Muxer) SetRating(rating uint8) {
	if rating > 5 {
		rating = 5
	}
	m.Metadata.Rating = rating
}
