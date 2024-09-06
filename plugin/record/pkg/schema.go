package record

import "time"

const (
	FRAME_TYPE_AUDIO = iota + 1
	FRAME_TYPE_VIDEO_KEY_FRAME
	FRAME_TYPE_VIDEO
)

type (
	RecordStream struct {
		ID                       uint `gorm:"primarykey"`
		StartTime, EndTime       time.Time
		FilePath                 string
		AudioCodec, VideoCodec   string
		AudioConfig, VideoConfig []byte
	}
	Sample struct {
		ID        uint `gorm:"primarykey"`
		Type      byte
		Timestamp int64
		CTS       int64
		Offset    int64
		Length    uint
	}
)
