package flv

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	m7s "m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
)

type (
	RecordReader struct {
		m7s.RecordFilePuller
		reader *util.BufReader
	}
)

func NewPuller(conf config.Pull) m7s.IPuller {
	if strings.HasPrefix(conf.URL, "http") || strings.HasSuffix(conf.URL, ".flv") {
		p := &Puller{}
		p.SetDescription(task.OwnerTypeKey, "FlvPuller")
		return p
	}
	if conf.Args.Get(util.StartKey) != "" {
		p := &RecordReader{}
		p.Type = "flv"
		p.SetDescription(task.OwnerTypeKey, "FlvRecordReader")
		return p
	}
	return nil
}

func (p *RecordReader) Dispose() {
	if p.reader != nil {
		p.reader.Recycle()
	}
	p.RecordFilePuller.Dispose()
}

func (p *RecordReader) Run() (err error) {
	pullJob := &p.PullJob
	publisher := pullJob.Publisher
	if publisher == nil {
		return pkg.ErrDisabled
	}
	allocator := util.NewScalableMemoryAllocator(1 << 10)
	writer := m7s.NewPublisherWriter[*rtmp.AudioFrame, *rtmp.VideoFrame](publisher, allocator)
	var tagHeader [11]byte
	var ts int64
	var realTime time.Time
	var seekPosition int64
	var seekTsOffset int64
	// defer allocator.Recycle()
	defer func() {
		allocator.Recycle()
	}()
	publisher.OnGetPosition = func() time.Time {
		return realTime
	}

	for loop := 0; loop < p.Loop; loop++ {
	nextStream:
		for i, stream := range p.Streams {
			seekTsOffset = ts
			if p.File != nil {
				p.File.Close()
			}
			p.File, err = os.Open(stream.FilePath)
			if err != nil {
				continue
			}
			if p.reader != nil {
				p.reader.Recycle()
			}
			p.reader = util.NewBufReader(p.File)
			var head util.Memory
			head, err = p.reader.ReadBytes(9)
			if err != nil {
				return
			}
			var flvHead [3]byte
			var version, flag byte
			r := head.NewReader()
			err = r.ReadByteTo(&flvHead[0], &flvHead[1], &flvHead[2], &version, &flag)
			hasAudio := (flag & 0x04) != 0
			hasVideo := (flag & 0x01) != 0
			if err != nil {
				return
			}
			if !hasAudio {
				publisher.NoAudio()
			}
			if !hasVideo {
				publisher.NoVideo()
			}
			if flvHead != [3]byte{'F', 'L', 'V'} {
				return errors.New("not flv file")
			}

			startTimestamp := int64(0)
			if i == 0 {
				startTimestamp = p.PullStartTime.Sub(stream.StartTime).Milliseconds()
				if startTimestamp < 0 {
					startTimestamp = 0
				}
			}

			for {
				if p.IsStopped() {
					return p.StopReason()
				}
				if publisher.Paused != nil {
					publisher.Paused.Await()
				}

				if needSeek, err := p.CheckSeek(); err != nil {
					continue
				} else if needSeek {
					goto nextStream
				}

				if _, err = p.reader.ReadBE(4); err != nil { // previous tag size
					break
				}
				// Read tag header (11 bytes total)
				if err = p.reader.ReadNto(11, tagHeader[:]); err != nil {
					break
				}
				t := tagHeader[0]                                                            // tag type (1 byte)
				dataSize := int(tagHeader[1])<<16 | int(tagHeader[2])<<8 | int(tagHeader[3]) // data size (3 bytes)
				timestamp := uint32(tagHeader[4])<<16 | uint32(tagHeader[5])<<8 | uint32(tagHeader[6]) | uint32(tagHeader[7])<<24
				// stream id is tagHeader[8:11] (3 bytes), always 0

				ts = int64(timestamp)
				if i != 0 || seekPosition == 0 {
					ts += seekTsOffset
				}
				realTime = stream.StartTime.Add(time.Duration(timestamp) * time.Millisecond)
				switch t {
				case FLV_TAG_TYPE_AUDIO:
					if publisher.PubAudio {
						frame := writer.AudioFrame
						err = p.reader.ReadNto(dataSize, frame.NextN(dataSize))
						if err != nil {
							return err
						}
						frame.SetTS32(uint32(ts))
						if err = writer.NextAudio(); err != nil {
							return err
						}
					} else {
						p.reader.Skip(dataSize)
					}
				case FLV_TAG_TYPE_VIDEO:
					if publisher.PubVideo {
						frame := writer.VideoFrame
						err = p.reader.ReadNto(dataSize, frame.NextN(dataSize))
						if err != nil {
							return err
						}
						frame.SetTS32(uint32(ts))
						if err = writer.NextVideo(); err != nil {
							return err
						}
						// After processing the first video frame, check if we need to seek
						if i == 0 && seekPosition > 0 {
							_, err = p.File.Seek(seekPosition, io.SeekStart)
							if err != nil {
								return
							}
							p.reader.Recycle()
							p.reader = util.NewBufReader(p.File)
							seekPosition = 0 // Reset to avoid seeking again
						}
					}
				case FLV_TAG_TYPE_SCRIPT:
					buf := allocator.Borrow(dataSize)
					amf := rtmp.AMF(buf)
					var obj any
					if obj, err = amf.Unmarshal(); err != nil {
						return
					}
					name := obj
					if obj, err = amf.Unmarshal(); err != nil {
						return
					}
					if i == 0 {
						if metaData, ok := obj.(rtmp.EcmaArray); ok {
							if keyframes, ok := metaData["keyframes"].(map[string]any); ok {
								filepositions := keyframes["filepositions"].([]any)
								times := keyframes["times"].([]any)
								for i, t := range times {
									if ts := int64(t.(float64) * 1000); ts > startTimestamp {
										if i < 2 {
											break
										}
										seekPosition = int64(filepositions[i-1].(float64) - 4)
										seekTsOffset = -int64(times[i-1].(float64) * 1000)
										break
									}
								}
							}
						}
					} else {
						p.Info("script", name, obj)
					}
				default:
					err = fmt.Errorf("unknown tag type: %d", t)
				}
				if err != nil {
					return
				}
				if p.MaxTS > 0 && ts > p.MaxTS {
					return
				}
			}
		}
	}
	return
}
