package plugin_mp4

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	task "github.com/langhuihui/gotask"
	"github.com/mcuadros/go-defaults"
	"google.golang.org/protobuf/types/known/emptypb"
	m7s "m7s.live/v5"
	"m7s.live/v5/pb"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/storage"
	"m7s.live/v5/pkg/util"
	mp4pb "m7s.live/v5/plugin/mp4/pb"
	mp4 "m7s.live/v5/plugin/mp4/pkg"
	"m7s.live/v5/plugin/mp4/pkg/box"
)

type ContentPart struct {
	file   storage.File
	Start  int64
	Size   int
	boxies []box.IBox
}

func (c *ContentPart) Read(p []byte) (n int, err error) {
	return c.file.Read(p)
}

func (c *ContentPart) Seek(offset int64, whence int) (int64, error) {
	return c.file.Seek(offset, whence)
}

func (c *ContentPart) Close() error {
	return c.file.Close()
}

func (p *MP4Plugin) downloadSingleFile(stream *m7s.RecordStream, flag mp4.Flag, w http.ResponseWriter, r *http.Request) {
	// 获取文件（本地或远程）
	var file storage.File
	var err error

	// 最高优先级：如果 FilePath 是绝对路径，直接使用，跳过所有 storage 处理
	if filepath.IsAbs(stream.FilePath) {
		if flag == 0 {
			// 普通 MP4：直接 ServeFile
			http.ServeFile(w, r, stream.FilePath)
			return
		}
		// fMP4：直接打开文件
		file, err = os.Open(stream.FilePath)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to open file: %v", err), http.StatusInternalServerError)
			p.Error("failed to open file", "err", err, "path", stream.FilePath)
			return
		}
		defer file.Close()
		p.Info("reading file for fmp4 conversion from absolute path", "path", stream.FilePath)
		// 继续执行 fMP4 转换处理
	} else {
		// 相对路径：使用 storage 处理
		// 检查全局存储是否存在且类型匹配
		st := p.Server.Storage
		var globalStorageType string
		if st != nil {
			globalStorageType = st.GetKey()
		}
		useGlobalStorage := st != nil && globalStorageType == stream.StorageType
		isLocalStorage := stream.StorageType == string(storage.StorageTypeLocal) || stream.StorageType == ""

		// 对于普通 MP4，优先直接获取存储URL或路径
		if flag == 0 {
			if useGlobalStorage {
				if isLocalStorage {
					// 本地存储：根据存储级别获取完整路径
					if localStorage, ok := st.(*storage.LocalStorage); ok {
						fullPath := localStorage.GetFullPath(stream.FilePath, stream.StorageLevel)
						http.ServeFile(w, r, fullPath)
					} else {
						// 类型不匹配，使用 GetURL 作为兜底
						url, err := st.GetURL(context.Background(), stream.FilePath)
						if err != nil {
							http.Error(w, fmt.Sprintf("failed to get URL: %v", err), http.StatusInternalServerError)
							p.Error("failed to get URL", "err", err)
							return
						}
						http.ServeFile(w, r, url)
					}
				} else {
					// 其他存储类型，使用 GetURL 并重定向
					url, err := st.GetURL(context.Background(), stream.FilePath)
					if err != nil {
						http.Error(w, fmt.Sprintf("failed to get URL: %v", err), http.StatusInternalServerError)
						p.Error("failed to get URL", "err", err)
						return
					}
					p.Info("redirect to storage URL", "storageType", stream.StorageType, "url", url)
					http.Redirect(w, r, url, http.StatusFound)
				}
			} else {
				// 兜底逻辑：直接使用 stream.FilePath
				if isLocalStorage {
					http.ServeFile(w, r, stream.FilePath)
				} else {
					http.Error(w, "storage type mismatch, cannot serve file", http.StatusInternalServerError)
					p.Error("storage type mismatch", "streamType", stream.StorageType, "globalType", globalStorageType)
				}
			}
			return
		}

		// 对于 fmp4，需要读取文件进行转换（只读模式，不会上传）
		if useGlobalStorage {
			if isLocalStorage {
				// 本地存储：根据存储级别获取完整路径后打开文件
				if localStorage, ok := st.(*storage.LocalStorage); ok {
					fullPath := localStorage.GetFullPath(stream.FilePath, stream.StorageLevel)
					file, err = os.Open(fullPath)
					if err != nil {
						http.Error(w, fmt.Sprintf("failed to open local file: %v", err), http.StatusInternalServerError)
						p.Error("failed to open local file", "err", err, "path", fullPath)
						return
					}
					defer file.Close()
					p.Info("reading file for fmp4 conversion from local storage", "storageLevel", stream.StorageLevel, "path", fullPath)
				} else {
					// 类型不匹配，使用 OpenFile 作为兜底
					file, err = st.OpenFile(context.Background(), stream.FilePath)
					if err != nil {
						http.Error(w, fmt.Sprintf("failed to open file: %v", err), http.StatusInternalServerError)
						p.Error("failed to open file", "err", err)
						return
					}
					defer file.Close()
					p.Info("reading file for fmp4 conversion from global storage", "storageType", stream.StorageType, "path", stream.FilePath)
				}
			} else {
				// 其他存储类型，使用 OpenFile
				file, err = st.OpenFile(context.Background(), stream.FilePath)
				if err != nil {
					http.Error(w, fmt.Sprintf("failed to open file: %v", err), http.StatusInternalServerError)
					p.Error("failed to open file", "err", err)
					return
				}
				defer file.Close()
				p.Info("reading file for fmp4 conversion from global storage", "storageType", stream.StorageType, "path", stream.FilePath)
			}
		} else {
			// 兜底逻辑：直接使用 stream.FilePath 作为本地文件
			if isLocalStorage {
				file, err = os.Open(stream.FilePath)
				if err != nil {
					http.Error(w, fmt.Sprintf("failed to open local file: %v", err), http.StatusInternalServerError)
					p.Error("failed to open local file", "err", err)
					return
				}
				defer file.Close()
				p.Info("reading file for fmp4 conversion from local path", "path", stream.FilePath)
			} else {
				http.Error(w, "storage type mismatch, cannot open file", http.StatusInternalServerError)
				p.Error("storage type mismatch", "streamType", stream.StorageType, "globalType", globalStorageType)
				return
			}
		}
	}

	// fmp4 转换处理（本地和远程文件统一处理）
	if flag == mp4.FLAG_FRAGMENT {
		p.Info("converting to fmp4", "file", file.Name())
		demuxer := mp4.NewDemuxer(file)
		err = demuxer.Demux()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var trackMap = make(map[box.MP4_CODEC_TYPE]*mp4.Track)
		muxer := mp4.NewMuxer(mp4.FLAG_FRAGMENT)
		for _, track := range demuxer.Tracks {
			t := muxer.AddTrack(track.Cid)
			t.ICodecCtx = track.ICodecCtx
			trackMap[track.Cid] = t
		}
		moov := muxer.MakeMoov()
		var parts []*ContentPart
		var part *ContentPart
		for track, sample := range demuxer.RangeSample {
			if part == nil {
				part = &ContentPart{
					file:  file,
					Start: sample.Offset,
				}
				parts = append(parts, part)
			}
			fixSample := *sample
			part.Seek(sample.Offset, io.SeekStart)
			fixSample.Buffers = net.Buffers{make([]byte, sample.Size)}
			part.Read(fixSample.Buffers[0])
			moof, mdat := muxer.CreateFlagment(trackMap[track.Cid], fixSample)
			if moof != nil {
				part.boxies = append(part.boxies, moof, mdat)
				part.Size += int(moof.Size() + mdat.Size())
			}
		}
		var children []box.IBox
		var totalSize uint64
		ftyp := muxer.CreateFTYPBox()
		children = append(children, ftyp, moov)
		totalSize += uint64(ftyp.Size() + moov.Size())
		for _, part := range parts {
			totalSize += uint64(part.Size)
			children = append(children, part.boxies...)
			part.Close()
		}
		w.Header().Set("Content-Length", fmt.Sprintf("%d", totalSize))
		_, err = box.WriteTo(w, children...)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}

// download 处理 MP4 文件下载请求
// 支持两种模式：
// 1. 单个文件下载：通过 id 参数指定特定的录制文件
// 2. 时间范围合并下载：根据时间范围合并多个录制文件
func (p *MP4Plugin) download(w http.ResponseWriter, r *http.Request) {
	// 检查数据库连接
	if p.DB == nil {
		http.Error(w, pkg.ErrNoDB.Error(), http.StatusInternalServerError)
		return
	}

	// 设置响应头为 MP4 视频格式
	w.Header().Set("Content-Type", "video/mp4")

	// 从路径中提取流路径，并检查是否为分片格式
	streamPath := r.PathValue("streamPath")
	var flag mp4.Flag
	if strings.HasSuffix(streamPath, ".fmp4") {
		// 分片 MP4 格式
		flag = mp4.FLAG_FRAGMENT
		streamPath = strings.TrimSuffix(streamPath, ".fmp4")
	} else {
		// 常规 MP4 格式
		streamPath = strings.TrimSuffix(streamPath, ".mp4")
	}

	query := r.URL.Query()
	var streams []m7s.RecordStream

	// 处理单个文件下载请求
	if id := query.Get("id"); id != "" {
		// 设置下载文件名
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_%s.mp4", streamPath, id))

		// 从数据库查询指定 ID 的录制记录
		p.DB.Find(&streams, "id=? AND stream_path=?", id, streamPath)
		if len(streams) == 0 {
			http.Error(w, "record not found", http.StatusNotFound)
			return
		}

		// 下载单个文件
		p.downloadSingleFile(&streams[0], flag, w, r)
		return
	}

	// 处理时间范围合并下载请求

	// 解析时间范围参数
	startTime, endTime, err := util.TimeRangeQueryParse(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	p.Info("download", "streamPath", streamPath, "start", startTime, "end", endTime)

	// 设置合并下载的文件名，包含时间范围
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_%s_%s.mp4", streamPath, startTime.Format("20060102150405"), endTime.Format("20060102150405")))

	// 构建查询条件，查找指定时间范围内的录制记录
	queryRecord := m7s.RecordStream{
		Type: "mp4",
	}
	p.DB.Where(&queryRecord).Find(&streams, "end_time>? AND start_time<? AND stream_path=?", startTime, endTime, streamPath)

	// 创建 MP4 混合器
	muxer := mp4.NewMuxer(flag)
	ftyp := muxer.CreateFTYPBox()
	n := ftyp.Size()
	muxer.CurrentOffset = int64(n)

	// 初始化变量
	var lastTs, tsOffset int64                               // 时间戳偏移量，用于合并多个文件时保持时间连续性
	var parts []*ContentPart                                 // 内容片段列表
	sampleOffset := muxer.CurrentOffset + mp4.BeforeMdatData // 样本数据偏移量
	mdatOffset := sampleOffset                               // 媒体数据偏移量
	var audioTrack, videoTrack *mp4.Track                    // 音频和视频轨道
	var file *os.File                                        // 当前处理的文件
	var moov box.IBox                                        // MOOV box，包含元数据
	streamCount := len(streams)                              // 流的总数

	// Track ExtraData history for each track
	// 轨道额外数据历史记录，用于处理编码参数变化的情况
	type TrackHistory struct {
		Track     *mp4.Track
		ExtraData []byte
	}
	var audioHistory, videoHistory []TrackHistory

	// 添加音频轨道的函数
	addAudioTrack := func(track *mp4.Track) {
		t := muxer.AddTrack(track.Cid)
		t.ICodecCtx = track.ICodecCtx
		// 如果之前有音频轨道，继承其样本列表
		if len(audioHistory) > 0 {
			t.Samplelist = audioHistory[len(audioHistory)-1].Track.Samplelist
		}
		audioTrack = t
		audioHistory = append(audioHistory, TrackHistory{Track: t, ExtraData: track.GetRecord()})
	}

	// 添加视频轨道的函数
	addVideoTrack := func(track *mp4.Track) {
		t := muxer.AddTrack(track.Cid)
		t.ICodecCtx = track.ICodecCtx
		// 如果之前有视频轨道，继承其样本列表
		if len(videoHistory) > 0 {
			t.Samplelist = videoHistory[len(videoHistory)-1].Track.Samplelist
		}
		videoTrack = t
		videoHistory = append(videoHistory, TrackHistory{Track: t, ExtraData: track.GetRecord()})
	}

	// 智能添加轨道的函数，处理编码参数变化
	addTrack := func(track *mp4.Track) {
		var lastAudioTrack, lastVideoTrack *TrackHistory
		if len(audioHistory) > 0 {
			lastAudioTrack = &audioHistory[len(audioHistory)-1]
		}
		if len(videoHistory) > 0 {
			lastVideoTrack = &videoHistory[len(videoHistory)-1]
		}

		trackExtraData := track.GetRecord()
		if track.Cid.IsAudio() {
			if lastAudioTrack == nil {
				// 首次添加音频轨道
				addAudioTrack(track)
			} else if !bytes.Equal(lastAudioTrack.ExtraData, trackExtraData) {
				// 音频编码参数发生变化，不再创建新轨道，直接重用最后一个轨道
				audioTrack = lastAudioTrack.Track
				audioTrack.Samplelist = lastAudioTrack.Track.Samplelist
			}
		} else if track.Cid.IsVideo() {
			if lastVideoTrack == nil {
				// 首次添加视频轨道
				addVideoTrack(track)
			} else if !bytes.Equal(lastVideoTrack.ExtraData, trackExtraData) {
				// 视频编码参数发生变化，不再创建新轨道，直接重用最后一个轨道
				videoTrack = lastVideoTrack.Track
				videoTrack.Samplelist = lastVideoTrack.Track.Samplelist
			}
		}
	}

	// 遍历处理每个录制文件
	for i, stream := range streams {
		tsOffset = lastTs // 设置时间戳偏移

		// 打开录制文件
		file, err = os.Open(stream.FilePath)
		if err != nil {
			return
		}
		p.Info("read", "file", file.Name())

		// 创建解复用器并解析文件
		demuxer := mp4.NewDemuxer(file)
		err = demuxer.Demux()
		if err != nil {
			return
		}

		//trackCount := len(demuxer.Tracks)

		// 处理轨道信息
		if i == 0 || flag == mp4.FLAG_FRAGMENT {
			// 第一个文件或分片模式，添加所有轨道
			for _, track := range demuxer.Tracks {
				addTrack(track)
			}
		}

		// 处理开始时间偏移（仅第一个文件）
		if i == 0 {
			startTimestamp := startTime.Sub(stream.StartTime).Milliseconds()
			if startTimestamp > 0 {
				// 如果请求的开始时间晚于文件开始时间，需要定位到指定时间点
				var startSample *box.Sample
				if startSample, err = demuxer.SeekTime(uint64(startTimestamp)); err != nil {
					continue
				}
				tsOffset = -int64(startSample.Timestamp)
			}
		}

		var part *ContentPart

		// 遍历处理每个样本
		for track, sample := range demuxer.RangeSample {
			// 检查是否超出结束时间（仅最后一个文件）
			if i == streamCount-1 && int64(sample.Timestamp) > endTime.Sub(stream.StartTime).Milliseconds() {
				break
			}

			// 创建内容片段
			if part == nil {
				part = &ContentPart{
					file:  file,
					Start: sample.Offset,
				}
			}

			// 计算调整后的时间戳
			lastTs = int64(sample.Timestamp + uint32(tsOffset))
			fixSample := *sample
			fixSample.Timestamp += uint32(tsOffset)

			if flag == 0 {
				// 常规 MP4 模式
				fixSample.Offset = sampleOffset + (fixSample.Offset - part.Start)
				part.Size += sample.Size

				// 将样本添加到对应的轨道
				if track.Cid.IsAudio() {
					audioTrack.AddSampleEntry(fixSample)
				} else if track.Cid.IsVideo() {
					videoTrack.AddSampleEntry(fixSample)
				}
			} else {
				// 分片 MP4 模式
				// 读取样本数据
				part.Seek(sample.Offset, io.SeekStart)
				fixSample.Buffers = net.Buffers{make([]byte, sample.Size)}
				part.Read(fixSample.Buffers[0])

				// 创建分片
				var moof, mdat box.IBox
				if track.Cid.IsAudio() {
					moof, mdat = muxer.CreateFlagment(audioTrack, fixSample)
				} else if track.Cid.IsVideo() {
					moof, mdat = muxer.CreateFlagment(videoTrack, fixSample)
				}

				// 添加分片到内容片段
				if moof != nil {
					part.boxies = append(part.boxies, moof, mdat)
					part.Size += int(moof.Size() + mdat.Size())
				}
			}
		}

		// 更新偏移量并添加到片段列表
		if part != nil {
			sampleOffset += int64(part.Size)
			parts = append(parts, part)
		}
	}

	if flag == 0 {
		// 常规 MP4 模式：生成完整的 MP4 文件
		moovSize := muxer.MakeMoov().Size()
		dataSize := uint64(sampleOffset - mdatOffset)

		// 设置内容长度
		w.Header().Set("Content-Length", fmt.Sprintf("%d", uint64(sampleOffset)+moovSize))

		// 调整样本偏移量以适应 MOOV box
		for _, track := range muxer.Tracks {
			for i := range track.Samplelist {
				track.Samplelist[i].Offset += int64(moovSize)
			}
		}

		// 创建 MDAT box
		mdatBox := box.CreateBaseBox(box.TypeMDAT, dataSize+box.BasicBoxLen)

		var freeBox *box.FreeBox
		if mdatBox.HeaderSize() == box.BasicBoxLen {
			freeBox = box.CreateFreeBox(nil)
		}

		var written, totalWritten int64

		// 写入文件头部（FTYP、MOOV、FREE、MDAT header）
		totalWritten, err = box.WriteTo(w, ftyp, muxer.MakeMoov(), freeBox, mdatBox)
		if err != nil {
			return
		}

		// 写入所有内容片段的数据
		for _, part := range parts {
			part.Seek(part.Start, io.SeekStart)
			written, err = io.CopyN(w, part, int64(part.Size))
			if err != nil {
				return
			}
			totalWritten += written
			part.Close()
		}
	} else {

		// 检查轨道数量是否发生变化
		//if trackCount != len(muxer.Tracks) {
		if flag == mp4.FLAG_FRAGMENT {
			// 分片模式下重新生成 MOOV box
			moov = muxer.MakeMoov()
		}
		//}

		// 分片 MP4 模式：输出分片格式
		var children []box.IBox
		var totalSize uint64

		// 添加文件头和所有分片
		children = append(children, ftyp, moov)
		totalSize += uint64(ftyp.Size() + moov.Size())

		for _, part := range parts {
			totalSize += uint64(part.Size)
			children = append(children, part.boxies...)
			part.Close()
		}

		// 设置内容长度并写入数据
		w.Header().Set("Content-Length", fmt.Sprintf("%d", totalSize))
		_, err = box.WriteTo(w, children...)
		if err != nil {
			return
		}
	}
}

func (p *MP4Plugin) StartRecord(ctx context.Context, req *mp4pb.ReqStartRecord) (res *mp4pb.ResponseStartRecord, err error) {
	var recordExists bool
	var filePath = "."
	var fragment = time.Minute
	if req.Fragment != nil {
		fragment = req.Fragment.AsDuration()
	}
	if req.FilePath != "" {
		filePath = req.FilePath
	}
	res = &mp4pb.ResponseStartRecord{}
	_, recordExists = p.Server.Records.Find(func(job *m7s.RecordJob) bool {
		return job.StreamPath == req.StreamPath && job.RecConf.FilePath == req.FilePath
	})
	if recordExists {
		err = pkg.ErrRecordExists
		return
	}

	recordConf := config.Record{
		Append:   false,
		Fragment: fragment,
		FilePath: filePath,
	}
	var stream *m7s.Publisher
	var ok bool
	if stream, ok = p.Server.Streams.SafeGet(req.StreamPath); !ok {
		var sub *m7s.Subscriber
		sub, err = p.Subscribe(ctx, req.StreamPath)
		if err != nil || sub == nil {
			err = pkg.ErrNotFound
			return
		}
		defer sub.Stop(task.ErrAutoStop)
		if stream, ok = p.Server.Streams.SafeGet(req.StreamPath); !ok {
			err = pkg.ErrNotFound
			return
		}
	}
	job := p.Record(stream, recordConf, nil)
	res.Data = uint64(job.GetTaskPointer())
	err = job.WaitStarted()
	return
}

func (p *MP4Plugin) StopRecord(ctx context.Context, req *mp4pb.ReqStopRecord) (res *mp4pb.ResponseStopRecord, err error) {
	res = &mp4pb.ResponseStopRecord{}
	var recordJob *m7s.RecordJob
	recordJob, _ = p.Server.Records.Find(func(job *m7s.RecordJob) bool {
		return job.StreamPath == req.StreamPath
	})
	if recordJob != nil {
		t := recordJob.GetTask()
		if t != nil {
			res.Data = uint64(uintptr(unsafe.Pointer(t)))
			t.Stop(task.ErrStopByUser)
		}
	}
	return
}

func (p *MP4Plugin) EventStart(ctx context.Context, req *mp4pb.ReqEventRecord) (res *mp4pb.ResponseEventRecord, err error) {
	beforeDuration := p.BeforeDuration
	afterDuration := p.AfterDuration
	res = &mp4pb.ResponseEventRecord{}
	if req.BeforeDuration != "" {
		beforeDuration, err = time.ParseDuration(req.BeforeDuration)
		if err != nil {
			p.Error("EventStart", "error", err)
		}
	}
	if req.AfterDuration != "" {
		afterDuration, err = time.ParseDuration(req.AfterDuration)
		if err != nil {
			p.Error("EventStart", "error", err)
		}
	}
	//recorder := p.Meta.Recorder(config.Record{})
	var tmpJob *m7s.RecordJob
	tmpJob, _ = p.Server.Records.Find(func(job *m7s.RecordJob) bool {
		return job.StreamPath == req.StreamPath
	})
	if tmpJob == nil { //为空表示没有正在进行的录制，也就是没有自动录像，则进行正常的事件录像
		if stream, ok := p.Server.Streams.SafeGet(req.StreamPath); ok {
			recordConf := config.Record{
				Append:   false,
				Fragment: 0,
				FilePath: filepath.Join(p.EventRecordFilePath, stream.StreamPath, time.Now().Local().Format("2006-01-02-15-04-05")),
				Mode:     config.RecordModeEvent,
				Event: &config.RecordEvent{
					EventId:        req.EventId,
					EventLevel:     req.EventLevel,
					EventName:      req.EventName,
					EventDesc:      req.EventDesc,
					BeforeDuration: uint32(beforeDuration / time.Millisecond),
					AfterDuration:  uint32(afterDuration / time.Millisecond),
				},
			}
			//recordJob := recorder.GetRecordJob()
			var subconfig config.Subscribe
			defaults.SetDefaults(&subconfig)
			subconfig.BufferTime = beforeDuration
			p.Record(stream, recordConf, &subconfig)
		}
	} else {
		if tmpJob.Event != nil { //当前有事件录像正在录制，则更新该录像的结束时间
			tmpJob.Event.AfterDuration = tmpJob.Subscriber.VideoReader.AbsTime + uint32(afterDuration/time.Millisecond)
			if p.DB != nil {
				p.DB.Save(&tmpJob.Event)
			}
		} else { //当前有自动录像正在录制，则生成事件录像的记录，而不去生成事件录像的文件
			newEvent := &config.RecordEvent{
				EventId:        req.EventId,
				EventLevel:     req.EventLevel,
				EventName:      req.EventName,
				EventDesc:      req.EventDesc,
				BeforeDuration: uint32(beforeDuration / time.Millisecond),
				AfterDuration:  uint32(afterDuration / time.Millisecond),
			}
			if p.DB != nil {
				// Calculate total duration as the sum of BeforeDuration and AfterDuration
				totalDuration := newEvent.BeforeDuration + newEvent.AfterDuration

				// Calculate StartTime and EndTime based on current time and durations
				now := time.Now()
				startTime := now.Add(-time.Duration(newEvent.BeforeDuration) * time.Millisecond)
				endTime := now.Add(time.Duration(newEvent.AfterDuration) * time.Millisecond)

				p.DB.Save(&m7s.EventRecordStream{
					RecordEvent: newEvent,
					RecordStream: m7s.RecordStream{
						StreamPath: req.StreamPath,
						Duration:   totalDuration,
						StartTime:  startTime,
						EndTime:    endTime,
						Type:       "mp4",
					},
				})
			}
		}
	}
	return res, err
}

func (p *MP4Plugin) List(ctx context.Context, req *mp4pb.ReqRecordList) (resp *pb.RecordResponseList, err error) {
	globalReq := &pb.ReqRecordList{
		StreamPath: req.StreamPath,
		Range:      req.Range,
		Start:      req.Start,
		End:        req.End,
		PageNum:    req.PageNum,
		PageSize:   req.PageSize,
		Type:       "mp4",
		EventLevel: req.EventLevel,
	}
	return p.Server.GetRecordList(ctx, globalReq)
}

func (p *MP4Plugin) Catalog(ctx context.Context, req *emptypb.Empty) (resp *pb.ResponseCatalog, err error) {
	return p.Server.GetRecordCatalog(ctx, &pb.ReqRecordCatalog{Type: "mp4"})
}

func (p *MP4Plugin) Delete(ctx context.Context, req *mp4pb.ReqRecordDelete) (resp *pb.ResponseDelete, err error) {
	globalReq := &pb.ReqRecordDelete{
		StreamPath: req.StreamPath,
		Ids:        req.Ids,
		StartTime:  req.StartTime,
		EndTime:    req.EndTime,
		Range:      req.Range,
		Type:       "mp4",
	}
	return p.Server.DeleteRecord(ctx, globalReq)
}

// CreateTag 创建标签
func (p *MP4Plugin) CreateTag(ctx context.Context, req *mp4pb.ReqCreateTag) (res *mp4pb.ResponseTag, err error) {
	res = &mp4pb.ResponseTag{}

	// 检查数据库连接
	if p.DB == nil {
		res.Code = 500
		res.Message = pkg.ErrNoDB.Error()
		return res, pkg.ErrNoDB
	}

	// 解析标签时间
	tagTime, err := util.TimeQueryParse(req.TagTime)
	if err != nil {
		res.Code = 400
		res.Message = "标签时间格式错误: " + err.Error()
		return res, err
	}

	// 创建标签记录
	tag := &mp4.TagModel{
		TagName:    req.TagName,
		StreamPath: req.StreamPath,
		TagTime:    tagTime,
	}

	// 保存到数据库
	if err = p.DB.Create(tag).Error; err != nil {
		res.Code = 500
		res.Message = "创建标签失败: " + err.Error()
		return res, err
	}

	// 返回成功结果
	res.Code = 0
	res.Message = "创建成功"
	res.Data = &mp4pb.TagInfo{
		Id:         uint32(tag.ID),
		TagName:    tag.TagName,
		StreamPath: tag.StreamPath,
		TagTime:    tag.TagTime.Format(time.RFC3339),
		CreatedAt:  tag.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  tag.UpdatedAt.Format(time.RFC3339),
	}

	return res, nil
}

// UpdateTag 更新标签
func (p *MP4Plugin) UpdateTag(ctx context.Context, req *mp4pb.ReqUpdateTag) (res *mp4pb.ResponseTag, err error) {
	res = &mp4pb.ResponseTag{}

	// 检查数据库连接
	if p.DB == nil {
		res.Code = 500
		res.Message = pkg.ErrNoDB.Error()
		return res, pkg.ErrNoDB
	}

	// 查询标签是否存在
	var tag mp4.TagModel
	if err = p.DB.First(&tag, req.Id).Error; err != nil {
		res.Code = 404
		res.Message = "标签不存在: " + err.Error()
		return res, err
	}

	// 更新字段
	if req.TagName != "" {
		tag.TagName = req.TagName
	}
	if req.StreamPath != "" {
		tag.StreamPath = req.StreamPath
	}
	if req.TagTime != "" {
		tagTime, err := util.TimeQueryParse(req.TagTime)
		if err != nil {
			res.Code = 400
			res.Message = "标签时间格式错误: " + err.Error()
			return res, err
		}
		tag.TagTime = tagTime
	}

	// 保存更新
	if err = p.DB.Save(&tag).Error; err != nil {
		res.Code = 500
		res.Message = "更新标签失败: " + err.Error()
		return res, err
	}

	// 返回成功结果
	res.Code = 0
	res.Message = "更新成功"
	res.Data = &mp4pb.TagInfo{
		Id:         uint32(tag.ID),
		TagName:    tag.TagName,
		StreamPath: tag.StreamPath,
		TagTime:    tag.TagTime.Format(time.RFC3339),
		CreatedAt:  tag.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  tag.UpdatedAt.Format(time.RFC3339),
	}

	return res, nil
}

// DeleteTag 删除标签（软删除）
func (p *MP4Plugin) DeleteTag(ctx context.Context, req *mp4pb.ReqDeleteTag) (res *mp4pb.ResponseTag, err error) {
	res = &mp4pb.ResponseTag{}

	// 检查数据库连接
	if p.DB == nil {
		res.Code = 500
		res.Message = pkg.ErrNoDB.Error()
		return res, pkg.ErrNoDB
	}

	// 软删除标签
	if err = p.DB.Delete(&mp4.TagModel{}, req.Id).Error; err != nil {
		res.Code = 500
		res.Message = "删除标签失败: " + err.Error()
		return res, err
	}

	// 返回成功结果
	res.Code = 0
	res.Message = "删除成功"

	return res, nil
}

// ListTag 查询标签列表
func (p *MP4Plugin) ListTag(ctx context.Context, req *mp4pb.ReqListTag) (res *mp4pb.ResponseTagList, err error) {
	res = &mp4pb.ResponseTagList{}

	// 检查数据库连接
	if p.DB == nil {
		res.Code = 500
		res.Message = pkg.ErrNoDB.Error()
		return res, pkg.ErrNoDB
	}

	// 构建查询
	query := p.DB.Model(&mp4.TagModel{})

	// 流路径过滤（默认模糊匹配）
	if req.StreamPath != "" {
		if strings.Contains(req.StreamPath, "*") {
			query = query.Where("stream_path LIKE ?", strings.ReplaceAll(req.StreamPath, "*", "%"))
		} else {
			query = query.Where("stream_path LIKE ?", "%"+req.StreamPath+"%")
		}
	}

	// 标签名称过滤（默认模糊匹配）
	if req.TagName != "" {
		if strings.Contains(req.TagName, "*") {
			query = query.Where("tag_name LIKE ?", strings.ReplaceAll(req.TagName, "*", "%"))
		} else {
			query = query.Where("tag_name LIKE ?", "%"+req.TagName+"%")
		}
	}

	// 时间范围过滤（只有当传入了时间参数时才进行过滤）
	if req.Start != "" {
		startTime, err := util.TimeQueryParse(req.Start)
		if err == nil && !startTime.IsZero() {
			query = query.Where("tag_time >= ?", startTime)
		}
	}
	if req.End != "" {
		endTime, err := util.TimeQueryParse(req.End)
		if err == nil && !endTime.IsZero() {
			query = query.Where("tag_time <= ?", endTime)
		}
	}

	// 分页
	page := req.Page
	count := req.Count
	if page < 1 {
		page = 1
	}
	if count < 1 {
		count = 10
	}
	offset := (page - 1) * count

	// 获取总数
	var total int64
	if err = query.Count(&total).Error; err != nil {
		res.Code = 500
		res.Message = "查询总数失败: " + err.Error()
		return res, err
	}

	// 查询数据
	var tags []mp4.TagModel
	if err = query.Order("tag_time DESC").Offset(int(offset)).Limit(int(count)).Find(&tags).Error; err != nil {
		res.Code = 500
		res.Message = "查询标签失败: " + err.Error()
		return res, err
	}

	// 转换为响应格式
	res.Code = 0
	res.Message = "查询成功"
	res.Total = uint32(total)
	res.List = make([]*mp4pb.TagInfo, 0, len(tags))

	for _, tag := range tags {
		res.List = append(res.List, &mp4pb.TagInfo{
			Id:         uint32(tag.ID),
			TagName:    tag.TagName,
			StreamPath: tag.StreamPath,
			TagTime:    tag.TagTime.Format(time.RFC3339),
			CreatedAt:  tag.CreatedAt.Format(time.RFC3339),
			UpdatedAt:  tag.UpdatedAt.Format(time.RFC3339),
		})
	}

	return res, nil
}
