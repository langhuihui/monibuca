package box

func makeMvex(muxer *Movmuxer) []byte {
	trexs := make([]byte, 0, 64)
	for i := uint32(1); i < muxer.nextTrackId; i++ {
		trex := NewTrackExtendsBox(muxer.tracks[i].trackId)
		trex.DefaultSampleDescriptionIndex = 1
		_, boxData := trex.Encode()
		trexs = append(trexs, boxData...)
	}
	mvex := BasicBox{Type: [4]byte{'m', 'v', 'e', 'x'}}
	mvex.Size = 8 + uint64(len(trexs))
	offset, mvexBox := mvex.Encode()
	copy(mvexBox[offset:], trexs)
	return mvexBox
}
