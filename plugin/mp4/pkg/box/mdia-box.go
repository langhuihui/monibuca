package box

func makeMdiaBox(track *mp4track) []byte {
	mdhdbox := makeMdhdBox(track.duration)
	hdlrbox := makeHdlrBox(getHandlerType(track.cid))
	minfbox := makeMinfBox(track)
	mdia := BasicBox{Type: [4]byte{'m', 'd', 'i', 'a'}}
	mdia.Size = 8 + uint64(len(mdhdbox)+len(hdlrbox)+len(minfbox))
	offset, mdiabox := mdia.Encode()
	copy(mdiabox[offset:], mdhdbox)
	offset += len(mdhdbox)
	copy(mdiabox[offset:], hdlrbox)
	offset += len(hdlrbox)
	copy(mdiabox[offset:], minfbox)
	offset += len(minfbox)
	return mdiabox
}
