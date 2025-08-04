package flv

import (
	"errors"
	"io"

	"m7s.live/v5"
	pkg "m7s.live/v5/pkg"
	"m7s.live/v5/pkg/util"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
)

type Puller struct {
	m7s.HTTPFilePuller
}

func (p *Puller) Run() (err error) {
	pullJob := &p.PullJob
	// Move to parsing step
	pullJob.GoToStepConst(pkg.StepParsing)

	reader := util.NewBufReader(p.ReadCloser)
	publisher := p.PullJob.Publisher
	if publisher == nil {
		io.Copy(io.Discard, p.ReadCloser)
		return
	}
	var hasAudio, hasVideo bool
	var absTS uint32
	var head util.Memory
	head, err = reader.ReadBytes(13)
	if err == nil {
		var flvHead [3]byte
		var version, flag byte
		r := head.NewReader()
		err = r.ReadByteTo(&flvHead[0], &flvHead[1], &flvHead[2], &version, &flag)
		if flvHead != [...]byte{'F', 'L', 'V'} {
			err = errors.New("not flv file")
		} else {
			hasAudio = flag&0x04 != 0
			hasVideo = flag&0x01 != 0
		}
	}
	var startTs uint32
	pubConf := &publisher.Publish
	if !hasAudio {
		pubConf.PubAudio = false
	}
	if !hasVideo {
		pubConf.PubVideo = false
	}
	allocator := util.NewScalableMemoryAllocator(1 << 10)
	defer allocator.Recycle()
	writer := m7s.NewPublisherWriter[*rtmp.AudioFrame, *rtmp.VideoFrame](publisher, allocator)

	// Move to streaming step
	pullJob.GoToStepConst(pkg.StepStreaming)

	for offsetTs := absTS; err == nil; _, err = reader.ReadBE(4) {
		if p.IsStopped() {
			return p.StopReason()
		}
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
		ds := int(dataSize)
		absTS = offsetTs + (timestamp - startTs)
		//fmt.Println(t, offsetTs, timestamp, startTs, puller.absTS)
		switch t {
		case FLV_TAG_TYPE_AUDIO:
			if publisher.PubAudio {
				frame := writer.AudioFrame
				_, err = reader.Read(frame.NextN(ds))
				if err != nil {
					return err
				}
				frame.SetTS32(absTS)
				if err = writer.NextAudio(); err != nil {
					return err
				}
			} else {
				reader.Skip(ds)
			}
		case FLV_TAG_TYPE_VIDEO:
			if publisher.PubVideo {
				frame := writer.VideoFrame
				_, err = reader.Read(frame.NextN(ds))
				if err != nil {
					return err
				}
				frame.SetTS32(absTS)
				if err = writer.NextVideo(); err != nil {
					return err
				}
			} else {
				reader.Skip(ds)
			}
		case FLV_TAG_TYPE_SCRIPT:
			var amf rtmp.AMF = allocator.Borrow(ds)
			_, err = reader.Read(amf)
			if err != nil {
				return err
			}
			var name, metaData any
			name, err = amf.Unmarshal()
			if err != nil {
				return err
			}
			metaData, err = amf.Unmarshal()
			if err != nil {
				return err
			}
			publisher.Info("script", name, metaData)
		}
	}
	return
}
