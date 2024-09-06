package plugin_record

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"gorm.io/gorm"
	"m7s.live/m7s/v5/pkg/codec"
	"m7s.live/m7s/v5/pkg/db"
	"m7s.live/m7s/v5/pkg/util"
	flv "m7s.live/m7s/v5/plugin/flv/pkg"
	record "m7s.live/m7s/v5/plugin/record/pkg"
)

func (plugin *RecordPlugin) vodFLV(w http.ResponseWriter, r *http.Request) {
	streamPath := r.PathValue("streamPath")
	query := r.URL.Query()
	speedStr := query.Get("speed")
	speed, err := strconv.ParseFloat(speedStr, 64)
	if err != nil {
		speed = 1
	}
	beginTime := time.Now()
	var startTimestamp int64
	speedControl := func(ts int64) {
		targetTime := time.Duration(float64(time.Since(beginTime)) * speed)
		sleepTime := time.Duration(ts-startTimestamp)*time.Millisecond - targetTime
		fmt.Println("sleepTime", sleepTime)
		// if sleepTime > 0 {
		// 	time.Sleep(sleepTime)
		// }
	}
	if startTime, err := util.TimeQueryParse(query.Get("start")); err == nil {
		var streams []*record.RecordStream
		tx := plugin.DB.Find(&streams, "end_time>? AND stream_path=?", startTime, streamPath)
		if tx.Error != nil {
			http.Error(w, tx.Error.Error(), http.StatusInternalServerError)
			return
		}
		if tx.RowsAffected <= 0 {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "video/x-flv")
		w.Header().Set("Transfer-Encoding", "identity")
		w.WriteHeader(http.StatusOK)
		var flvWriter = flv.NewFlvWriter(w)
		flvWriter.WriteHeader(true, true)
		for _, stream := range streams {
			dbType := plugin.GetCommonConf().DBType
			if factory, ok := db.Factory[dbType]; ok {
				var streamDB *gorm.DB
				streamDB, err = gorm.Open(factory(filepath.Join(stream.FilePath, fmt.Sprintf("%d.db", stream.ID))), &gorm.Config{})
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				startTimestamp = startTime.Sub(stream.StartTime).Milliseconds()
				hasAudio, hasVideo := stream.AudioCodec != "", stream.VideoCodec != ""
				var startId uint
				if hasAudio && stream.AudioCodec == codec.FourCC_MP4A.String() {
					flvWriter.WriteTag(flv.FLV_TAG_TYPE_AUDIO, 0, uint32(len(stream.AudioConfig))+2, append([]byte{0xaf, 0x00}, stream.AudioConfig...))
				}
				if hasVideo {
					var avccConfig []byte
					if stream.VideoCodec == codec.FourCC_H264.String() {
						avccConfig = append([]byte{0x17, 0x00, 0x00, 0x00, 0x00}, stream.VideoConfig...)
					} else if stream.VideoCodec == codec.FourCC_H265.String() {
						// TODO: HEVC
						//avccConfig = append([]byte{0x40, 0x01, 0x00, 0x00, 0x00}, stream.VideoConfig...)
					}
					flvWriter.WriteTag(flv.FLV_TAG_TYPE_VIDEO, 0, uint32(len(avccConfig)), avccConfig)
					streamDB.Last(&record.Sample{}, "type=? AND timestamp<=?", record.FRAME_TYPE_VIDEO_KEY_FRAME, startTimestamp).Scan(&startId)
				} else {
					// TODO
				}
				rows, err := streamDB.Model(&record.Sample{}).Where("id>=? ", startId).Rows()
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				for rows.Next() {
					var frame record.Sample
					streamDB.ScanRows(rows, &frame)
					switch frame.Type {
					case record.FRAME_TYPE_AUDIO:
						//flvWriter.WriteTag(flv.FLV_TAG_TYPE_AUDIO, uint32(frame.Timestamp), uint32(len(frame.Data)), frame.Data)
					case record.FRAME_TYPE_VIDEO, record.FRAME_TYPE_VIDEO_KEY_FRAME:
						//flvWriter.WriteTag(flv.FLV_TAG_TYPE_VIDEO, uint32(frame.Timestamp), uint32(len(frame.Data)), frame.Data)
					}
					speedControl(frame.Timestamp)
				}
				rows.Close()
			} else {
				http.Error(w, fmt.Sprintf("db type not found %s", dbType), http.StatusInternalServerError)
				return
			}
		}
	}
}
