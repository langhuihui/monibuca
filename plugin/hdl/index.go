package plugin_hdl

import (
	"encoding/binary"
	"net"
	"net/http"
	"strings"
	"time"

	"m7s.live/m7s/v5"
	. "m7s.live/m7s/v5/pkg"
	. "m7s.live/m7s/v5/plugin/hdl/pkg"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
)

type HDLPlugin struct {
	m7s.Plugin
}

const defaultConfig m7s.DefaultYaml = `publish:
  speed: 1`

func (p *HDLPlugin) OnInit() error {
	for streamPath, url := range p.GetCommonConf().PullOnStart {
		go p.Pull(streamPath, url, NewHDLPuller())
	}
	return nil
}

var _ = m7s.InstallPlugin[HDLPlugin](defaultConfig)

func (p *HDLPlugin) WriteFlvHeader(sub *m7s.Subscriber) (flv net.Buffers) {
	at, vt := &sub.Publisher.AudioTrack, &sub.Publisher.VideoTrack
	hasAudio, hasVideo := at.AVTrack != nil && sub.SubAudio, vt.AVTrack != nil && sub.SubVideo
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
		metaData["width"] = ctx.GetWidth()
		metaData["height"] = ctx.GetHeight()
	}
	var data = amf.Marshal(metaData)
	var b [15]byte
	WriteFLVTag(FLV_TAG_TYPE_SCRIPT, 0, uint32(len(data)), b[:])
	flv = append(flv, []byte{'F', 'L', 'V', 0x01, flags, 0, 0, 0, 9, 0, 0, 0, 0}, b[:11], data, b[11:])
	binary.BigEndian.PutUint32(b[11:], uint32(len(data))+11)
	return
}

func (p *HDLPlugin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	streamPath := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/"), ".flv")
	if r.URL.RawQuery != "" {
		streamPath += "?" + r.URL.RawQuery
	}

	sub, err := p.Subscribe(streamPath, w, r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "video/x-flv")
	w.Header().Set("Transfer-Encoding", "identity")
	w.WriteHeader(http.StatusOK)
	wto := p.GetCommonConf().WriteTimeout
	var gotFlvTag func(net.Buffers) error
	var b [15]byte

	if hijacker, ok := w.(http.Hijacker); ok && wto > 0 {
		conn, _, _ := hijacker.Hijack()
		conn.SetWriteDeadline(time.Now().Add(wto))
		sub.Closer = conn
		gotFlvTag = func(flv net.Buffers) (err error) {
			conn.SetWriteDeadline(time.Now().Add(wto))
			_, err = flv.WriteTo(conn)
			return
		}
	} else {
		gotFlvTag = func(flv net.Buffers) (err error) {
			_, err = flv.WriteTo(w)
			return
		}
		w.(http.Flusher).Flush()
	}
	flv := p.WriteFlvHeader(sub)
	copy(b[:4], flv[3])
	gotFlvTag(flv[:3])
	rtmpData2FlvTag := func(t byte, data *rtmp.RTMPData) error {
		WriteFLVTag(t, data.Timestamp, uint32(data.Size), b[4:])
		defer binary.BigEndian.PutUint32(b[:4], uint32(data.Size)+11)
		return gotFlvTag(append(net.Buffers{b[:]}, data.Memory.Buffers...))
	}
	m7s.PlayBlock(sub, func(audio *rtmp.RTMPAudio) error {
		return rtmpData2FlvTag(FLV_TAG_TYPE_AUDIO, &audio.RTMPData)
	}, func(video *rtmp.RTMPVideo) error {
		return rtmpData2FlvTag(FLV_TAG_TYPE_VIDEO, &video.RTMPData)
	})
	gotFlvTag(net.Buffers{b[:4]})
}
