package plugin_hls

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/format"
	mpegts "m7s.live/v5/pkg/format/ts"
	"m7s.live/v5/pkg/util"
	hls "m7s.live/v5/plugin/hls/pkg"
	mp4 "m7s.live/v5/plugin/mp4/pkg"
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
	recordType      string // "ts", "mp4", "fmp4"
}

// parseRequestParams 解析请求参数
func (plugin *HLSPlugin) parseRequestParams(r *http.Request) (*requestParams, error) {
	// 从URL路径中提取流路径，去除前缀 "/download/" 和后缀 ".ts"
	streamPath := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/download/"), ".ts")

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
func (plugin *HLSPlugin) queryRecordStreams(params *requestParams) ([]m7s.RecordStream, error) {
	// 检查数据库是否可用
	if plugin.DB == nil {
		return nil, fmt.Errorf("database not available")
	}

	var recordStreams []m7s.RecordStream

	// 首先查询HLS记录 (ts)
	query := plugin.DB.Model(&m7s.RecordStream{}).Where("stream_path = ? AND type = ?", params.streamPath, "hls")

	// 添加时间范围查询条件
	if !params.startTime.IsZero() && !params.endTime.IsZero() {
		query = query.Where("(start_time <= ? AND end_time >= ?) OR (start_time >= ? AND start_time <= ?)",
			params.endTime, params.startTime, params.startTime, params.endTime)
	}

	err := query.Order("start_time ASC").Find(&recordStreams).Error
	if err != nil {
		return nil, err
	}

	// 如果没有找到HLS记录，尝试查询MP4记录
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
func (plugin *HLSPlugin) buildFileInfoList(recordStreams []m7s.RecordStream, startTime, endTime time.Time) ([]*fileInfo, bool) {
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
func (plugin *HLSPlugin) hasOnlyMp4Records(fileInfoList []*fileInfo) bool {
	if len(fileInfoList) == 0 {
		return false
	}

	for _, info := range fileInfoList {
		if info.recordType == "hls" {
			return false
		}
	}
	return true
}

// filterTsFiles 过滤HLS TS文件
func (plugin *HLSPlugin) filterTsFiles(fileInfoList []*fileInfo) []*fileInfo {
	var filteredList []*fileInfo

	for _, info := range fileInfoList {
		if info.recordType == "hls" {
			filteredList = append(filteredList, info)
		}
	}

	plugin.Debug("TS files filtered", "original", len(fileInfoList), "filtered", len(filteredList))
	return filteredList
}

// filterMp4Files 过滤MP4文件
func (plugin *HLSPlugin) filterMp4Files(fileInfoList []*fileInfo) []*fileInfo {
	var filteredList []*fileInfo

	for _, info := range fileInfoList {
		if info.recordType == "mp4" || info.recordType == "fmp4" {
			filteredList = append(filteredList, info)
		}
	}

	plugin.Debug("MP4 files filtered", "original", len(fileInfoList), "filtered", len(filteredList))
	return filteredList
}

// processMp4ToTs 将MP4记录转换为TS输出
func (plugin *HLSPlugin) processMp4ToTs(w http.ResponseWriter, r *http.Request, fileInfoList []*fileInfo, params *requestParams) {
	plugin.Info("Converting MP4 records to TS", "count", len(fileInfoList))

	// 设置HTTP响应头
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Content-Disposition", "attachment")

	// 创建MP4流列表
	var mp4Streams []m7s.RecordStream
	for _, info := range fileInfoList {
		plugin.Debug("Processing MP4 file", "path", info.filePath, "startTime", info.startTime, "endTime", info.endTime)
		mp4Streams = append(mp4Streams, m7s.RecordStream{
			FilePath:  info.filePath,
			StartTime: info.startTime,
			EndTime:   info.endTime,
			Type:      info.recordType,
		})
	}

	// 创建DemuxerConverterRange进行MP4解复用和转换
	demuxer := &mp4.DemuxerConverterRange[*format.Mpeg2Audio, *format.AnnexB]{
		DemuxerRange: mp4.DemuxerRange{
			StartTime: params.startTime,
			EndTime:   params.endTime,
			Streams:   mp4Streams,
			Logger:    plugin.Logger.With("demuxer", "mp4_Ts"),
		},
	}
	// 创建TS编码器状态
	tsWriter := &hls.TsInMemory{}

	pesAudio, pesVideo := mpegts.CreatePESWriters()
	demuxer.OnCodec = func(a, v codec.ICodecCtx) {
		var audio, video codec.FourCC
		if a != nil {
			audio = a.FourCC()
		}
		if v != nil {
			video = v.FourCC()
		}
		tsWriter.WritePMTPacket(audio, video)
	}
	demuxer.OnAudio = func(audio *format.Mpeg2Audio) error {
		pesAudio.Pts = uint64(audio.GetPTS())
		return pesAudio.WritePESPacket(audio.Memory, &tsWriter.RecyclableMemory)
	}
	demuxer.OnVideo = func(video *format.AnnexB) error {
		pesVideo.IsKeyFrame = video.IDR
		pesVideo.Pts = uint64(video.GetPTS())
		pesVideo.Dts = uint64(video.GetDTS())
		return pesVideo.WritePESPacket(video.Memory, &tsWriter.RecyclableMemory)
	}
	// 执行解复用和转换
	err := demuxer.Demux(r.Context())

	if err != nil {
		plugin.Error("MP4 to TS conversion failed", "err", err)
		return
	}

	// 将所有累积的 TsInMemory 内容写入到响应
	w.WriteHeader(http.StatusOK)
	_, err = tsWriter.WriteTo(w)
	if err != nil {
		plugin.Error("Failed to write TS data to response", "error", err)
		return
	}

	plugin.Info("MP4 to TS conversion completed")
}

// processTsFiles 处理原生TS文件拼接
func (plugin *HLSPlugin) processTsFiles(w http.ResponseWriter, r *http.Request, fileInfoList []*fileInfo, params *requestParams) {
	plugin.Info("Processing TS files", "count", len(fileInfoList))

	// 设置HTTP响应头
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Content-Disposition", "attachment")

	var writer io.Writer = w
	var totalSize uint64

	// 第一次遍历：计算总大小
	for _, info := range fileInfoList {
		if r.Context().Err() != nil {
			return
		}

		fileInfo, err := os.Stat(info.filePath)
		if err != nil {
			plugin.Error("Failed to stat file", "path", info.filePath, "err", err)
			continue
		}
		totalSize += uint64(fileInfo.Size())
	}

	// 设置内容长度
	w.Header().Set("Content-Length", strconv.FormatUint(totalSize, 10))
	w.WriteHeader(http.StatusOK)

	// 第二次遍历：写入数据
	for i, info := range fileInfoList {
		if r.Context().Err() != nil {
			return
		}

		plugin.Debug("Processing TS file", "path", info.filePath)
		file, err := os.Open(info.filePath)
		if err != nil {
			plugin.Error("Failed to open file", "path", info.filePath, "err", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		reader := bufio.NewReader(file)

		if i == 0 {
			// 第一个文件，直接拷贝
			_, err = io.Copy(writer, reader)
		} else {
			// 后续文件，跳过PAT/PMT包，只拷贝媒体数据
			err = plugin.copyTsFileSkipHeaders(writer, reader)
		}

		file.Close()

		if err != nil {
			plugin.Error("Failed to copy file", "path", info.filePath, "err", err)
			return
		}
	}

	plugin.Info("TS download completed")
}

// copyTsFileSkipHeaders 拷贝TS文件，跳过PAT/PMT包
func (plugin *HLSPlugin) copyTsFileSkipHeaders(writer io.Writer, reader *bufio.Reader) error {
	buffer := make([]byte, mpegts.TS_PACKET_SIZE)

	for {
		n, err := io.ReadFull(reader, buffer)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return err
		}

		if n != mpegts.TS_PACKET_SIZE {
			continue
		}

		// 检查同步字节
		if buffer[0] != 0x47 {
			continue
		}

		// 提取PID
		pid := uint16(buffer[1]&0x1f)<<8 | uint16(buffer[2])

		// 跳过PAT(PID=0)和PMT(PID=256)包
		if pid == mpegts.PID_PAT || pid == mpegts.PID_PMT {
			continue
		}

		// 写入媒体数据包
		_, err = writer.Write(buffer)
		if err != nil {
			return err
		}
	}

	return nil
}

// download 下载处理函数
func (plugin *HLSPlugin) download(w http.ResponseWriter, r *http.Request) {
	// 解析请求参数
	params, err := plugin.parseRequestParams(r)
	if err != nil {
		plugin.Error("Failed to parse request params", "err", err)
		http.Error(w, "Invalid parameters", http.StatusBadRequest)
		return
	}

	plugin.Info("TS download request", "streamPath", params.streamPath, "timeRange", params.timeRange)

	// 查询录像记录
	recordStreams, err := plugin.queryRecordStreams(params)
	if err != nil {
		plugin.Error("Failed to query record streams", "err", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if len(recordStreams) == 0 {
		plugin.Warn("No records found", "streamPath", params.streamPath)
		http.Error(w, "No records found", http.StatusNotFound)
		return
	}

	// 构建文件信息列表
	fileInfoList, found := plugin.buildFileInfoList(recordStreams, params.startTime, params.endTime)
	if !found {
		plugin.Warn("No valid files found", "streamPath", params.streamPath)
		http.Error(w, "No valid files found", http.StatusNotFound)
		return
	}

	// 检查文件类型并处理
	if plugin.hasOnlyMp4Records(fileInfoList) {
		// 只有MP4记录，转换为TS
		mp4Files := plugin.filterMp4Files(fileInfoList)
		plugin.processMp4ToTs(w, r, mp4Files, params)
	} else {
		// 有TS记录，优先使用TS文件
		tsFiles := plugin.filterTsFiles(fileInfoList)
		if len(tsFiles) > 0 {
			plugin.processTsFiles(w, r, tsFiles, params)
		} else {
			// 没有TS文件，使用MP4转换
			mp4Files := plugin.filterMp4Files(fileInfoList)
			plugin.processMp4ToTs(w, r, mp4Files, params)
		}
	}
}
