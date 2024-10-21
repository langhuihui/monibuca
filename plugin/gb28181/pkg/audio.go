package gb28181

import (
	"io"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
	mpegts "m7s.live/v5/plugin/hls/pkg/ts"
	"time"
)

type PSAudio struct {
	PTS, DTS uint32
	util.RecyclableMemory
	streamType byte
}

func (es *PSAudio) parsePESPacket(payload util.Memory) (result pkg.IAVFrame, err error) {
	if payload.Size < 4 {
		err = io.ErrShortBuffer
		return
	}
	var flag, pesHeaderDataLen byte
	reader := payload.NewReader()
	reader.Skip(1)
	//data_alignment_indicator := (payload[0]&0b0001_0000)>>4 == 1
	err = reader.ReadByteTo(&flag, &pesHeaderDataLen)
	if err != nil {
		return
	}
	ptsFlag := flag>>7 == 1
	dtsFlag := (flag&0b0100_0000)>>6 == 1
	if payload.Size < int(pesHeaderDataLen) {
		err = io.ErrShortBuffer
		return
	}
	var extraData []byte
	extraData, err = reader.ReadBytes(int(pesHeaderDataLen))
	pts, dts := es.PTS, es.DTS
	if ptsFlag && len(extraData) > 4 {
		pts = uint32(extraData[0]&0b0000_1110) << 29
		pts |= uint32(extraData[1]) << 22
		pts |= uint32(extraData[2]&0b1111_1110) << 14
		pts |= uint32(extraData[3]) << 7
		pts |= uint32(extraData[4]) >> 1
		if dtsFlag && len(extraData) > 9 {
			dts = uint32(extraData[5]&0b0000_1110) << 29
			dts |= uint32(extraData[6]) << 22
			dts |= uint32(extraData[7]&0b1111_1110) << 14
			dts |= uint32(extraData[8]) << 7
			dts |= uint32(extraData[9]) >> 1
		} else {
			dts = pts
		}
	}
	if pts != es.PTS && es.Memory.Size > 0 {
		switch es.streamType {
		case mpegts.STREAM_TYPE_AAC:
			var adts = &pkg.ADTS{
				DTS: time.Duration(es.PTS),
			}
			adts.Memory.CopyFrom(&es.Memory)
			result = adts
		case mpegts.STREAM_TYPE_G711A:
			rawAudio := &pkg.RawAudio{
				Timestamp: time.Duration(es.PTS) * time.Millisecond / 90,
				FourCC:    codec.FourCC_ALAW,
			}
			rawAudio.Memory.CopyFrom(&es.Memory)
			result = rawAudio
		case mpegts.STREAM_TYPE_G711U:
			rawAudio := &pkg.RawAudio{
				Timestamp: time.Duration(es.PTS) * time.Millisecond / 90,
				FourCC:    codec.FourCC_ULAW,
			}
			rawAudio.Memory.CopyFrom(&es.Memory)
			result = rawAudio
		}
		es.Recycle()
		es.Memory = util.Memory{}
	}
	es.PTS, es.DTS = pts, dts
	reader.Range(es.AppendOne)
	return
}
