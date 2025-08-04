package plugin_flv

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
	flv "m7s.live/v5/plugin/flv/pkg"
	mp4 "m7s.live/v5/plugin/mp4/pkg"
	rtmp "m7s.live/v5/plugin/rtmp/pkg"
)

// requestParams 包含请求解析后的参数
type requestParams struct {
	streamPath string
	startTime  time.Time
	endTime    time.Time
	timeRange  time.Duration
}

// fileInfo 包含文件信息
type fileInfo struct {
	filePath        string
	startTime       time.Time
	endTime         time.Time
	startOffsetTime time.Duration
	recordType      string // "flv" 或 "mp4"
}

// parseRequestParams 解析请求参数
func (plugin *FLVPlugin) parseRequestParams(r *http.Request) (*requestParams, error) {
	// 从URL路径中提取流路径，去除前缀 "/download/" 和后缀 ".flv"
	streamPath := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/download/"), ".flv")

	// 解析URL查询参数中的时间范围（start和end参数）
	startTime, endTime, err := util.TimeRangeQueryParse(r.URL.Query())
	if err != nil {
		return nil, err
	}

	return &requestParams{
		streamPath: streamPath,
		startTime:  startTime,
		endTime:    endTime,
		timeRange:  endTime.Sub(startTime),
	}, nil
}

// queryRecordStreams 从数据库查询录像记录
func (plugin *FLVPlugin) queryRecordStreams(params *requestParams) ([]m7s.RecordStream, error) {
	// 检查数据库是否可用
	if plugin.DB == nil {
		return nil, fmt.Errorf("database not available")
	}

	var recordStreams []m7s.RecordStream

	// 首先查询FLV记录
	query := plugin.DB.Model(&m7s.RecordStream{}).Where("stream_path = ? AND type = ?", params.streamPath, "flv")

	// 添加时间范围查询条件
	if !params.startTime.IsZero() && !params.endTime.IsZero() {
		query = query.Where("(start_time <= ? AND end_time >= ?) OR (start_time >= ? AND start_time <= ?)",
			params.endTime, params.startTime, params.startTime, params.endTime)
	}

	err := query.Order("start_time ASC").Find(&recordStreams).Error
	if err != nil {
		return nil, err
	}

	// 如果没有找到FLV记录，尝试查询MP4记录
	if len(recordStreams) == 0 {
		query = plugin.DB.Model(&m7s.RecordStream{}).Where("stream_path = ? AND type IN (?)", params.streamPath, []string{"mp4", "fmp4"})

		if !params.startTime.IsZero() && !params.endTime.IsZero() {
			query = query.Where("(start_time <= ? AND end_time >= ?) OR (start_time >= ? AND start_time <= ?)",
				params.endTime, params.startTime, params.startTime, params.endTime)
		}

		err = query.Order("start_time ASC").Find(&recordStreams).Error
		if err != nil {
			return nil, err
		}
	}

	return recordStreams, nil
}

// buildFileInfoList 构建文件信息列表
func (plugin *FLVPlugin) buildFileInfoList(recordStreams []m7s.RecordStream, startTime, endTime time.Time) ([]*fileInfo, bool) {
	var fileInfoList []*fileInfo
	var found bool

	for _, record := range recordStreams {
		// 检查文件是否存在
		if !util.Exist(record.FilePath) {
			plugin.Warn("Record file not found", "filePath", record.FilePath)
			continue
		}

		var startOffsetTime time.Duration
		recordStartTime := record.StartTime
		recordEndTime := record.EndTime

		// 计算文件内的偏移时间
		if startTime.After(recordStartTime) {
			startOffsetTime = startTime.Sub(recordStartTime)
		}

		// 检查是否在时间范围内
		if recordEndTime.Before(startTime) || recordStartTime.After(endTime) {
			continue
		}

		fileInfoList = append(fileInfoList, &fileInfo{
			filePath:        record.FilePath,
			startTime:       recordStartTime,
			endTime:         recordEndTime,
			startOffsetTime: startOffsetTime,
			recordType:      record.Type,
		})

		found = true
	}

	return fileInfoList, found
}

// hasOnlyMp4Records 检查是否只有MP4记录
func (plugin *FLVPlugin) hasOnlyMp4Records(fileInfoList []*fileInfo) bool {
	if len(fileInfoList) == 0 {
		return false
	}

	for _, info := range fileInfoList {
		if info.recordType == "flv" {
			return false
		}
	}
	return true
}

// filterFlvFiles 过滤FLV文件
func (plugin *FLVPlugin) filterFlvFiles(fileInfoList []*fileInfo) []*fileInfo {
	var filteredList []*fileInfo

	for _, info := range fileInfoList {
		if info.recordType == "flv" {
			filteredList = append(filteredList, info)
		}
	}

	plugin.Debug("FLV files filtered", "original", len(fileInfoList), "filtered", len(filteredList))
	return filteredList
}

// filterMp4Files 过滤MP4文件
func (plugin *FLVPlugin) filterMp4Files(fileInfoList []*fileInfo) []*fileInfo {
	var filteredList []*fileInfo

	for _, info := range fileInfoList {
		if info.recordType == "mp4" || info.recordType == "fmp4" {
			filteredList = append(filteredList, info)
		}
	}

	plugin.Debug("MP4 files filtered", "original", len(fileInfoList), "filtered", len(filteredList))
	return filteredList
}

// processMp4ToFlv 将MP4记录转换为FLV输出
func (plugin *FLVPlugin) processMp4ToFlv(w http.ResponseWriter, r *http.Request, fileInfoList []*fileInfo, params *requestParams) {
	plugin.Info("Converting MP4 records to FLV", "count", len(fileInfoList))

	// 设置HTTP响应头
	w.Header().Set("Content-Type", "video/x-flv")
	w.Header().Set("Content-Disposition", "attachment")

	// 创建MP4流列表
	var mp4Streams []m7s.RecordStream
	for _, info := range fileInfoList {
		mp4Streams = append(mp4Streams, m7s.RecordStream{
			FilePath:  info.filePath,
			StartTime: info.startTime,
			EndTime:   info.endTime,
			Type:      info.recordType,
		})
	}

	// 创建DemuxerConverterRange进行MP4解复用和转换
	demuxer := &mp4.DemuxerConverterRange[*rtmp.AudioFrame, *rtmp.VideoFrame]{
		DemuxerRange: mp4.DemuxerRange{
			StartTime: params.startTime,
			EndTime:   params.endTime,
			Streams:   mp4Streams,
			Logger:    plugin.Logger.With("demuxer", "mp4_flv"),
		},
	}

	// 创建FLV编码器状态
	flvWriter := flv.NewFlvWriter(w)
	ts := int64(0)       // 初始化时间戳
	tsOffset := int64(0) // 偏移时间戳
	demuxer.OnCodec = func(a, v codec.ICodecCtx) {
		flvWriter.WriteHeader(a != nil, v != nil)
	}
	demuxer.OnAudio = func(audio *rtmp.AudioFrame) error {
		// 计算调整后的时间戳
		ts = int64(audio.Timestamp) + tsOffset
		timestamp := uint32(ts)
		// 写入音频数据帧
		return flvWriter.WriteTag(flv.FLV_TAG_TYPE_AUDIO, timestamp, uint32(audio.Size), audio.Buffers...)
	}
	demuxer.OnVideo = func(frame *rtmp.VideoFrame) error {
		// 计算调整后的时间戳
		ts = int64(frame.Timestamp) + tsOffset
		timestamp := uint32(ts)
		// 写入视频数据帧
		return flvWriter.WriteTag(flv.FLV_TAG_TYPE_VIDEO, timestamp, uint32(frame.Size), frame.Buffers...)
	}
	// 执行解复用和转换
	err := demuxer.Demux(r.Context())
	if err != nil {
		plugin.Error("MP4 to FLV conversion failed", "err", err)
		return
	}

	plugin.Info("MP4 to FLV conversion completed")
}

// processFlvFiles 处理原生FLV文件
func (plugin *FLVPlugin) processFlvFiles(w http.ResponseWriter, r *http.Request, fileInfoList []*fileInfo, params *requestParams) {
	plugin.Info("Processing FLV files", "count", len(fileInfoList))

	// 设置HTTP响应头
	w.Header().Set("Content-Type", "video/x-flv")
	w.Header().Set("Content-Disposition", "attachment")

	var writer io.Writer = w
	flvHead := make([]byte, 9+4)
	tagHead := make(util.Buffer, 11)
	var contentLength uint64
	var startOffsetTime time.Duration

	// 计算第一个文件的偏移时间
	if len(fileInfoList) > 0 {
		startOffsetTime = fileInfoList[0].startOffsetTime
	}

	var amf rtmp.AMF
	var metaData rtmp.EcmaArray
	initMetaData := func(reader io.Reader, dataLen uint32) {
		data := make([]byte, dataLen+4)
		_, err := io.ReadFull(reader, data)
		if err != nil {
			return
		}
		amf = rtmp.AMF(data[1+2+len("onMetaData") : len(data)-4])
		var obj any
		obj, err = amf.Unmarshal()
		if err == nil {
			metaData = obj.(rtmp.EcmaArray)
		}
	}

	var filepositions []uint64
	var times []float64

	// 两次遍历：第一次计算大小，第二次写入数据
	for pass := 0; pass < 2; pass++ {
		offsetTime := startOffsetTime
		var offsetTimestamp, lastTimestamp uint32
		var init, seqAudioWritten, seqVideoWritten bool

		if pass == 1 {
			// 第二次遍历时，准备写入
			metaData["keyframes"] = map[string]any{
				"filepositions": filepositions,
				"times":         times,
			}
			amf.Marshals("onMetaData", metaData)
			offsetDelta := amf.GetBuffer().Len() + 15
			offset := offsetDelta + len(flvHead)
			contentLength += uint64(offset)
			metaData["duration"] = params.timeRange.Seconds()
			metaData["filesize"] = contentLength
			for i := range filepositions {
				filepositions[i] += uint64(offset)
			}
			metaData["keyframes"] = map[string]any{
				"filepositions": filepositions,
				"times":         times,
			}
			amf.GetBuffer().Reset()
			amf.Marshals("onMetaData", metaData)
			plugin.Info("start download", "metaData", metaData)
			w.Header().Set("Content-Length", strconv.FormatInt(int64(contentLength), 10))
			w.WriteHeader(http.StatusOK)
		}

		if offsetTime == 0 {
			init = true
		} else {
			offsetTimestamp = -uint32(offsetTime.Milliseconds())
		}

		for i, info := range fileInfoList {
			if r.Context().Err() != nil {
				return
			}

			plugin.Debug("Processing file", "path", info.filePath)
			file, err := os.Open(info.filePath)
			if err != nil {
				plugin.Error("Failed to open file", "path", info.filePath, "err", err)
				if pass == 1 {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
				return
			}

			reader := bufio.NewReader(file)

			if i == 0 {
				_, err = io.ReadFull(reader, flvHead)
				if err != nil {
					file.Close()
					if pass == 1 {
						http.Error(w, err.Error(), http.StatusInternalServerError)
					}
					return
				}
				if pass == 1 {
					// 第一次写入头
					_, err = writer.Write(flvHead)
					if err != nil {
						file.Close()
						return
					}
					tagHead[0] = flv.FLV_TAG_TYPE_SCRIPT
					l := amf.GetBuffer().Len()
					tagHead[1] = byte(l >> 16)
					tagHead[2] = byte(l >> 8)
					tagHead[3] = byte(l)
					flv.PutFlvTimestamp(tagHead, 0)
					writer.Write(tagHead)
					writer.Write([]byte(amf))
					l += 11
					binary.BigEndian.PutUint32(tagHead[:4], uint32(l))
					writer.Write(tagHead[:4])
				}
			} else {
				// 后面的头跳过
				_, err = reader.Discard(13)
				if err != nil {
					file.Close()
					continue
				}
				if !init {
					offsetTime = 0
					offsetTimestamp = 0
				}
			}

			// 处理FLV标签
			for err == nil {
				_, err = io.ReadFull(reader, tagHead)
				if err != nil {
					break
				}
				tmp := tagHead
				t := tmp.ReadByte()
				dataLen := tmp.ReadUint24()
				lastTimestamp = tmp.ReadUint24() | uint32(tmp.ReadByte())<<24

				if init {
					if t == flv.FLV_TAG_TYPE_SCRIPT {
						if pass == 0 {
							initMetaData(reader, dataLen)
						} else {
							_, err = reader.Discard(int(dataLen) + 4)
						}
					} else {
						lastTimestamp += offsetTimestamp
						if lastTimestamp >= uint32(params.timeRange.Milliseconds()) {
							break
						}
						if pass == 0 {
							data := make([]byte, dataLen+4)
							_, err = io.ReadFull(reader, data)
							if err == nil {
								frameType := (data[0] >> 4) & 0b0111
								idr := frameType == 1 || frameType == 4
								if idr {
									filepositions = append(filepositions, contentLength)
									times = append(times, float64(lastTimestamp)/1000)
								}
								contentLength += uint64(11 + dataLen + 4)
							}
						} else {
							flv.PutFlvTimestamp(tagHead, lastTimestamp)
							_, err = writer.Write(tagHead)
							if err == nil {
								_, err = io.CopyN(writer, reader, int64(dataLen+4))
							}
						}
					}
					continue
				}

				switch t {
				case flv.FLV_TAG_TYPE_SCRIPT:
					if pass == 0 {
						initMetaData(reader, dataLen)
					} else {
						_, err = reader.Discard(int(dataLen) + 4)
					}
				case flv.FLV_TAG_TYPE_AUDIO:
					if !seqAudioWritten {
						if pass == 0 {
							contentLength += uint64(11 + dataLen + 4)
							_, err = reader.Discard(int(dataLen) + 4)
						} else {
							flv.PutFlvTimestamp(tagHead, 0)
							_, err = writer.Write(tagHead)
							if err == nil {
								_, err = io.CopyN(writer, reader, int64(dataLen+4))
							}
						}
						seqAudioWritten = true
					} else {
						_, err = reader.Discard(int(dataLen) + 4)
					}
				case flv.FLV_TAG_TYPE_VIDEO:
					if !seqVideoWritten {
						if pass == 0 {
							contentLength += uint64(11 + dataLen + 4)
							_, err = reader.Discard(int(dataLen) + 4)
						} else {
							flv.PutFlvTimestamp(tagHead, 0)
							_, err = writer.Write(tagHead)
							if err == nil {
								_, err = io.CopyN(writer, reader, int64(dataLen+4))
							}
						}
						seqVideoWritten = true
					} else {
						if lastTimestamp >= uint32(offsetTime.Milliseconds()) {
							data := make([]byte, dataLen+4)
							_, err = io.ReadFull(reader, data)
							if err == nil {
								frameType := (data[0] >> 4) & 0b0111
								idr := frameType == 1 || frameType == 4
								if idr {
									init = true
									plugin.Debug("init", "lastTimestamp", lastTimestamp)
									if pass == 0 {
										filepositions = append(filepositions, contentLength)
										times = append(times, float64(lastTimestamp)/1000)
										contentLength += uint64(11 + dataLen + 4)
									} else {
										flv.PutFlvTimestamp(tagHead, 0)
										_, err = writer.Write(tagHead)
										if err == nil {
											_, err = writer.Write(data)
										}
									}
								}
							}
						} else {
							_, err = reader.Discard(int(dataLen) + 4)
						}
					}
				}
			}
			offsetTimestamp = lastTimestamp
			file.Close()
		}
	}
	plugin.Info("FLV download completed")
}
