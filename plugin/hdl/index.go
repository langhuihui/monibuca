package plugin_hdl

import (
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
	. "m7s.live/m7s/v5/plugin/hdl/pkg"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
)

type HDLPlugin struct {
	m7s.Plugin
}

var _ = m7s.InstallPlugin[HDLPlugin]()

func (p *HDLPlugin) WriteFlvHeader(sub *m7s.Subscriber, w io.Writer) {
	// at, vt := sub.Publisher, sub.Video
	// hasAudio, hasVideo := at != nil, vt != nil
	// var amf rtmp.AMF
	// amf.Marshal("onMetaData")
	// metaData := rtmp.EcmaArray{
	// 	"MetaDataCreator": "m7s" + m7s.Version,
	// 	"hasVideo":        hasVideo,
	// 	"hasAudio":        hasAudio,
	// 	"hasMatadata":     true,
	// 	"canSeekToEnd":    false,
	// 	"duration":        0,
	// 	"hasKeyFrames":    0,
	// 	"framerate":       0,
	// 	"videodatarate":   0,
	// 	"filesize":        0,
	// }
	var flags byte
	// if hasAudio {
	flags |= (1 << 2)
	// 	metaData["audiocodecid"] = int(at.CodecID)
	// 	metaData["audiosamplerate"] = at.SampleRate
	// 	metaData["audiosamplesize"] = at.SampleSize
	// 	metaData["stereo"] = at.Channels == 2
	// }
	// if hasVideo {
	flags |= 1
	// 	metaData["videocodecid"] = int(vt.CodecID)
	// 	metaData["width"] = vt.SPSInfo.Width
	// 	metaData["height"] = vt.SPSInfo.Height
	// }
	// amf.Marshal(metaData)
	// 写入FLV头
	w.Write([]byte{'F', 'L', 'V', 0x01, flags, 0, 0, 0, 9, 0, 0, 0, 0})
	// codec.WriteFLVTag(w, codec.FLV_TAG_TYPE_SCRIPT, 0, amf.Buffer)
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
	var gotFlvTag func(tag *net.Buffers) error
	if hijacker, ok := w.(http.Hijacker); ok && wto > 0 {
		conn, _, _ := hijacker.Hijack()
		conn.SetWriteDeadline(time.Now().Add(wto))
		sub.Closer = conn
		p.WriteFlvHeader(sub, conn)
		gotFlvTag = func(tag *net.Buffers) (err error) {
			conn.SetWriteDeadline(time.Now().Add(wto))
			_, err = tag.WriteTo(conn)
			return
		}
	} else {
		w.(http.Flusher).Flush()
		p.WriteFlvHeader(sub, w)
		gotFlvTag = func(tag *net.Buffers) (err error) {
			_, err = tag.WriteTo(w)
			return
		}
	}
	b := util.Buffer(make([]byte, 0, 15))
	var flv net.Buffers
	sub.Handle(func(audio *rtmp.RTMPAudio) error {
		b.Reset()
		b.WriteByte(FLV_TAG_TYPE_AUDIO)
		dataSize := audio.Length
		b.WriteUint24(uint32(dataSize))
		b.WriteUint24(audio.Timestamp)
		b.WriteByte(byte(audio.Timestamp >> 24))
		b.WriteUint24(0)
		flv = append(append(append(flv, b), audio.Buffers.Buffers...), util.PutBE(b.Malloc(4), dataSize+11))
		return gotFlvTag(&flv)
	}, func(video *rtmp.RTMPVideo) error {
		b.Reset()
		b.WriteByte(FLV_TAG_TYPE_VIDEO)
		dataSize := video.Length
		b.WriteUint24(uint32(dataSize))
		b.WriteUint24(video.Timestamp)
		b.WriteByte(byte(video.Timestamp >> 24))
		b.WriteUint24(0)
		flv = append(append(append(flv, b), video.Buffers.Buffers...), util.PutBE(b.Malloc(4), dataSize+11))
		return gotFlvTag(&flv)
	})
}
