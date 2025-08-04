/**
 * @file 文件名.h
 * @brief MP4 文件查询提取功能,GOP提取新的MP4，片段提取图片等，已验证测试H264,H265
 * @author erroot
 * @date 250614
 * @version 1.0.0
 */

package plugin_mp4

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	m7s "m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/pkg/util"
	mp4 "m7s.live/v5/plugin/mp4/pkg"
	"m7s.live/v5/plugin/mp4/pkg/box"
)

// bytes2hexStr 将字节数组前n个字节转为16进制字符串
// data: 原始字节数组
// length: 需要转换的字节数（超过实际长度时自动截断）
func Bytes2HexStr(data []byte, length int) string {
	if length > len(data) {
		length = len(data)
	}

	var builder strings.Builder
	for i := 0; i < length; i++ {
		if i > 0 {
			builder.WriteString(" ")
		}
		builder.WriteString(fmt.Sprintf("%02X", data[i]))
	}
	return builder.String()
}

/*
提取压缩视频（快放视频）

njtv/glgc.mp4?
start=1748620153000&
end=1748620453000&
outputPath=/opt/njtv/1748620153000.mp4
gopSeconds=1&
gopInterval=1&

FLAG_FRAGMENT  暂时不支持没有调试

假设原生帧率25fps   GOP = 50 frame
时间范围: endTime-startTime = 300s   = 7500 frame =   150 GOP
gopSeconds=0.2   6 frame
gopInterval=10
提取结果15 gop,   90 frame ,  90/25 = 3.6 s

反过推算 要求 5范围分钟 压缩到15s 播放完
当gopSeconds=0.1， 推算 gopInterval=1
当gopSeconds=0.2， 推算 gopInterval=2
*/
func (p *MP4Plugin) extractCompressedVideo(streamPath string, startTime, endTime time.Time, writer io.Writer, gopSeconds float64, gopInterval int) error {
	if p.DB == nil {
		return pkg.ErrNoDB
	}

	var flag mp4.Flag
	if strings.HasSuffix(streamPath, ".fmp4") {
		flag = mp4.FLAG_FRAGMENT
		streamPath = strings.TrimSuffix(streamPath, ".fmp4")
	} else {
		streamPath = strings.TrimSuffix(streamPath, ".mp4")
	}

	// 查询数据库获取符合条件的片段
	queryRecord := m7s.RecordStream{
		Type: "mp4",
	}
	var streams []m7s.RecordStream
	p.DB.Where(&queryRecord).Find(&streams, "end_time>? AND start_time<? AND stream_path=?", startTime, endTime, streamPath)
	if len(streams) == 0 {
		return fmt.Errorf("no matching MP4 segments found")
	}

	// 创建输出文件
	outputFile := writer

	p.Info("extracting compressed video", "streamPath", streamPath, "start", startTime, "end", endTime,
		"gopSeconds", gopSeconds, "gopInterval", gopInterval)

	muxer := mp4.NewMuxer(flag)
	ftyp := muxer.CreateFTYPBox()
	n := ftyp.Size()
	muxer.CurrentOffset = int64(n)
	var videoTrack *mp4.Track
	sampleOffset := muxer.CurrentOffset + mp4.BeforeMdatData
	mdatOffset := sampleOffset

	//var audioTrack *mp4.Track
	var extraData []byte

	// 压缩相关变量
	currentGOPCount := -1
	inGOP := false
	targetFrameInterval := 40 // 25fps对应的毫秒间隔 (1000/25=40ms)
	var filteredSamples []box.Sample
	//var lastVideoTimestamp uint32
	var timescale uint32 = 1000 // 默认时间刻度为1000 (毫秒)
	var currentGopStartTime int64 = -1

	// 仅处理视频轨道
	for i, stream := range streams {
		file, err := os.Open(stream.FilePath)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %v", stream.FilePath, err)
		}
		defer file.Close()

		p.Info("processing segment", "file", file.Name())
		demuxer := mp4.NewDemuxer(file)
		err = demuxer.Demux()
		if err != nil {
			p.Warn("demux error, skipping segment", "error", err, "file", stream.FilePath)
			continue
		}

		// 确保有视频轨道
		var hasVideo bool
		for _, track := range demuxer.Tracks {
			if track.Cid.IsVideo() {
				hasVideo = true
				// 只在第一个片段或关键帧变化时更新extraData
				trackExtraData := track.GetRecord()
				if extraData == nil || !bytes.Equal(extraData, trackExtraData) {
					extraData = trackExtraData
					if videoTrack == nil {
						videoTrack = muxer.AddTrack(track.Cid)
						videoTrack.ICodecCtx = track.ICodecCtx
					}
				}
				break
			}
		}

		if !hasVideo {
			p.Warn("no video track found in segment", "file", stream.FilePath)
			continue
		}

		// 处理起始时间边界
		var tsOffset int64
		if i == 0 {
			startTimestamp := startTime.Sub(stream.StartTime).Milliseconds()
			if startTimestamp < 0 {
				startTimestamp = 0
			}
			startSample, err := demuxer.SeekTime(uint64(startTimestamp))
			if err == nil {
				tsOffset = -int64(startSample.Timestamp)
			}
		}

		// 处理样本
		for track, sample := range demuxer.RangeSample {
			if !track.Cid.IsVideo() {
				continue
			}

			//for _, sample := range samples {
			adjustedTimestamp := sample.Timestamp + uint32(tsOffset)

			// 处理GOP逻辑
			if sample.KeyFrame {
				currentGOPCount++
				inGOP = false
				if currentGOPCount%gopInterval == 0 {
					currentGopStartTime = int64(sample.Timestamp)
					inGOP = true
				}
			}

			// 跳过不在当前GOP的帧
			if !inGOP {
				currentGopStartTime = -1
				continue
			}

			// 如果不在有效的GOP中，跳过
			if currentGopStartTime == -1 {
				continue
			}

			// 检查是否超过gopSeconds限制
			currentTime := int64(sample.Timestamp)
			gopElapsed := float64(currentTime-currentGopStartTime) / float64(timescale)
			if gopSeconds > 0 && gopElapsed > gopSeconds {
				continue
			}

			// 处理结束时间边界
			if i == len(streams)-1 && int64(adjustedTimestamp) > endTime.Sub(streams[0].StartTime).Milliseconds() {
				continue
			}

			// 确保样本数据有效
			if sample.Size <= 0 || sample.Size > 10*1024*1024 { // 10MB限制
				p.Warn("invalid sample size", "size", sample.Size, "timestamp", sample.Timestamp)
				continue
			}

			// 读取样本数据
			if _, err := file.Seek(sample.Offset, io.SeekStart); err != nil {
				p.Warn("seek error", "error", err, "offset", sample.Offset)
				continue
			}
			data := make([]byte, sample.Size)
			if _, err := io.ReadFull(file, data); err != nil {
				p.Warn("read sample error", "error", err, "size", sample.Size)
				continue
			}

			// 创建新的样本
			newSample := box.Sample{
				KeyFrame:  sample.KeyFrame,
				Timestamp: adjustedTimestamp,
				Offset:    sampleOffset,
				Duration:  sample.Duration,
			}
			newSample.PushOne(data)
			// p.Info("Compressed", "KeyFrame", newSample.KeyFrame,
			// 	"CTS", newSample.CTS,
			// 	"Timestamp", newSample.Timestamp,
			// 	"Offset", newSample.Offset,
			// 	"Size", newSample.Size,
			// 	"Duration", newSample.Duration,
			// 	"Data", Bytes2HexStr(newSample.Data, 16))

			sampleOffset += int64(newSample.Size)
			filteredSamples = append(filteredSamples, newSample)

		}
	}

	if len(filteredSamples) == 0 {
		return fmt.Errorf("no valid video samples found")
	}

	// 按25fps重新计算时间戳
	for i := range filteredSamples {
		filteredSamples[i].Timestamp = uint32(i * targetFrameInterval)
	}

	// 添加样本到轨道
	for _, sample := range filteredSamples {
		videoTrack.AddSampleEntry(sample)
	}

	// 计算视频时长
	videoDuration := uint32(len(filteredSamples) * targetFrameInterval)

	// 写入输出文件
	if flag == 0 {
		// 非分片MP4处理
		moovSize := muxer.MakeMoov().Size()
		dataSize := uint64(sampleOffset - mdatOffset)

		// 调整sample偏移量
		for _, track := range muxer.Tracks {
			for i := range track.Samplelist {
				track.Samplelist[i].Offset += int64(moovSize)
			}
		}

		// 创建MDAT盒子 (添加8字节头)
		mdatHeaderSize := uint64(8)
		mdatBox := box.CreateBaseBox(box.TypeMDAT, dataSize+mdatHeaderSize)

		var freeBox *box.FreeBox
		if mdatBox.HeaderSize() == box.BasicBoxLen {
			freeBox = box.CreateFreeBox(nil)
		}

		// 写入文件头
		_, err := box.WriteTo(outputFile, ftyp, muxer.MakeMoov(), freeBox, mdatBox)
		if err != nil {
			return fmt.Errorf("failed to write header: %v", err)
		}

		for _, track := range muxer.Tracks {
			for i := range track.Samplelist {
				track.Samplelist[i].Offset += int64(moovSize)
				if _, err := outputFile.Write(track.Samplelist[i].Buffers[0]); err != nil {
					return err
				}
			}
		}
	} else {
		// 分片MP4处理
		var children []box.IBox
		moov := muxer.MakeMoov()
		children = append(children, ftyp, moov)

		// 创建分片
		for _, sample := range filteredSamples {
			moof, mdat := muxer.CreateFlagment(videoTrack, sample)
			children = append(children, moof, mdat)
		}

		_, err := box.WriteTo(outputFile, children...)
		if err != nil {
			return fmt.Errorf("failed to write fragmented MP4: %v", err)
		}
	}

	p.Info("compressed video saved",
		"originalDuration", (endTime.Sub(startTime)).Milliseconds(),
		"compressedDuration", videoDuration,
		"frameCount", len(filteredSamples),
		"fps", 25)
	return nil
}

/*
根据时间范围提取视频片段
njtv/glgc.mp4?
timest=1748620153000&
outputPath=/opt/njtv/gop_tmp_1748620153000.mp4

原理：根据时间戳找到最近的mp4文件，再从mp4 文件中找到最近gop 生成mp4 文件
*/
func (p *MP4Plugin) extractGopVideo(streamPath string, targetTime time.Time, writer io.Writer) (float64, error) {
	if p.DB == nil {
		return 0, pkg.ErrNoDB
	}

	var flag mp4.Flag
	if strings.HasSuffix(streamPath, ".fmp4") {
		flag = mp4.FLAG_FRAGMENT
		streamPath = strings.TrimSuffix(streamPath, ".fmp4")
	} else {
		streamPath = strings.TrimSuffix(streamPath, ".mp4")
	}

	// 查询数据库获取符合条件的片段
	queryRecord := m7s.RecordStream{
		Type: "mp4",
	}
	var streams []m7s.RecordStream
	p.DB.Where(&queryRecord).Find(&streams, "end_time>=? AND start_time<=? AND stream_path=?", targetTime, targetTime, streamPath)
	if len(streams) == 0 {
		return 0, fmt.Errorf("no matching MP4 segments found")
	}

	// 创建输出文件
	outputFile := writer

	p.Info("extracting compressed video", "streamPath", streamPath, "targetTime", targetTime)

	muxer := mp4.NewMuxer(flag)
	ftyp := muxer.CreateFTYPBox()
	n := ftyp.Size()
	muxer.CurrentOffset = int64(n)
	var videoTrack *mp4.Track
	sampleOffset := muxer.CurrentOffset + mp4.BeforeMdatData
	mdatOffset := sampleOffset

	//var audioTrack *mp4.Track
	var extraData []byte

	// 压缩相关变量
	findGOP := false
	targetFrameInterval := 40 // 25fps对应的毫秒间隔 (1000/25=40ms)
	var filteredSamples []box.Sample
	//var lastVideoTimestamp uint32
	var timescale uint32 = 1000 // 默认时间刻度为1000 (毫秒)
	var currentGopStartTime int64 = -1
	var gopElapsed float64 = 0
	// 仅处理视频轨道
	for _, stream := range streams {
		file, err := os.Open(stream.FilePath)
		if err != nil {
			return 0, fmt.Errorf("failed to open file %s: %v", stream.FilePath, err)
		}
		defer file.Close()

		p.Info("processing segment", "file", file.Name())
		demuxer := mp4.NewDemuxer(file)
		err = demuxer.Demux()
		if err != nil {
			p.Warn("demux error, skipping segment", "error", err, "file", stream.FilePath)
			continue
		}

		// 确保有视频轨道
		var hasVideo bool
		for _, track := range demuxer.Tracks {
			if track.Cid.IsVideo() {
				hasVideo = true
				// 只在第一个片段或关键帧变化时更新extraData
				trackExtraData := track.GetRecord()
				if extraData == nil || !bytes.Equal(extraData, trackExtraData) {
					extraData = trackExtraData
					if videoTrack == nil {
						videoTrack = muxer.AddTrack(track.Cid)
						videoTrack.ICodecCtx = track.ICodecCtx
					}
				}
				break
			}
		}

		if !hasVideo {
			p.Warn("no video track found in segment", "file", stream.FilePath)
			continue
		}

		// 处理起始时间边界
		var tsOffset int64

		startTimestamp := targetTime.Sub(stream.StartTime).Milliseconds()

		// p.Info("extractGop", "targetTime", targetTime,
		// 	"stream.StartTime", stream.StartTime,
		// 	"startTimestamp", startTimestamp)

		if startTimestamp < 0 {
			startTimestamp = 0
		}
		//通过时间戳定位到最近的‌关键帧‌（如视频IDR帧），返回的startSample是该关键帧对应的样本
		startSample, err := demuxer.SeekTime(uint64(startTimestamp))
		if err == nil {
			tsOffset = -int64(startSample.Timestamp)
		}

		//p.Info("extractGop", "startSample", startSample)

		// 处理样本
		//RangeSample迭代的是‌当前时间范围内的所有样本‌（可能包含非关键帧），顺序取决于MP4文件中样本的物理存储顺序
		for track, sample := range demuxer.RangeSample {
			if !track.Cid.IsVideo() {
				continue
			}

			if sample.Timestamp < startSample.Timestamp {
				continue
			}

			//for _, sample := range samples {
			adjustedTimestamp := sample.Timestamp + uint32(tsOffset)

			// 处理GOP逻辑,已经处理完上一个gop
			if sample.KeyFrame && findGOP {
				break
			}

			// 处理GOP逻辑
			if sample.KeyFrame && !findGOP {
				findGOP = true
				currentGopStartTime = int64(sample.Timestamp)
			}

			// 跳过不在当前GOP的帧
			if !findGOP {
				currentGopStartTime = -1
				continue
			}
			// 检查是否超过gopSeconds限制
			currentTime := int64(sample.Timestamp)
			gopElapsed = float64(currentTime-currentGopStartTime) / float64(timescale)

			// 确保样本数据有效
			if sample.Size <= 0 || sample.Size > 10*1024*1024 { // 10MB限制
				p.Warn("invalid sample size", "size", sample.Size, "timestamp", sample.Timestamp)
				continue
			}

			// 读取样本数据
			if _, err := file.Seek(sample.Offset, io.SeekStart); err != nil {
				p.Warn("seek error", "error", err, "offset", sample.Offset)
				continue
			}
			data := make([]byte, sample.Size)
			if _, err := io.ReadFull(file, data); err != nil {
				p.Warn("read sample error", "error", err, "size", sample.Size)
				continue
			}

			// 创建新的样本
			newSample := box.Sample{
				KeyFrame:  sample.KeyFrame,
				Timestamp: adjustedTimestamp,
				Offset:    sampleOffset,
				Duration:  sample.Duration,
			}
			newSample.PushOne(data)

			// p.Info("extractGop", "KeyFrame", newSample.KeyFrame,
			// 	"CTS", newSample.CTS,
			// 	"Timestamp", newSample.Timestamp,
			// 	"Offset", newSample.Offset,
			// 	"Size", newSample.Size,
			// 	"Duration", newSample.Duration,
			// 	"Data", Bytes2HexStr(newSample.Data, 16))

			sampleOffset += int64(newSample.Size)
			filteredSamples = append(filteredSamples, newSample)

		}
	}

	if len(filteredSamples) == 0 {
		return 0, fmt.Errorf("no valid video samples found")
	}

	// 按25fps重新计算时间戳
	for i := range filteredSamples {
		filteredSamples[i].Timestamp = uint32(i * targetFrameInterval)
	}

	// 添加样本到轨道
	for _, sample := range filteredSamples {
		videoTrack.AddSampleEntry(sample)
	}

	// 计算视频时长
	videoDuration := uint32(len(filteredSamples) * targetFrameInterval)

	// 写入输出文件
	if flag == 0 {
		// 非分片MP4处理
		moovSize := muxer.MakeMoov().Size()
		dataSize := uint64(sampleOffset - mdatOffset)

		// 调整sample偏移量
		for _, track := range muxer.Tracks {
			for i := range track.Samplelist {
				track.Samplelist[i].Offset += int64(moovSize)
			}
		}

		// 创建MDAT盒子 (添加8字节头)
		mdatHeaderSize := uint64(8)
		mdatBox := box.CreateBaseBox(box.TypeMDAT, dataSize+mdatHeaderSize)

		var freeBox *box.FreeBox
		if mdatBox.HeaderSize() == box.BasicBoxLen {
			freeBox = box.CreateFreeBox(nil)
		}

		// 写入文件头
		_, err := box.WriteTo(outputFile, ftyp, muxer.MakeMoov(), freeBox, mdatBox)
		if err != nil {
			return 0, fmt.Errorf("failed to write header: %v", err)
		}

		for _, track := range muxer.Tracks {
			for i := range track.Samplelist {
				track.Samplelist[i].Offset += int64(moovSize)
				if _, err := outputFile.Write(track.Samplelist[i].Buffers[0]); err != nil {
					return 0, err
				}
			}
		}
	} else {
		// 分片MP4处理
		var children []box.IBox
		moov := muxer.MakeMoov()
		children = append(children, ftyp, moov)

		// 创建分片
		for _, sample := range filteredSamples {
			moof, mdat := muxer.CreateFlagment(videoTrack, sample)
			children = append(children, moof, mdat)
		}

		_, err := box.WriteTo(outputFile, children...)
		if err != nil {
			return 0, fmt.Errorf("failed to write fragmented MP4: %v", err)
		}
	}
	p.Info("extract gop video saved",
		"targetTime", targetTime,
		"compressedDuration", videoDuration,
		"gopElapsed", gopElapsed,
		"frameCount", len(filteredSamples),
		"fps", 25)
	return gopElapsed, nil
}

/*
提取压缩视频

GET http://192.168.0.238:8080/mp4/extract/compressed/
njtv/glgc.mp4?
start=1748620153000&
end=1748620453000&
outputPath=/opt/njtv/1748620153000.mp4
gopSeconds=1&
gopInterval=1&
*/
func (p *MP4Plugin) extractCompressedVideoHandel(w http.ResponseWriter, r *http.Request) {
	streamPath := r.PathValue("streamPath")
	query := r.URL.Query()
	// 合并多个 mp4
	startTime, endTime, err := util.TimeRangeQueryParse(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p.Info("extractCompressedVideoHandel", "streamPath", streamPath, "start", startTime, "end", endTime)

	gopSeconds, _ := strconv.ParseFloat(query.Get("gopSeconds"), 64)
	gopInterval, _ := strconv.Atoi(query.Get("gopInterval"))

	if gopSeconds == 0 {
		gopSeconds = 1
	}
	if gopInterval == 0 {
		gopInterval = 1
	}

	// 设置响应头
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", "attachment; filename=\"compressed_video.mp4\"")

	err = p.extractCompressedVideo(streamPath, startTime, endTime, w, gopSeconds, gopInterval)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (p *MP4Plugin) extractGopVideoHandel(w http.ResponseWriter, r *http.Request) {
	streamPath := r.PathValue("streamPath")
	query := r.URL.Query()

	targetTimeString := query.Get("targetTime")
	// 合并多个 mp4
	targetTime, err := util.UnixTimeQueryParse(targetTimeString)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p.Info("extractGopVideoHandel", "streamPath", streamPath, "targetTime", targetTime)

	// 设置响应头
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", "attachment; filename=\"gop_video.mp4\"")

	_, err = p.extractGopVideo(streamPath, targetTime, w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (p *MP4Plugin) snapHandel(w http.ResponseWriter, r *http.Request) {
	streamPath := r.PathValue("streamPath")
	query := r.URL.Query()

	targetTimeString := query.Get("targetTime")
	// 合并多个 mp4
	targetTime, err := util.UnixTimeQueryParse(targetTimeString)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p.Info("snapHandel", "streamPath", streamPath, "targetTime", targetTime)

	// 设置响应头
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Disposition", "attachment; filename=\"snapshot.jpg\"")

	err = p.snapToWriter(streamPath, targetTime, w)
	if err != nil {
		p.Info("snapHandel", "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
}

func (p *MP4Plugin) snapToWriter(streamPath string, targetTime time.Time, writer io.Writer) error {
	if p.DB == nil {
		return pkg.ErrNoDB
	}

	var flag mp4.Flag
	if strings.HasSuffix(streamPath, ".fmp4") {
		flag = mp4.FLAG_FRAGMENT
		streamPath = strings.TrimSuffix(streamPath, ".fmp4")
	} else {
		streamPath = strings.TrimSuffix(streamPath, ".mp4")
	}

	// 查询数据库获取符合条件的片段
	queryRecord := m7s.RecordStream{
		Type: "mp4",
	}
	var streams []m7s.RecordStream
	p.DB.Where(&queryRecord).Find(&streams, "end_time>=? AND start_time<=? AND stream_path=?", targetTime, targetTime, streamPath)
	if len(streams) == 0 {
		return fmt.Errorf("no matching MP4 segments found")
	}

	muxer := mp4.NewMuxer(flag)
	ftyp := muxer.CreateFTYPBox()
	n := ftyp.Size()
	muxer.CurrentOffset = int64(n)
	var videoTrack *mp4.Track
	sampleOffset := muxer.CurrentOffset + mp4.BeforeMdatData

	//var audioTrack *mp4.Track
	var extraData []byte

	// 压缩相关变量
	findGOP := false
	var filteredSamples net.Buffers
	var sampleIdx = 0
	// 仅处理视频轨道
	for _, stream := range streams {
		file, err := os.Open(stream.FilePath)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %v", stream.FilePath, err)
		}
		defer file.Close()

		p.Info("processing segment", "file", file.Name())
		demuxer := mp4.NewDemuxer(file)
		err = demuxer.Demux()
		if err != nil {
			p.Warn("demux error, skipping segment", "error", err, "file", stream.FilePath)
			continue
		}

		// 确保有视频轨道
		var hasVideo bool
		for _, track := range demuxer.Tracks {
			if track.Cid.IsVideo() {
				hasVideo = true
				// 只在第一个片段或关键帧变化时更新extraData
				trackExtraData := track.GetRecord()
				if extraData == nil || !bytes.Equal(extraData, trackExtraData) {
					extraData = trackExtraData
					if videoTrack == nil {
						videoTrack = muxer.AddTrack(track.Cid)
						videoTrack.ICodecCtx = track.ICodecCtx
					}
				}
				break
			}
		}

		if !hasVideo {
			p.Warn("no video track found in segment", "file", stream.FilePath)
			continue
		}

		startTimestamp := targetTime.Sub(stream.StartTime).Milliseconds()

		if startTimestamp < 0 {
			startTimestamp = 0
		}
		//通过时间戳定位到最近的‌关键帧‌（如视频IDR帧），返回的startSample是该关键帧对应的样本
		startSample, err := demuxer.SeekTime(uint64(startTimestamp))
		if err == nil {
		}

		// 处理样本
		//RangeSample迭代的是‌当前时间范围内的所有样本‌（可能包含非关键帧），顺序取决于MP4文件中样本的物理存储顺序
		for track, sample := range demuxer.RangeSample {
			if !track.Cid.IsVideo() {
				continue
			}

			if sample.Timestamp < startSample.Timestamp {
				continue
			}
			//记录GOP内帧的序号，没有考虑B帧的情况
			if sample.Timestamp < uint32(startTimestamp) {
				sampleIdx++
			}

			// 处理GOP逻辑,已经处理完上一个gop
			if sample.KeyFrame && findGOP {
				break
			}

			// 处理GOP逻辑
			if sample.KeyFrame && !findGOP {
				findGOP = true
			}

			// 跳过不在当前GOP的帧
			if !findGOP {
				continue
			}

			// 确保样本数据有效
			if sample.Size <= 0 || sample.Size > 10*1024*1024 { // 10MB限制
				p.Warn("invalid sample size", "size", sample.Size, "timestamp", sample.Timestamp)
				continue
			}

			// 读取样本数据
			if _, err := file.Seek(sample.Offset, io.SeekStart); err != nil {
				p.Warn("seek error", "error", err, "offset", sample.Offset)
				continue
			}
			data := make([]byte, sample.Size)
			if _, err := io.ReadFull(file, data); err != nil {
				p.Warn("read sample error", "error", err, "size", sample.Size)
				continue
			}
			for offset := 0; offset < sample.Size; {
				nalusSize := util.BigEndian.Uint32(data[offset:])
				filteredSamples = append(filteredSamples, codec.NALU_Delimiter2[:], data[offset+4:offset+4+int(nalusSize)])
				offset += int(nalusSize) + 4
			}

			sampleOffset += int64(sample.Size)
		}
	}

	if len(filteredSamples) == 0 {
		return fmt.Errorf("no valid video samples found")
	}

	err := ProcessWithFFmpeg(filteredSamples, sampleIdx, writer)
	if err != nil {
		return err
	}

	p.Info("extract gop and snap saved",
		"targetTime", targetTime,
		"frameCount", len(filteredSamples))

	return nil
}
