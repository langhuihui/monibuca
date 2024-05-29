package hdl

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

type HDLPuller struct {
	*util.BufReader
	hasAudio bool
	hasVideo bool
	absTS    uint32 //绝对时间戳
}

func NewHDLPuller() *HDLPuller {
	return &HDLPuller{}
}

func (puller *HDLPuller) Connect(p *m7s.Client) (err error) {
	if strings.HasPrefix(p.RemoteURL, "http") {
		var res *http.Response
		client := http.DefaultClient
		if proxyConf := p.Proxy; proxyConf != "" {
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
			p.Closer = res.Body
			puller.BufReader = util.NewBufReader(res.Body)
		}
	} else {
		var res *os.File
		if res, err = os.Open(p.RemoteURL); err == nil {
			p.Closer = res
			puller.BufReader = util.NewBufReader(res)
		}
	}
	if err == nil {
		var head util.RecyclableMemory
		head, err = puller.BufReader.ReadBytes(13)
		defer head.Recycle()
		if err == nil {
			var flvHead [3]byte
			var version, flag byte
			var reader = head.NewReader()
			reader.ReadByteTo(&flvHead[0], &flvHead[1], &flvHead[2], &version, &flag)
			if flvHead != [...]byte{'F', 'L', 'V'} {
				err = errors.New("not flv file")
			} else {
				puller.hasAudio = flag&0x04 != 0
				puller.hasVideo = flag&0x01 != 0
			}
		}
	}
	return
}

func (puller *HDLPuller) Pull(p *m7s.Puller) (err error) {
	var startTs uint32
	pubConf := p.GetPublishConfig()
	if !puller.hasAudio {
		pubConf.PubAudio = false
	}
	if !puller.hasVideo {
		pubConf.PubVideo = false
	}
	for offsetTs := puller.absTS; err == nil; _, err = puller.ReadBE(4) {
		t, err := puller.ReadByte()
		if err != nil {
			return err
		}
		dataSize, err := puller.ReadBE32(3)
		if err != nil {
			return err
		}
		timestamp, err := puller.ReadBE32(3)
		if err != nil {
			return err
		}
		h, err := puller.ReadByte()
		if err != nil {
			return err
		}
		timestamp = timestamp | uint32(h)<<24
		if startTs == 0 {
			startTs = timestamp
		}
		puller.ReadBE(3) // stream id always 0
		var frame rtmp.RTMPData
		frame.RecyclableMemory, err = puller.ReadBytes(int(dataSize))
		if err != nil {
			frame.Recycle()
			return err
		}
		puller.absTS = offsetTs + (timestamp - startTs)
		frame.Timestamp = puller.absTS
		// fmt.Println(t, offsetTs, timestamp, startTs, puller.absTS)
		switch t {
		case FLV_TAG_TYPE_AUDIO:
			p.WriteAudio(frame.WrapAudio())
		case FLV_TAG_TYPE_VIDEO:
			p.WriteVideo(frame.WrapVideo())
		case FLV_TAG_TYPE_SCRIPT:
			p.Info("script")
			frame.Recycle()
		}
	}
	return
}
