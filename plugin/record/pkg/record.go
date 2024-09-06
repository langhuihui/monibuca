package record

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"time"

	"gorm.io/gorm"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg"
	"m7s.live/m7s/v5/pkg/db"
)

func NewRecorder() m7s.IRecorder {
	return &Recorder{}
}

type Recorder struct {
	m7s.DefaultRecorder
	DB     *gorm.DB
	file   *os.File
	stream *RecordStream
}

func (r *Recorder) Start() (err error) {
	recordJob := &r.RecordJob
	plugin := recordJob.Plugin
	if plugin.DB == nil {
		return fmt.Errorf("db not found")
	}
	err = recordJob.Subscribe()
	if err != nil {
		return
	}
	sub := recordJob.Subscriber
	var newStream RecordStream
	r.stream = &newStream
	newStream.FilePath = recordJob.FilePath
	newStream.StartTime = sub.StartTime
	if sub.Publisher.HasAudioTrack() {
		newStream.AudioCodec = sub.Publisher.AudioTrack.ICodecCtx.FourCC().String()
		newStream.AudioConfig = sub.Publisher.AudioTrack.ICodecCtx.GetRecord()
	}
	if sub.Publisher.HasVideoTrack() {
		newStream.VideoCodec = sub.Publisher.VideoTrack.ICodecCtx.FourCC().String()
		newStream.VideoConfig = sub.Publisher.VideoTrack.ICodecCtx.GetRecord()
	}
	err = plugin.DB.AutoMigrate(r.stream)
	if err != nil {
		return
	}
	plugin.DB.Save(r.stream)
	dbType := plugin.GetCommonConf().DBType
	if factory, ok := db.Factory[dbType]; ok {
		_ = os.MkdirAll(recordJob.FilePath, 0755)
		r.file, err = os.Create(filepath.Join(recordJob.FilePath, fmt.Sprintf("%d.rec", newStream.ID)))
		if err != nil {
			return
		}
		r.DB, err = gorm.Open(factory(filepath.Join(recordJob.FilePath, fmt.Sprintf("%d.db", newStream.ID))), &gorm.Config{})
		if err != nil {
			r.Error("failed to connect database", "error", err, "dsn", recordJob.FilePath, "type", dbType)
			return
		}
		if err = r.DB.AutoMigrate(&Sample{}); err != nil {
			return fmt.Errorf("failed to migrate Frame: %w", err)
		}
	} else {
		return fmt.Errorf("db type not found %s", dbType)
	}
	return
}

func (r *Recorder) Run() (err error) {
	recordJob := &r.RecordJob
	sub := recordJob.Subscriber
	return m7s.PlayBlock(sub, func(audio *pkg.RawAudio) (err error) {
		var sample Sample
		sample.Type = FRAME_TYPE_AUDIO
		sample.Timestamp = audio.Timestamp.Milliseconds()
		sample.Length = uint(audio.Size)
		data := slices.Clone(audio.Buffers)
		sample.Offset, err = r.file.Seek(0, io.SeekCurrent)
		_, err = data.WriteTo(r.file)
		r.DB.Save(&sample)
		return
	}, func(video *pkg.H26xFrame) (err error) {
		var sample Sample
		sample.Type = FRAME_TYPE_VIDEO
		if sub.VideoReader.Value.IDR {
			sample.Type = FRAME_TYPE_VIDEO_KEY_FRAME
		}
		sample.Timestamp = video.Timestamp.Milliseconds()
		sample.CTS = video.CTS.Milliseconds()
		sample.Offset, err = r.file.Seek(0, io.SeekCurrent)
		for _, nalu := range video.Nalus {
			sample.Length += uint(nalu.Size) + 4
			avcc := append(net.Buffers{[]byte{byte(nalu.Size >> 24), byte(nalu.Size >> 16), byte(nalu.Size >> 8), byte(nalu.Size)}}, nalu.Buffers...)
			_, err = avcc.WriteTo(r.file)
		}
		r.DB.Save(&sample)
		return
	})
}

func (r *Recorder) Dispose() {
	r.stream.EndTime = time.Now()
	r.RecordJob.Plugin.DB.Save(r.stream)
	if db, err := r.DB.DB(); err == nil {
		db.Close()
	}
}
