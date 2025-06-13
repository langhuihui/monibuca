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
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/util"
	hls "m7s.live/v5/plugin/hls/pkg"
	mpegts "m7s.live/v5/plugin/hls/pkg/ts"
	mp4 "m7s.live/v5/plugin/mp4/pkg"
	"m7s.live/v5/plugin/mp4/pkg/box"
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

	// 创建一个TS写入器，在循环外面，所有MP4文件共享同一个TsInMemory
	tsWriter := &simpleTsWriter{
		TsInMemory: &hls.TsInMemory{},
		plugin:     plugin,
	}

	// 对于MP4到TS的转换，我们采用简化的方法
	// 直接将每个MP4文件转换输出
	for _, info := range fileInfoList {
		if r.Context().Err() != nil {
			return
		}

		plugin.Debug("Converting MP4 file to TS", "path", info.filePath)

		// 创建MP4解复用器
		demuxer := &mp4.DemuxerRange{
			StartTime: params.startTime,
			EndTime:   params.endTime,
			Streams: []m7s.RecordStream{{
				FilePath:  info.filePath,
				StartTime: info.startTime,
				EndTime:   info.endTime,
				Type:      info.recordType,
			}},
		}

		// 设置回调函数
		demuxer.OnVideoExtraData = tsWriter.onVideoExtraData
		demuxer.OnAudioExtraData = tsWriter.onAudioExtraData
		demuxer.OnVideoSample = tsWriter.onVideoSample
		demuxer.OnAudioSample = tsWriter.onAudioSample

		// 执行解复用和转换
		err := demuxer.Demux(r.Context())
		if err != nil {
			plugin.Error("MP4 to TS conversion failed", "err", err, "file", info.filePath)
			if !tsWriter.hasWritten {
				http.Error(w, "Conversion failed", http.StatusInternalServerError)
			}
			return
		}
	}

	// 将所有累积的 TsInMemory 内容写入到响应
	_, err := tsWriter.WriteTo(w)
	if err != nil {
		plugin.Error("Failed to write TS data to response", "error", err)
		return
	}

	plugin.Info("MP4 to TS conversion completed")
}

// simpleTsWriter 简化的TS写入器
type simpleTsWriter struct {
	*hls.TsInMemory
	plugin     *HLSPlugin
	hasWritten bool
	spsData    []byte
	ppsData    []byte
	videoCodec box.MP4_CODEC_TYPE
	audioCodec box.MP4_CODEC_TYPE
}

func (w *simpleTsWriter) WritePMT() {
	// 初始化 TsInMemory 的 PMT
	var videoCodec, audioCodec [4]byte
	switch w.videoCodec {
	case box.MP4_CODEC_H264:
		copy(videoCodec[:], []byte("H264"))
	case box.MP4_CODEC_H265:
		copy(videoCodec[:], []byte("H265"))
	}
	switch w.audioCodec {
	case box.MP4_CODEC_AAC:
		copy(audioCodec[:], []byte("MP4A"))

	}
	w.WritePMTPacket(audioCodec, videoCodec)
	w.hasWritten = true
}

// onVideoExtraData 处理视频序列头
func (w *simpleTsWriter) onVideoExtraData(codecType box.MP4_CODEC_TYPE, data []byte) error {
	w.videoCodec = codecType
	// 解析并存储SPS/PPS数据
	if codecType == box.MP4_CODEC_H264 && len(data) > 0 {
		if w.plugin != nil {
			w.plugin.Debug("Processing H264 extra data", "size", len(data))
		}

		// 解析AVCC格式的extra data
		if len(data) >= 8 {
			// AVCC格式: configurationVersion(1) + AVCProfileIndication(1) + profile_compatibility(1) + AVCLevelIndication(1) +
			//          lengthSizeMinusOne(1) + numOfSequenceParameterSets(1) + ...

			offset := 5 // 跳过前5个字节
			if offset < len(data) {
				// 读取SPS数量
				numSPS := data[offset] & 0x1f
				offset++

				// 解析SPS
				for i := 0; i < int(numSPS) && offset < len(data)-1; i++ {
					if offset+1 >= len(data) {
						break
					}
					spsLength := int(data[offset])<<8 | int(data[offset+1])
					offset += 2

					if offset+spsLength <= len(data) {
						// 添加起始码并存储SPS
						w.spsData = make([]byte, 4+spsLength)
						copy(w.spsData[0:4], []byte{0x00, 0x00, 0x00, 0x01})
						copy(w.spsData[4:], data[offset:offset+spsLength])
						offset += spsLength

						if w.plugin != nil {
							w.plugin.Debug("Extracted SPS", "length", spsLength)
						}
						break // 只取第一个SPS
					}
				}

				// 读取PPS数量
				if offset < len(data) {
					numPPS := data[offset]
					offset++

					// 解析PPS
					for i := 0; i < int(numPPS) && offset < len(data)-1; i++ {
						if offset+1 >= len(data) {
							break
						}
						ppsLength := int(data[offset])<<8 | int(data[offset+1])
						offset += 2

						if offset+ppsLength <= len(data) {
							// 添加起始码并存储PPS
							w.ppsData = make([]byte, 4+ppsLength)
							copy(w.ppsData[0:4], []byte{0x00, 0x00, 0x00, 0x01})
							copy(w.ppsData[4:], data[offset:offset+ppsLength])

							if w.plugin != nil {
								w.plugin.Debug("Extracted PPS", "length", ppsLength)
							}
							break // 只取第一个PPS
						}
					}
				}
			}
		}
	}

	return nil
}

// onAudioExtraData 处理音频序列头
func (w *simpleTsWriter) onAudioExtraData(codecType box.MP4_CODEC_TYPE, data []byte) error {
	w.audioCodec = codecType
	w.plugin.Debug("Processing audio extra data", "codec", codecType, "size", len(data))
	return nil
}

// onVideoSample 处理视频样本
func (w *simpleTsWriter) onVideoSample(codecType box.MP4_CODEC_TYPE, sample box.Sample) error {
	if !w.hasWritten {
		w.WritePMT()
	}

	w.plugin.Debug("Processing video sample", "size", len(sample.Data), "keyFrame", sample.KeyFrame, "timestamp", sample.Timestamp)

	// 转换AVCC格式到Annex-B格式
	annexBData, err := w.convertAVCCToAnnexB(sample.Data, sample.KeyFrame)
	if err != nil {
		w.plugin.Error("Failed to convert AVCC to Annex-B", "error", err)
		return err
	}

	if len(annexBData) == 0 {
		w.plugin.Warn("Empty Annex-B data after conversion")
		return nil
	}

	// 创建视频帧结构
	videoFrame := mpegts.MpegtsPESFrame{
		Pid:        mpegts.PID_VIDEO,
		IsKeyFrame: sample.KeyFrame,
	}

	// 创建 AnnexB 帧
	annexBFrame := &pkg.AnnexB{
		PTS: (time.Duration(sample.Timestamp) + time.Duration(sample.CTS)) * 90,
		DTS: time.Duration(sample.Timestamp) * 90, // 对于MP4转换，假设PTS=DTS
	}

	// 根据编解码器类型设置 Hevc 标志
	if codecType == box.MP4_CODEC_H265 {
		annexBFrame.Hevc = true
	}

	annexBFrame.AppendOne(annexBData)

	// 使用 WriteVideoFrame 写入TS包
	err = w.WriteVideoFrame(annexBFrame, &videoFrame)
	if err != nil {
		w.plugin.Error("Failed to write video frame", "error", err)
		return err
	}

	return nil
}

// convertAVCCToAnnexB 将AVCC格式转换为Annex-B格式
func (w *simpleTsWriter) convertAVCCToAnnexB(avccData []byte, isKeyFrame bool) ([]byte, error) {
	if len(avccData) == 0 {
		return nil, fmt.Errorf("empty AVCC data")
	}

	var annexBBuffer []byte

	// 如果是关键帧，先添加SPS和PPS
	if isKeyFrame {
		if len(w.spsData) > 0 {
			annexBBuffer = append(annexBBuffer, w.spsData...)
			w.plugin.Debug("Added SPS to key frame", "spsSize", len(w.spsData))
		}
		if len(w.ppsData) > 0 {
			annexBBuffer = append(annexBBuffer, w.ppsData...)
			w.plugin.Debug("Added PPS to key frame", "ppsSize", len(w.ppsData))
		}
	}

	// 解析AVCC格式的NAL单元
	offset := 0
	nalCount := 0

	for offset < len(avccData) {
		// AVCC格式：4字节长度 + NAL数据
		if offset+4 > len(avccData) {
			break
		}

		// 读取NAL单元长度（大端序）
		nalLength := int(avccData[offset])<<24 |
			int(avccData[offset+1])<<16 |
			int(avccData[offset+2])<<8 |
			int(avccData[offset+3])
		offset += 4

		if nalLength <= 0 || offset+nalLength > len(avccData) {
			w.plugin.Warn("Invalid NAL length", "length", nalLength, "remaining", len(avccData)-offset)
			break
		}

		nalData := avccData[offset : offset+nalLength]
		offset += nalLength
		nalCount++

		if len(nalData) > 0 {
			nalType := nalData[0] & 0x1f
			w.plugin.Debug("Converting NAL unit", "type", nalType, "length", nalLength)

			// 添加起始码前缀
			annexBBuffer = append(annexBBuffer, []byte{0x00, 0x00, 0x00, 0x01}...)
			annexBBuffer = append(annexBBuffer, nalData...)
		}
	}

	if nalCount == 0 {
		return nil, fmt.Errorf("no NAL units found in AVCC data")
	}

	w.plugin.Debug("AVCC to Annex-B conversion completed",
		"inputSize", len(avccData),
		"outputSize", len(annexBBuffer),
		"nalUnits", nalCount)

	return annexBBuffer, nil
}

// onAudioSample 处理音频样本
func (w *simpleTsWriter) onAudioSample(codecType box.MP4_CODEC_TYPE, sample box.Sample) error {
	if !w.hasWritten {
		w.WritePMT()
	}

	w.plugin.Debug("Processing audio sample", "codec", codecType, "size", len(sample.Data), "timestamp", sample.Timestamp)

	// 创建音频帧结构
	audioFrame := mpegts.MpegtsPESFrame{
		Pid: mpegts.PID_AUDIO,
	}

	// 根据编解码器类型处理音频数据
	switch codecType {
	case box.MP4_CODEC_AAC: // AAC
		// 创建 ADTS 帧
		adtsFrame := &pkg.ADTS{
			DTS: time.Duration(sample.Timestamp) * 90,
		}

		// 将音频数据添加到帧中
		copy(adtsFrame.NextN(len(sample.Data)), sample.Data)

		// 使用 WriteAudioFrame 写入TS包
		err := w.WriteAudioFrame(adtsFrame, &audioFrame)
		if err != nil {
			w.plugin.Error("Failed to write audio frame", "error", err)
			return err
		}
	default:
		// 对于非AAC音频，暂时使用原来的PES包方式
		pesPacket := mpegts.MpegTsPESPacket{
			Header: mpegts.MpegTsPESHeader{
				PacketStartCodePrefix: 0x000001,
				StreamID:              mpegts.STREAM_ID_AUDIO,
			},
		}
		// 设置可选字段
		pesPacket.Header.ConstTen = 0x80
		pesPacket.Header.PtsDtsFlags = 0x80 // 只有PTS
		pesPacket.Header.PesHeaderDataLength = 5
		pesPacket.Header.Pts = uint64(sample.Timestamp)

		pesPacket.Buffers = append(pesPacket.Buffers, sample.Data)

		// 写入TS包
		err := w.WritePESPacket(&audioFrame, pesPacket)
		if err != nil {
			w.plugin.Error("Failed to write audio PES packet", "error", err)
			return err
		}
	}

	return nil
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
