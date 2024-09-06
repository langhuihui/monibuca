package record

import (
	"database/sql"
	"fmt"
	"github.com/deepch/vdk/codec/h264parser"
	"github.com/deepch/vdk/codec/h265parser"
	"gorm.io/gorm"
	"io"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/db"
	"m7s.live/m7s/v5/pkg/util"
	"os"
	"path/filepath"
	"time"
)

func NewPuller() m7s.IPuller {
	return &Puller{}
}

type Puller struct {
	m7s.HTTPFilePuller
	PullStartTime time.Time
	allocator     *util.ScalableMemoryAllocator
	db            *sql.DB
}

func (p *Puller) Start() (err error) {
	if err = p.PullJob.Publish(); err != nil {
		return
	}

	if p.PullStartTime, err = util.TimeQueryParse(p.PullJob.Args.Get("start")); err != nil {
		return
	}
	return
}

func (p *Puller) Run() (err error) {
	p.allocator = util.NewScalableMemoryAllocator(1 << 10)
	var streams []*RecordStream
	tx := p.PullJob.Plugin.DB.Find(&streams, "start_time<=? AND end_time>? AND file_path=?", p.PullStartTime, p.PullStartTime, p.PullJob.RemoteURL)
	if tx.Error != nil {
		return tx.Error
	}
	var startTimestamp int64
	beginTime := time.Now()
	speedControl := func(ts int64) {
		targetTime := time.Duration(float64(time.Since(beginTime)) * p.PullJob.Publisher.Speed)
		sleepTime := time.Duration(ts-startTimestamp)*time.Millisecond - targetTime
		fmt.Println("sleepTime", sleepTime)
		if sleepTime > 0 {
			time.Sleep(sleepTime)
		}
	}
	for _, stream := range streams {
		dbType := p.PullJob.Plugin.GetCommonConf().DBType
		if factory, ok := db.Factory[dbType]; ok {
			var streamDB *gorm.DB
			streamDB, err = gorm.Open(factory(filepath.Join(p.PullJob.RemoteURL, fmt.Sprintf("%d.db", stream.ID))), &gorm.Config{})
			if err != nil {
				return
			}
			if p.db != nil {
				p.db.Close()
			}
			p.db, err = streamDB.DB()
			var file *os.File
			file, err = os.Open(filepath.Join(p.PullJob.RemoteURL, fmt.Sprintf("%d.rec", stream.ID)))
			if err != nil {
				return
			}
			if p.ReadCloser != nil {
				p.ReadCloser.Close()
				p.ReadCloser = file
			}
			startTimestamp = p.PullStartTime.Sub(stream.StartTime).Milliseconds()
			hasAudio, hasVideo := stream.AudioCodec != "", stream.VideoCodec != ""
			audioFourCC, videoFourCC := codec.ParseFourCC(stream.AudioCodec), codec.ParseFourCC(stream.VideoCodec)
			var startId uint
			if hasAudio && audioFourCC == codec.FourCC_MP4A {
				var rawAudio pkg.RawAudio
				rawAudio.FourCC = audioFourCC
				rawAudio.Memory.AppendOne(stream.AudioConfig)
				err = p.PullJob.Publisher.WriteAudio(&rawAudio)
			}
			if hasVideo {
				var rawVideo pkg.H26xFrame
				rawVideo.FourCC = videoFourCC
				switch videoFourCC {
				case codec.FourCC_H264:
					conf, _ := h264parser.NewCodecDataFromAVCDecoderConfRecord(stream.VideoConfig)
					rawVideo.Nalus.Append(conf.SPS())
					rawVideo.Nalus.Append(conf.PPS())
				case codec.FourCC_H265:
					conf, _ := h265parser.NewCodecDataFromAVCDecoderConfRecord(stream.VideoConfig)
					rawVideo.Nalus.Append(conf.VPS())
					rawVideo.Nalus.Append(conf.SPS())
					rawVideo.Nalus.Append(conf.PPS())
				}
				err = p.PullJob.Publisher.WriteVideo(&rawVideo)
				var keyFrame Sample
				tx = streamDB.Last(&keyFrame, "type=? AND timestamp<=?", FRAME_TYPE_VIDEO_KEY_FRAME, startTimestamp)
				if tx.Error != nil {
					return tx.Error
				}
				startId = keyFrame.ID
			} else {
				// TODO
			}
			rows, err := streamDB.Model(&Sample{}).Where("id>=? ", startId).Rows()
			if err != nil {
				return err
			}
			for rows.Next() {
				var frame Sample
				streamDB.ScanRows(rows, &frame)
				switch frame.Type {
				case FRAME_TYPE_AUDIO:
					var rawAudio pkg.RawAudio
					rawAudio.SetAllocator(p.allocator)
					rawAudio.Timestamp = time.Duration(frame.Timestamp) * time.Millisecond
					rawAudio.FourCC = audioFourCC
					file.Seek(frame.Offset, io.SeekStart)
					file.Read(rawAudio.NextN(int(frame.Length)))
					err = p.PullJob.Publisher.WriteAudio(&rawAudio)
				case FRAME_TYPE_VIDEO, FRAME_TYPE_VIDEO_KEY_FRAME:
					var rawVideo pkg.H26xFrame
					rawVideo.FourCC = videoFourCC
					rawVideo.SetAllocator(p.allocator)
					rawVideo.Timestamp = time.Duration(frame.Timestamp) * time.Millisecond
					file.Seek(frame.Offset, io.SeekStart)
					file.Read(rawVideo.NextN(int(frame.Length)))
					r := rawVideo.NewReader()
					for {
						nalulen, err := r.ReadBE(4)
						var nalu util.Memory
						if err != nil {
							break
						}
						r.RangeN(int(nalulen), nalu.AppendOne)
						rawVideo.Nalus = append(rawVideo.Nalus, nalu)
					}
					err = p.PullJob.Publisher.WriteVideo(&rawVideo)
				}
				speedControl(frame.Timestamp)
			}
			err = rows.Close()
		} else {
			return fmt.Errorf("db type not found")
		}
	}
	return
}

func (p *Puller) Dispose() {
	if p.ReadCloser != nil {
		p.ReadCloser.Close()
	}
	if p.db != nil {
		p.db.Close()
	}
	p.allocator.Recycle()
}
