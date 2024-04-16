package hdl

import (
	"bufio"
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
	*bufio.Reader
	hasAudio bool
	hasVideo bool
	absTS    uint32 //绝对时间戳
	pool     *util.ScalableMemoryAllocator
}

func NewHDLPuller() *HDLPuller {
	return &HDLPuller{
		pool: util.NewScalableMemoryAllocator(1024),
	}
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
			puller.Reader = bufio.NewReader(res.Body)
		}
	} else {
		var res *os.File
		if res, err = os.Open(p.RemoteURL); err == nil {
			p.Closer = res
			puller.Reader = bufio.NewReader(res)
		}
	}
	if err == nil {
		header := puller.pool.Malloc(13)
		defer puller.pool.Free(header)
		if _, err = io.ReadFull(puller, header); err == nil {
			if header[0] != 'F' || header[1] != 'L' || header[2] != 'V' {
				err = errors.New("not flv file")
			} else {
				puller.hasAudio = header[4]&0x04 != 0
				puller.hasVideo = header[4]&0x01 != 0
			}
		}
	}
	return
}

func (puller *HDLPuller) Pull(p *m7s.Puller) (err error) {
	var startTs uint32
	var buf15 [15]byte
	pubConf := p.GetPublishConfig()
	if !puller.hasAudio {
		pubConf.PubAudio = false
	}
	if !puller.hasVideo {
		pubConf.PubVideo = false
	}
	pubaudio, pubvideo := pubConf.PubAudio, pubConf.PubVideo
	for offsetTs := puller.absTS; err == nil; _, err = io.ReadFull(puller, buf15[11:]) {
		tmp := util.Buffer(buf15[:11])
		_, err = io.ReadFull(puller, tmp)
		if err != nil {
			return
		}
		t := tmp.ReadByte()
		dataSize := tmp.ReadUint24()
		timestamp := tmp.ReadUint24() | uint32(tmp.ReadByte())<<24
		if startTs == 0 {
			startTs = timestamp
		}
		tmp.ReadUint24()
		var frame rtmp.RTMPData
		frame.ScalableMemoryAllocator = puller.pool
		mem := frame.Malloc(int(dataSize))
		_, err = io.ReadFull(puller, mem)
		if err != nil {
			frame.Recycle()
			return
		}
		frame.ReadFromBytes(mem)
		puller.absTS = offsetTs + (timestamp - startTs)
		frame.Timestamp = puller.absTS
		// fmt.Println(t, offsetTs, timestamp, startTs, puller.absTS)
		switch t {
		case FLV_TAG_TYPE_AUDIO:
			if pubaudio {
				p.WriteAudio(&rtmp.RTMPAudio{frame})
			}
		case FLV_TAG_TYPE_VIDEO:
			if pubvideo {
				p.WriteVideo(&rtmp.RTMPVideo{frame})
			}
		case FLV_TAG_TYPE_SCRIPT:
			p.Info("script", "data", mem)
			frame.Recycle()
		}
	}
	return
}
