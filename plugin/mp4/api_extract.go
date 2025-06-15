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
	"image"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	m7s "m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/util"
	mp4 "m7s.live/v5/plugin/mp4/pkg"
	"m7s.live/v5/plugin/mp4/pkg/box"
)

/*
根据时间范围提取视频片段
njtv/glgc.mp4?
start=1748620153000&
end=1748620453000&
outputPath=/opt/njtv/1748620153000.mp4
*/
func (p *MP4Plugin) extractClipToFile(streamPath string, startTime, endTime time.Time, outputPath string) error {
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
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outputFile.Close()

	p.Info("extracting clip", "streamPath", streamPath, "start", startTime, "end", endTime, "output", outputPath)

	muxer := mp4.NewMuxer(flag)
	ftyp := muxer.CreateFTYPBox()
	n := ftyp.Size()
	muxer.CurrentOffset = int64(n)
	var lastTs, tsOffset int64
	var parts []*ContentPart
	sampleOffset := muxer.CurrentOffset + mp4.BeforeMdatData
	mdatOffset := sampleOffset
	var audioTrack, videoTrack *mp4.Track
	var file *os.File
	var moov box.IBox
	streamCount := len(streams)

	// Track ExtraData history for each track
	type TrackHistory struct {
		Track     *mp4.Track
		ExtraData []byte
	}
	var audioHistory, videoHistory []TrackHistory

	addAudioTrack := func(track *mp4.Track) {
		t := muxer.AddTrack(track.Cid)
		t.ExtraData = track.ExtraData
		t.SampleSize = track.SampleSize
		t.SampleRate = track.SampleRate
		t.ChannelCount = track.ChannelCount
		if len(audioHistory) > 0 {
			t.Samplelist = audioHistory[len(audioHistory)-1].Track.Samplelist
		}
		audioTrack = t
		audioHistory = append(audioHistory, TrackHistory{Track: t, ExtraData: track.ExtraData})
	}

	addVideoTrack := func(track *mp4.Track) {
		t := muxer.AddTrack(track.Cid)
		t.ExtraData = track.ExtraData
		t.Width = track.Width
		t.Height = track.Height
		if len(videoHistory) > 0 {
			t.Samplelist = videoHistory[len(videoHistory)-1].Track.Samplelist
		}
		videoTrack = t
		videoHistory = append(videoHistory, TrackHistory{Track: t, ExtraData: track.ExtraData})
	}

	addTrack := func(track *mp4.Track) {
		var lastAudioTrack, lastVideoTrack *TrackHistory
		if len(audioHistory) > 0 {
			lastAudioTrack = &audioHistory[len(audioHistory)-1]
		}
		if len(videoHistory) > 0 {
			lastVideoTrack = &videoHistory[len(videoHistory)-1]
		}
		if track.Cid.IsAudio() {
			if lastAudioTrack == nil {
				addAudioTrack(track)
			} else if !bytes.Equal(lastAudioTrack.ExtraData, track.ExtraData) {
				for _, history := range audioHistory {
					if bytes.Equal(history.ExtraData, track.ExtraData) {
						audioTrack = history.Track
						audioTrack.Samplelist = audioHistory[len(audioHistory)-1].Track.Samplelist
						return
					}
				}
				addAudioTrack(track)
			}
		} else if track.Cid.IsVideo() {
			if lastVideoTrack == nil {
				addVideoTrack(track)
			} else if !bytes.Equal(lastVideoTrack.ExtraData, track.ExtraData) {
				for _, history := range videoHistory {
					if bytes.Equal(history.ExtraData, track.ExtraData) {
						videoTrack = history.Track
						videoTrack.Samplelist = videoHistory[len(videoHistory)-1].Track.Samplelist
						return
					}
				}
				addVideoTrack(track)
			}
		}
	}

	// 处理每个片段
	for i, stream := range streams {
		tsOffset = lastTs
		file, err = os.Open(stream.FilePath)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %v", stream.FilePath, err)
		}
		defer file.Close()

		p.Info("processing segment", "file", file.Name())
		demuxer := mp4.NewDemuxer(file)
		err = demuxer.Demux()
		if err != nil {
			return fmt.Errorf("demux error: %v", err)
		}

		trackCount := len(demuxer.Tracks)
		if i == 0 || flag == mp4.FLAG_FRAGMENT {
			for _, track := range demuxer.Tracks {
				addTrack(track)
			}
		}

		if trackCount != len(muxer.Tracks) {
			if flag == mp4.FLAG_FRAGMENT {
				moov = muxer.MakeMoov()
			}
		}

		if i == 0 {
			startTimestamp := startTime.Sub(stream.StartTime).Milliseconds()
			if startTimestamp < 0 {
				startTimestamp = 0
			}
			var startSample *box.Sample
			if startSample, err = demuxer.SeekTimePreIDR(uint64(startTimestamp)); err != nil {
				tsOffset = 0
				continue
			}
			tsOffset = -int64(startSample.Timestamp)
		}

		var part *ContentPart
		for track, sample := range demuxer.RangeSample {
			if i == streamCount-1 && int64(sample.Timestamp) > endTime.Sub(stream.StartTime).Milliseconds() {
				break
			}

			if part == nil {
				part = &ContentPart{
					File:  file,
					Start: sample.Offset,
				}
			}

			lastTs = int64(sample.Timestamp + uint32(tsOffset))
			fixSample := *sample
			fixSample.Timestamp += uint32(tsOffset)

			if flag == 0 {
				fixSample.Offset = sampleOffset + (fixSample.Offset - part.Start)
				part.Size += sample.Size
				if track.Cid.IsAudio() {
					audioTrack.AddSampleEntry(fixSample)
				} else if track.Cid.IsVideo() {
					videoTrack.AddSampleEntry(fixSample)
				}
			} else {
				part.Seek(sample.Offset, io.SeekStart)
				fixSample.Data = make([]byte, sample.Size)
				part.Read(fixSample.Data)
				var moof, mdat box.IBox
				if track.Cid.IsAudio() {
					moof, mdat = muxer.CreateFlagment(audioTrack, fixSample)
				} else if track.Cid.IsVideo() {
					moof, mdat = muxer.CreateFlagment(videoTrack, fixSample)
				}
				if moof != nil {
					part.boxies = append(part.boxies, moof, mdat)
					part.Size += int(moof.Size() + mdat.Size())
				}
			}
		}

		if part != nil {
			sampleOffset += int64(part.Size)
			parts = append(parts, part)
		}
	}

	// 写入输出文件
	if flag == 0 {
		moovSize := muxer.MakeMoov().Size()
		dataSize := uint64(sampleOffset - mdatOffset)

		// 调整sample偏移量
		for _, track := range muxer.Tracks {
			for i := range track.Samplelist {
				track.Samplelist[i].Offset += int64(moovSize)
			}
		}

		mdatBox := box.CreateBaseBox(box.TypeMDAT, dataSize+box.BasicBoxLen)

		var freeBox *box.FreeBox
		if mdatBox.HeaderSize() == box.BasicBoxLen {
			freeBox = box.CreateFreeBox(nil)
		}

		// 写入文件头
		_, err = box.WriteTo(outputFile, ftyp, muxer.MakeMoov(), freeBox, mdatBox)
		if err != nil {
			return fmt.Errorf("failed to write header: %v", err)
		}

		// 写入媒体数据
		for _, part := range parts {
			part.Seek(part.Start, io.SeekStart)
			_, err = io.CopyN(outputFile, part.File, int64(part.Size))
			if err != nil {
				return fmt.Errorf("failed to write media data: %v", err)
			}
			part.Close()
		}
	} else {
		var children []box.IBox
		children = append(children, ftyp, moov)
		for _, part := range parts {
			children = append(children, part.boxies...)
			part.Close()
		}
		_, err = box.WriteTo(outputFile, children...)
		if err != nil {
			return fmt.Errorf("failed to write fragmented MP4: %v", err)
		}
	}

	p.Info("clip saved successfully", "path", outputPath)
	return nil
}

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
func (p *MP4Plugin) extractCompressedVideo(streamPath string, startTime, endTime time.Time, outputPath string, gopSeconds float64, gopInterval int) error {
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
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outputFile.Close()

	p.Info("extracting compressed video", "streamPath", streamPath, "start", startTime, "end", endTime,
		"output", outputPath, "gopSeconds", gopSeconds, "gopInterval", gopInterval)

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
				if extraData == nil || !bytes.Equal(extraData, track.ExtraData) {
					extraData = track.ExtraData
					if videoTrack == nil {
						videoTrack = muxer.AddTrack(track.Cid)
						videoTrack.ExtraData = extraData
						videoTrack.Width = track.Width
						videoTrack.Height = track.Height
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
			startSample, err := demuxer.SeekTimePreIDR(uint64(startTimestamp))
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
				Data:      data,
				Timestamp: adjustedTimestamp,
				Offset:    sampleOffset,
				Size:      sample.Size,
				Duration:  sample.Duration,
			}

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
		_, err = box.WriteTo(outputFile, ftyp, muxer.MakeMoov(), freeBox, mdatBox)
		if err != nil {
			return fmt.Errorf("failed to write header: %v", err)
		}

		for _, track := range muxer.Tracks {
			for i := range track.Samplelist {
				track.Samplelist[i].Offset += int64(moovSize)
				if _, err := outputFile.Write(track.Samplelist[i].Data); err != nil {
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

		_, err = box.WriteTo(outputFile, children...)
		if err != nil {
			return fmt.Errorf("failed to write fragmented MP4: %v", err)
		}
	}

	p.Info("compressed video saved", "path", outputPath,
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
func (p *MP4Plugin) extractGopVideo(streamPath string, targetTime time.Time, outputPath string) (float64, error) {
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
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create output file: %v", err)
	}
	defer outputFile.Close()

	p.Info("extracting compressed video", "streamPath", streamPath, "targetTime", targetTime,
		"output", outputPath)

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
				if extraData == nil || !bytes.Equal(extraData, track.ExtraData) {
					extraData = track.ExtraData
					if videoTrack == nil {
						videoTrack = muxer.AddTrack(track.Cid)
						videoTrack.ExtraData = extraData
						videoTrack.Width = track.Width
						videoTrack.Height = track.Height
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
		startSample, err := demuxer.SeekTimePreIDR(uint64(startTimestamp))
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
				Data:      data,
				Timestamp: adjustedTimestamp,
				Offset:    sampleOffset,
				Size:      sample.Size,
				Duration:  sample.Duration,
			}

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
		_, err = box.WriteTo(outputFile, ftyp, muxer.MakeMoov(), freeBox, mdatBox)
		if err != nil {
			return 0, fmt.Errorf("failed to write header: %v", err)
		}

		for _, track := range muxer.Tracks {
			for i := range track.Samplelist {
				track.Samplelist[i].Offset += int64(moovSize)
				if _, err := outputFile.Write(track.Samplelist[i].Data); err != nil {
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

		_, err = box.WriteTo(outputFile, children...)
		if err != nil {
			return 0, fmt.Errorf("failed to write fragmented MP4: %v", err)
		}
	}
	p.Info("extract gop video saved", "path", outputPath,
		"targetTime", targetTime,
		"compressedDuration", videoDuration,
		"gopElapsed", gopElapsed,
		"frameCount", len(filteredSamples),
		"fps", 25)
	return gopElapsed, nil
}

/*
根据时间范围提取视频片段
njtv/glgc.mp4?
timest=1748620153000&
outputPath=/opt/njtv/gop_tmp_1748620153000.mp4

原理：根据时间戳找到最近的mp4文件，再从mp4 文件中找到最近gop 生成mp4 文件
*/
func (p *MP4Plugin) snapImage(streamPath string, targetTime time.Time) (image.Image, error) {
	if p.DB == nil {
		return nil, pkg.ErrNoDB
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
		return nil, fmt.Errorf("no matching MP4 segments found")
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
	targetFrameInterval := 40 // 25fps对应的毫秒间隔 (1000/25=40ms)
	var filteredSamples []box.Sample
	var sampleIdx = 0
	// 仅处理视频轨道
	for _, stream := range streams {
		file, err := os.Open(stream.FilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open file %s: %v", stream.FilePath, err)
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
				if extraData == nil || !bytes.Equal(extraData, track.ExtraData) {
					extraData = track.ExtraData
					if videoTrack == nil {
						videoTrack = muxer.AddTrack(track.Cid)
						videoTrack.ExtraData = extraData
						videoTrack.Width = track.Width
						videoTrack.Height = track.Height
					}
				}
				break
			}
		}

		if !hasVideo {
			p.Warn("no video track found in segment", "file", stream.FilePath)
			continue
		}

		//p.Info("extractGop", "SPS PPS", Bytes2HexStr(videoTrack.ExtraData, len(videoTrack.ExtraData)))

		// 处理起始时间边界
		var tsOffset int64

		startTimestamp := targetTime.Sub(stream.StartTime).Milliseconds()

		// p.Info("extractGop",
		// 	"Timescale", videoTrack.Timescale,
		// 	"targetTime", targetTime,
		// 	"stream.StartTime", stream.StartTime,
		// 	"startTimestamp", startTimestamp)

		if startTimestamp < 0 {
			startTimestamp = 0
		}
		//通过时间戳定位到最近的‌关键帧‌（如视频IDR帧），返回的startSample是该关键帧对应的样本
		startSample, err := demuxer.SeekTimePreIDR(uint64(startTimestamp))
		if err == nil {
			tsOffset = -int64(startSample.Timestamp)
		}

		// p.Info("extractGop", "startSample Timestamp",
		// 	startSample.Timestamp)

		// 处理样本
		//RangeSample迭代的是‌当前时间范围内的所有样本‌（可能包含非关键帧），顺序取决于MP4文件中样本的物理存储顺序
		for track, sample := range demuxer.RangeSample {
			if !track.Cid.IsVideo() {
				continue
			}

			if sample.Timestamp < startSample.Timestamp {
				p.Info("extractGop", "KeyFrame", sample.KeyFrame,
					"CTS", sample.CTS,
					"Timestamp", sample.Timestamp,
					"Offset", sample.Offset,
					"Size", sample.Size,
					"Duration", sample.Duration)

				continue
			}
			//记录GOP内帧的序号，没有考虑B帧的情况
			if sample.Timestamp < uint32(startTimestamp) {
				sampleIdx++
			}

			adjustedTimestamp := sample.Timestamp + uint32(tsOffset)

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
			// 检查是否超过gopSeconds限制

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

			// p.Info("extractGop", "KeyFrame", sample.KeyFrame,
			// 	"CTS", sample.CTS,
			// 	"Timestamp", sample.Timestamp,
			// 	"Offset", sample.Offset,
			// 	"Size", sample.Size,
			// 	"Duration", sample.Duration,
			// 	"Data", Bytes2HexStr(data, 32))

			// 创建新的样本
			newSample := box.Sample{
				KeyFrame:  sample.KeyFrame,
				Data:      data,
				Timestamp: adjustedTimestamp,
				Offset:    sampleOffset,
				Size:      sample.Size,
				Duration:  sample.Duration,
			}

			sampleOffset += int64(newSample.Size)
			filteredSamples = append(filteredSamples, newSample)
		}
	}

	if len(filteredSamples) == 0 {
		return nil, fmt.Errorf("no valid video samples found")
	}

	// 按25fps重新计算时间戳
	for i := range filteredSamples {
		filteredSamples[i].Timestamp = uint32(i * targetFrameInterval)
	}

	p.Info("extract gop and snap",
		"targetTime", targetTime,
		"frist", filteredSamples[0].Timestamp,
		"sampleIdx", sampleIdx,
		"frameCount", len(filteredSamples))

	img, err := ProcessWithFFmpeg(filteredSamples, sampleIdx, videoTrack)
	if err != nil {
		return nil, err
	}
	// 添加样本到轨道
	p.Info("extract gop and snap saved",
		"targetTime", targetTime,
		"frameCount", len(filteredSamples))

	return img, nil
}

/*
提取普通MP4视频
GET http://192.168.0.238:8080/mp4/extractClip/njtv/glgc.mp4?

	start=1748620153000&
	end=1748620453000&
	outputPath=/opt/njtv/1748620153000.mp4
*/
func (p *MP4Plugin) extractClipToFileHandel(w http.ResponseWriter, r *http.Request) {
	streamPath := r.PathValue("streamPath")
	query := r.URL.Query()
	// 合并多个 mp4
	startTime, endTime, err := util.TimeRangeQueryParse(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p.Info("extractClipToFileHandel", "streamPath", streamPath, "start", startTime, "end", endTime)

	outputPath := query.Get("outputPath")

	p.extractClipToFile(streamPath, startTime, endTime, outputPath)

	// 返回成功响应
	w.WriteHeader(http.StatusOK)
}

/*
提取压缩视频

GET http://192.168.0.238:8080/mp4/extractCompressed/
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
	p.Info("extractClipToFileHandel", "streamPath", streamPath, "start", startTime, "end", endTime)

	outputPath := query.Get("outputPath")
	gopSeconds, _ := strconv.ParseFloat(query.Get("gopSeconds"), 64)
	gopInterval, _ := strconv.Atoi(query.Get("gopInterval"))

	if gopSeconds == 0 {
		gopSeconds = 1
	}
	if gopInterval == 0 {
		gopInterval = 1
	}

	p.extractCompressedVideo(streamPath, startTime, endTime, outputPath, gopSeconds, gopInterval)

	// 返回成功响应
	w.WriteHeader(http.StatusOK)
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

	outputPath := query.Get("outputPath")
	p.extractGopVideo(streamPath, targetTime, outputPath)

	// 返回成功响应
	w.WriteHeader(http.StatusOK)
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

	outputPath := query.Get("outputPath")
	img, err := p.snapImage(streamPath, targetTime)
	if err == nil {
		//水印测试
		// wImg, err := watermark.WatermarkTest(img)
		// if err != nil {
		// 	p.Info("watermarkTest", "err", err)
		// 	http.Error(w, err.Error(), http.StatusBadRequest)
		// 	return
		// }
		//saveAsJPG(wImg, outputPath)
		saveAsJPG(img, outputPath)
	} else {
		p.Info("snapHandel", "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// 返回成功响应
	w.WriteHeader(http.StatusOK)
}
