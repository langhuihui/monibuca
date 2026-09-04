package mp4

import (
	"bytes"
	"encoding/binary"
	"io"

	"m7s.live/v5/pkg/storage"
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
		moovWritten  bool      // FMP4：moov 是否已写入（须在首个 moof 之前）
		// fmp4：开片 moov 写入后记录可回写时长的文件偏移；0 表示不可用
		mvhdDurationOffset int64
		mehdDurationOffset int64
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
			KeyFrame: lastSample.KeyFrame,
		})
		t.Samplelist[0] = sample
	} else {
		t.Samplelist = append(t.Samplelist, sample)
	}
	return
}

func (m *Muxer) WriteSample(w io.Writer, t *Track, sample Sample) (err error) {
	if m.isFragment() {
		// 首个 sample 只缓存，CreateFlagment 返回 nil,nil；等下一帧才写 moof/mdat
		moof, mdat := m.CreateFlagment(t, sample)
		if moof == nil {
			return nil
		}
		// 可播 FMP4 要求 moov(含 mvex) 在首个 moof 之前
		if !m.moovWritten {
			if err = m.WriteMoov(w); err != nil {
				return
			}
			m.moovWritten = true
		}
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
	// 数据已写入磁盘，立即释放 Buffers 引用。
	// Samplelist 只需要元数据（Timestamp/Offset/Size/Duration/CTS/KeyFrame），
	// 不需要 Buffers 中存储的原始载荷字节切片。
	// 长 fragment（如 1 小时）期间 Samplelist 会积累大量 Sample，
	// 若保留 Buffers，其中的 []byte 切片头指向原始 RTP 包缓冲区，
	// 使 GC 堆目标持续升高、GC 频率降低，导致 make([]byte,size)
	// 分配的 buf（位于 NetConnection.Receive 调用栈）长期无法被回收，
	// 进而造成严重的内存泄漏。
	sample.Buffers = nil
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
	// Confirmed via 寸止: 预留 mehd，trailer 时回写真实 fragment_duration，改善 VLC/浏览器 seek
	mehd := CreateMovieExtendsHeaderBox(0)
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

// findMoovDurationFieldOffsets 在已序列化的 moov box 缓冲中定位 mvhd/mehd 的 duration 字段偏移（相对 moov 起始）。
func findMoovDurationFieldOffsets(moovBuf []byte) (mvhdDurOff, mehdDurOff int64) {
	mvhdDurOff, mehdDurOff = -1, -1
	if len(moovBuf) < 16 || string(moovBuf[4:8]) != "moov" {
		return
	}
	// 递归扫描 container，offset 为相对 moovBuf 起始
	var walk func(start, end int)
	walk = func(start, end int) {
		pos := start
		for pos+8 <= end {
			size := int(binary.BigEndian.Uint32(moovBuf[pos : pos+4]))
			if size < 8 || pos+size > end {
				return
			}
			typ := string(moovBuf[pos+4 : pos+8])
			switch typ {
			case "mvhd":
				// FullBox 头 12 字节后：creation(4)+modification(4)+timescale(4)+duration(4)（version 0）
				if moovBuf[pos+8] == 0 && pos+28 <= end {
					mvhdDurOff = int64(pos + 24)
				}
			case "mehd":
				// FullBox 头 12 字节后即为 fragment_duration(4)（version 0）
				if moovBuf[pos+8] == 0 && pos+16 <= end {
					mehdDurOff = int64(pos + 12)
				}
			case "moov", "trak", "mdia", "minf", "stbl", "edts", "mvex", "udta":
				walk(pos+8, pos+size)
			}
			pos += size
		}
	}
	walk(8, len(moovBuf))
	return
}

func (m *Muxer) WriteMoov(w io.Writer) (err error) {
	moov := m.MakeMoov()
	moovStart := m.CurrentOffset
	var buf bytes.Buffer
	var n int64
	n, err = WriteTo(&buf, moov)
	if err != nil {
		return
	}
	moovBytes := buf.Bytes()
	if m.isFragment() {
		mvhdRel, mehdRel := findMoovDurationFieldOffsets(moovBytes)
		if mvhdRel >= 0 {
			m.mvhdDurationOffset = moovStart + mvhdRel
		}
		if mehdRel >= 0 {
			m.mehdDurationOffset = moovStart + mehdRel
		}
	}
	if _, err = w.Write(moovBytes); err != nil {
		return
	}
	m.CurrentOffset += n
	m.moov = moov
	m.moovWritten = true
	return
}

// patchFMP4Duration 将实际媒体时长写回已落盘 moov 中的 mvhd/mehd（timescale=1000）。
func (m *Muxer) patchFMP4Duration(file storage.Writer, duration uint32) (err error) {
	if duration == 0 {
		return nil
	}
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], duration)
	if m.mvhdDurationOffset > 0 {
		if _, err = file.Seek(m.mvhdDurationOffset, io.SeekStart); err != nil {
			return
		}
		if _, err = file.Write(tmp[:]); err != nil {
			return
		}
	}
	if m.mehdDurationOffset > 0 {
		if _, err = file.Seek(m.mehdDurationOffset, io.SeekStart); err != nil {
			return
		}
		if _, err = file.Write(tmp[:]); err != nil {
			return
		}
	}
	// 写回文件末尾，避免影响后续逻辑对偏移的假设
	if _, err = file.Seek(0, io.SeekEnd); err != nil {
		return
	}
	return nil
}

func (m *Muxer) fmp4MediaDuration() uint32 {
	var maxTs uint64
	for i := uint32(1); i < m.nextTrackId; i++ {
		track := m.Tracks[i]
		if track == nil {
			continue
		}
		for _, f := range track.fragments {
			if f.LastTs > maxTs {
				maxTs = f.LastTs
			}
		}
		if len(track.Samplelist) > 0 {
			last := track.Samplelist[len(track.Samplelist)-1]
			ts := uint64(last.Timestamp + last.Duration)
			if ts > maxTs {
				maxTs = ts
			}
		}
	}
	if maxTs > 0xffffffff {
		return 0xffffffff
	}
	return uint32(maxTs)
}

func (m *Muxer) WriteTrailer(file storage.Writer) (err error) {
	if m.isFragment() {
		// 刷出各轨 Samplelist 中尚未形成 moof 的最后一个 sample
		for i := uint32(1); i < m.nextTrackId; i++ {
			track := m.Tracks[i]
			if track == nil || len(track.Samplelist) == 0 {
				continue
			}
			last := &track.Samplelist[0]
			dur := last.Duration
			if dur == 0 {
				dur = 33 // timescale=1000 时约 30fps
			}
			moof, mdat := m.CreateFlagment(track, Sample{Timestamp: last.Timestamp + dur})
			if moof == nil {
				continue
			}
			if !m.moovWritten {
				if err = m.WriteMoov(file); err != nil {
					return
				}
			}
			if _, err = WriteTo(file, moof, mdat); err != nil {
				return
			}
			track.Samplelist = track.Samplelist[:0]
		}
		var mfraChildren []IBox
		var mfraSize uint32 = 0
		tfras := make([]*TrackFragmentRandomAccessBox, len(m.Tracks))
		for i := uint32(1); i < m.nextTrackId; i++ {
			if track := m.Tracks[i]; track != nil && len(track.fragments) > 0 {
				tfras[i-1] = track.makeTfraBox()
				mfraChildren = append(mfraChildren, tfras[i-1])
				mfraSize += uint32(tfras[i-1].Size())
			}
		}
		if mfraSize > 0 {
			mfraChildren = append(mfraChildren, CreateMfroBox(uint32(mfraSize)+16))
			mfra := CreateContainerBox(TypeMFRA, mfraChildren...)
			_, err = WriteTo(file, mfra)
			if err != nil {
				return err
			}
		}
		// 回写 moov 内时长，使播放器不必仅依赖文件尾 mfra 才能 seek/显示时长
		if err = m.patchFMP4Duration(file, m.fmp4MediaDuration()); err != nil {
			return err
		}
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
