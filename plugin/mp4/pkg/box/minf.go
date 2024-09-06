package box

func makeMinfBox(track *mp4track) []byte {
	var mhdbox []byte
	switch track.cid {
	case MP4_CODEC_H264, MP4_CODEC_H265:
		mhdbox = makeVmhdBox()
	case MP4_CODEC_G711A, MP4_CODEC_G711U, MP4_CODEC_AAC,
		MP4_CODEC_MP2, MP4_CODEC_MP3, MP4_CODEC_OPUS:
		mhdbox = makeSmhdBox()
	default:
		panic("unsupport codec id")
	}
	dinfbox := makeDefaultDinfBox()
	stblbox := makeStblBox(track)

	minf := BasicBox{Type: [4]byte{'m', 'i', 'n', 'f'}}
	minf.Size = 8 + uint64(len(mhdbox)+len(dinfbox)+len(stblbox))
	offset, minfbox := minf.Encode()
	copy(minfbox[offset:], mhdbox)
	offset += len(mhdbox)
	copy(minfbox[offset:], dinfbox)
	offset += len(dinfbox)
	copy(minfbox[offset:], stblbox)
	offset += len(stblbox)
	return minfbox
}
