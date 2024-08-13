package flv

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
)

func PullFLV(p *m7s.PullContext) (err error) {
	var reader *util.BufReader
	var hasAudio, hasVideo bool
	var absTS uint32
	if strings.HasPrefix(p.RemoteURL, "http") {
		var res *http.Response
		client := http.DefaultClient
		if proxyConf := p.ConnectProxy; proxyConf != "" {
			proxy, err := url.Parse(proxyConf)
			if err != nil {
				return err
			}
			transport := &http.Transport{Proxy: http.ProxyURL(proxy)}
			client = &http.Client{Transport: transport}
		}
		if res, err = client.Get(p.RemoteURL); err == nil {
			if res.StatusCode != http.StatusOK {
				return io.EOF
			}
			defer res.Body.Close()
			reader = util.NewBufReader(res.Body)
		}
	} else {
		var res *os.File
		if res, err = os.Open(p.RemoteURL); err == nil {
			defer res.Close()
			reader = util.NewBufReader(res)
		}
	}
	if err == nil {
		var head util.Memory
		head, err = reader.ReadBytes(13)
		if err == nil {
			var flvHead [3]byte
			var version, flag byte
			var reader = head.NewReader()
			err = reader.ReadByteTo(&flvHead[0], &flvHead[1], &flvHead[2], &version, &flag)
			if flvHead != [...]byte{'F', 'L', 'V'} {
				err = errors.New("not flv file")
			} else {
				hasAudio = flag&0x04 != 0
				hasVideo = flag&0x01 != 0
			}
		}
	}

	var startTs uint32
	pubConf := p.Publisher.GetPublishConfig()
	if !hasAudio {
		pubConf.PubAudio = false
	}
	if !hasVideo {
		pubConf.PubVideo = false
	}
	allocator := util.NewScalableMemoryAllocator(1 << 10)
	for offsetTs := absTS; err == nil; _, err = reader.ReadBE(4) {
		t, err := reader.ReadByte()
		if err != nil {
			return err
		}
		dataSize, err := reader.ReadBE32(3)
		if err != nil {
			return err
		}
		timestamp, err := reader.ReadBE32(3)
		if err != nil {
			return err
		}
		h, err := reader.ReadByte()
		if err != nil {
			return err
		}
		timestamp = timestamp | uint32(h)<<24
		if startTs == 0 {
			startTs = timestamp
		}
		if _, err = reader.ReadBE(3); err != nil { // stream id always 0
			return err
		}
		var frame rtmp.RTMPData
		ds := int(dataSize)
		frame.SetAllocator(allocator)
		err = reader.ReadNto(ds, frame.NextN(ds))
		if err != nil {
			return err
		}
		absTS = offsetTs + (timestamp - startTs)
		frame.Timestamp = absTS
		//fmt.Println(t, offsetTs, timestamp, startTs, puller.absTS)
		switch t {
		case FLV_TAG_TYPE_AUDIO:
			err = p.Publisher.WriteAudio(frame.WrapAudio())
		case FLV_TAG_TYPE_VIDEO:
			err = p.Publisher.WriteVideo(frame.WrapVideo())
		case FLV_TAG_TYPE_SCRIPT:
			r := frame.NewReader()
			amf := &rtmp.AMF{
				Buffer: util.Buffer(r.ToBytes()),
			}
			var obj any
			obj, err = amf.Unmarshal()
			name := obj
			obj, err = amf.Unmarshal()
			metaData := obj
			frame.Recycle()
			if err != nil {
				return err
			}
			p.Info("script", name, metaData)
		}
	}
	return
}
