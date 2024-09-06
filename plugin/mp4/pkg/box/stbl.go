package box

func makeStblBox(track *mp4track) []byte {
	var stsdbox []byte
	var sttsbox []byte
	var cttsbox []byte
	var stscbox []byte
	var stszbox []byte
	var stcobox []byte
	var stssbox []byte
	stsdbox = makeStsd(track, getHandlerType(track.cid))
	if track.stbltable != nil {
		if track.stbltable.stts != nil {
			sttsbox = makeStts(track.stbltable.stts)
		}
		if track.stbltable.ctts != nil {
			cttsbox = makeCtts(track.stbltable.ctts)
		}
		if track.stbltable.stsc != nil {
			stscbox = makeStsc(track.stbltable.stsc)
		}
		if track.stbltable.stsz != nil {
			stszbox = makeStsz(track.stbltable.stsz)
		}
		if track.stbltable.stco != nil {
			stcobox = makeStco(track.stbltable.stco)
		}
		if track.cid == MP4_CODEC_H264 || track.cid == MP4_CODEC_H265 {
			stssbox = makeStss(track)
		}
	}

	stbl := BasicBox{Type: [4]byte{'s', 't', 'b', 'l'}}
	stbl.Size = uint64(8 + len(stsdbox) + len(sttsbox) + len(cttsbox) + len(stscbox) + len(stszbox) + len(stcobox) + len(stssbox))
	offset, stblbox := stbl.Encode()
	copy(stblbox[offset:], stsdbox)
	offset += len(stsdbox)
	copy(stblbox[offset:], sttsbox)
	offset += len(sttsbox)
	copy(stblbox[offset:], cttsbox)
	offset += len(cttsbox)
	copy(stblbox[offset:], stscbox)
	offset += len(stscbox)
	copy(stblbox[offset:], stszbox)
	offset += len(stszbox)
	copy(stblbox[offset:], stcobox)
	offset += len(stcobox)
	copy(stblbox[offset:], stssbox)
	offset += len(stssbox)
	return stblbox
}
