package flv

import (
	"encoding/binary"
	"m7s.live/m7s/v5"
	. "m7s.live/m7s/v5/pkg"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
	"net"
)

type Live struct {
	Subscriber  *m7s.Subscriber
	WriteFlvTag func(net.Buffers) error
}

func (task *Live) WriteFlvHeader() (flv net.Buffers) {
	at, vt := &task.Subscriber.Publisher.AudioTrack, &task.Subscriber.Publisher.VideoTrack
	hasAudio, hasVideo := at.AVTrack != nil && task.Subscriber.SubAudio, vt.AVTrack != nil && task.Subscriber.SubVideo
	var amf rtmp.AMF
	amf.Marshal("onMetaData")
	metaData := rtmp.EcmaArray{
		"MetaDataCreator": "m7s" + m7s.Version,
		"hasVideo":        hasVideo,
		"hasAudio":        hasAudio,
		"hasMatadata":     true,
		"canSeekToEnd":    false,
		"duration":        0,
		"hasKeyFrames":    0,
		"framerate":       0,
		"videodatarate":   0,
		"filesize":        0,
	}
	var flags byte
	if hasAudio {
		flags |= (1 << 2)
		metaData["audiocodecid"] = int(rtmp.ParseAudioCodec(at.FourCC()))
		ctx := at.ICodecCtx.(IAudioCodecCtx)
		metaData["audiosamplerate"] = ctx.GetSampleRate()
		metaData["audiosamplesize"] = ctx.GetSampleSize()
		metaData["stereo"] = ctx.GetChannels() == 2
	}
	if hasVideo {
		flags |= 1
		metaData["videocodecid"] = int(rtmp.ParseVideoCodec(vt.FourCC()))
		ctx := vt.ICodecCtx.(IVideoCodecCtx)
		metaData["width"] = ctx.Width()
		metaData["height"] = ctx.Height()
	}
	var data = amf.Marshal(metaData)
	var b [15]byte
	WriteFLVTagHead(FLV_TAG_TYPE_SCRIPT, 0, uint32(len(data)), b[:])
	flv = append(flv, []byte{'F', 'L', 'V', 0x01, flags, 0, 0, 0, 9, 0, 0, 0, 0}, b[:11], data, b[11:])
	binary.BigEndian.PutUint32(b[11:], uint32(len(data))+11)
	return
}

func (task *Live) Run() (err error) {
	var b [15]byte
	flv := task.WriteFlvHeader()
	copy(b[:4], flv[3])
	err = task.WriteFlvTag(flv[:3])
	rtmpData2FlvTag := func(t byte, data *rtmp.RTMPData) error {
		WriteFLVTagHead(t, data.Timestamp, uint32(data.Size), b[4:])
		defer binary.BigEndian.PutUint32(b[:4], uint32(data.Size)+11)
		return task.WriteFlvTag(append(net.Buffers{b[:]}, data.Memory.Buffers...))
	}
	err = m7s.PlayBlock(task.Subscriber, func(audio *rtmp.RTMPAudio) error {
		return rtmpData2FlvTag(FLV_TAG_TYPE_AUDIO, &audio.RTMPData)
	}, func(video *rtmp.RTMPVideo) error {
		return rtmpData2FlvTag(FLV_TAG_TYPE_VIDEO, &video.RTMPData)
	})
	return task.WriteFlvTag(net.Buffers{b[:4]})
}
