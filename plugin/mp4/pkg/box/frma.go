package box

import (
	"io"
)

func decodeFrmaBox(demuxer *MovDemuxer, size uint32) (err error) {
	buf := make([]byte, size-BasicBoxLen)
	if _, err = io.ReadFull(demuxer.reader, buf); err != nil {
		return
	}
	track := demuxer.tracks[len(demuxer.tracks)-1]
	switch *(*[4]byte)(buf) {
	case TypeAVC1:
		track.cid = MP4_CODEC_H264
		if track.extra == nil {
			track.extra = new(h264ExtraData)
		}
		return
	case TypeMP4A:
		track.cid = MP4_CODEC_AAC
		if track.extra == nil {
			track.extra = new(aacExtraData)
		}
		return
	}

	return
}
