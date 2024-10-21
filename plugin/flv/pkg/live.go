package flv

import (
	"encoding/binary"
	"m7s.live/v5"
	. "m7s.live/v5/pkg"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
	"net"
)

type Live struct {
	b           [15]byte
	Subscriber  *m7s.Subscriber
	WriteFlvTag func(net.Buffers) error
}

func (task *Live) WriteFlvHeader() (err error) {
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
	WriteFLVTagHead(FLV_TAG_TYPE_SCRIPT, 0, uint32(len(data)), task.b[4:])
	defer binary.BigEndian.PutUint32(task.b[:4], uint32(len(data))+11)
	return task.WriteFlvTag(net.Buffers{[]byte{'F', 'L', 'V', 0x01, flags, 0, 0, 0, 9, 0, 0, 0, 0}, task.b[4:], data})
}

func (task *Live) rtmpData2FlvTag(t byte, data *rtmp.RTMPData) error {
	WriteFLVTagHead(t, data.Timestamp, uint32(data.Size), task.b[4:])
	defer binary.BigEndian.PutUint32(task.b[:4], uint32(data.Size)+11)
	return task.WriteFlvTag(append(net.Buffers{task.b[:]}, data.Memory.Buffers...))
}

func (task *Live) WriteAudioTag(data *rtmp.RTMPAudio) error {
	return task.rtmpData2FlvTag(FLV_TAG_TYPE_AUDIO, &data.RTMPData)
}

func (task *Live) WriteVideoTag(data *rtmp.RTMPVideo) error {
	return task.rtmpData2FlvTag(FLV_TAG_TYPE_VIDEO, &data.RTMPData)
}

func (task *Live) Run() (err error) {
	err = task.WriteFlvHeader()
	if err != nil {
		return
	}
	err = m7s.PlayBlock(task.Subscriber, func(audio *rtmp.RTMPAudio) error {
		return task.WriteAudioTag(audio)
	}, func(video *rtmp.RTMPVideo) error {
		return task.WriteVideoTag(video)
	})
	if err != nil {
		return
	}
	return task.WriteFlvTag(net.Buffers{task.b[:4]})
}
