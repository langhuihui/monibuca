package box

func makeTrak(track *mp4track, movflag MP4_FLAG) []byte {

	edts := []byte{}
	if movflag.isDash() || movflag.isFragment() {
		track.makeEmptyStblTable()
	} else {
		if len(track.samplelist) > 0 {
			track.makeStblTable()
			edts = makeEdtsBox(track)
		}
	}

	tkhd := makeTkhdBox(track)
	mdia := makeMdiaBox(track)

	trak := BasicBox{Type: [4]byte{'t', 'r', 'a', 'k'}}
	trak.Size = 8 + uint64(len(tkhd)+len(edts)+len(mdia))
	offset, trakBox := trak.Encode()
	copy(trakBox[offset:], tkhd)
	offset += len(tkhd)
	copy(trakBox[offset:], edts)
	offset += len(edts)
	copy(trakBox[offset:], mdia)
	return trakBox
}
