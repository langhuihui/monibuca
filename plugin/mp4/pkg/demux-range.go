package mp4

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"m7s.live/v5/pkg/storage"

	"m7s.live/v5"
	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/codec"
	"m7s.live/v5/plugin/mp4/pkg/box"
)

type DemuxerRange struct {
	*slog.Logger
	StartTime, EndTime     time.Time
	Streams                []m7s.RecordStream
	AudioCodec, VideoCodec codec.ICodecCtx
	OnAudio, OnVideo       func(box.Sample) error
	OnCodec                func(codec.ICodecCtx, codec.ICodecCtx)
	storage                storage.Storage
}

func (d *DemuxerRange) Demux(ctx context.Context) error {
	var ts, tsOffset int64
	var audioInitialized, videoInitialized bool
	st := d.storage
	var globalStorageType string
	var file storage.File
	var err error
	if st != nil {
		globalStorageType = st.GetKey()
	}
	for _, stream := range d.Streams {
		// 检查流的时间范围是否在指定范围内
		if stream.EndTime.Before(d.StartTime) || stream.StartTime.After(d.EndTime) {
			continue
		}
		// 如果是 HTTP/HTTPS URL，下载到临时文件
		if strings.HasPrefix(stream.FilePath, "http://") || strings.HasPrefix(stream.FilePath, "https://") {
			resp, err := http.Get(stream.FilePath)
			if err != nil {
				d.Error("failed to download file from URL", "err", err, "url", stream.FilePath)
				continue
			}
			defer resp.Body.Close()

			// 创建临时文件
			tmpFile, err := os.CreateTemp("", "mp4-*.tmp")
			if err != nil {
				d.Error("failed to create temp file", "err", err)
				continue
			}
			tmpPath := tmpFile.Name()

			// 复制内容到临时文件
			_, err = io.Copy(tmpFile, resp.Body)
			tmpFile.Close()
			if err != nil {
				os.Remove(tmpPath)
				d.Error("failed to save downloaded file", "err", err)
				continue
			}

			// 打开临时文件
			file, err = os.Open(tmpPath)
			if err != nil {
				os.Remove(tmpPath)
				d.Error("failed to open downloaded file", "err", err)
				continue
			}
			// 延迟关闭和删除临时文件
			defer func() {
				if file != nil {
					file.Close()
				}
				os.Remove(tmpPath)
			}()
			d.Info("reading downloaded file from URL", "url", stream.FilePath)
		} else if filepath.IsAbs(stream.FilePath) {
			file, err = os.Open(stream.FilePath)
			if err != nil {
				continue
			}
		} else {
			useGlobalStorage := st != nil && globalStorageType == stream.StorageType
			isLocalStorage := stream.StorageType == string(storage.StorageTypeLocal) || stream.StorageType == ""
			if useGlobalStorage {
				if isLocalStorage {
					if localStorage, ok := st.(*storage.LocalStorage); ok {
						fullPath := localStorage.GetFullPath(stream.FilePath, stream.StorageLevel)
						file, err = os.Open(fullPath)
						if err != nil {
							continue
						}
					} else {
						// 类型不匹配，使用 OpenFile 作为兜底
						file, err = st.OpenFile(ctx, stream.FilePath)
						if err != nil {
							continue
						}
					}
				} else {
					filePath, err := st.GetURL(ctx, stream.FilePath)
					if err != nil || filePath == "" {
						continue
					}
					file, err = st.OpenFile(ctx, filePath)
					if err != nil {
						continue
					}
				}
			} else {
				file, err = os.Open(stream.FilePath)
				if err != nil {
					continue
				}
			}
		}

		// 保存上一个文件的最后时间戳，用于跨文件连续
		baseOffset := ts
		//file, err := os.Open(stream.FilePath)
		//if err != nil {
		//	continue
		//}
		defer file.Close()

		demuxer := NewDemuxer(file)
		if err = demuxer.Demux(); err != nil {
			return err
		}

		// 处理每个轨道的额外数据 (序列头)，并检查是否需要初始化
		var newAudio, newVideo codec.ICodecCtx
		for _, track := range demuxer.Tracks {
			if track.Cid.IsAudio() {
				d.AudioCodec = track.ICodecCtx
				if !audioInitialized {
					newAudio = track.ICodecCtx
					audioInitialized = true
				}
			} else {
				d.VideoCodec = track.ICodecCtx
				if !videoInitialized {
					newVideo = track.ICodecCtx
					videoInitialized = true
				}
			}
		}
		// 只对新发现的音频或视频调用 OnCodec
		if (newAudio != nil || newVideo != nil) && d.OnCodec != nil {
			d.OnCodec(newAudio, newVideo)
		}

		// 计算起始时间戳偏移（用于 Seek）
		var seekOffset int64
		if !d.StartTime.IsZero() {
			startTimestamp := d.StartTime.Sub(stream.StartTime).Milliseconds()
			if startTimestamp < 0 {
				startTimestamp = 0
			}
			if startSample, err := demuxer.SeekTime(uint64(startTimestamp)); err == nil {
				seekOffset = -int64(startSample.Timestamp)
			}
		}
		// 合并偏移：跨文件连续偏移 + Seek 偏移
		tsOffset = baseOffset + seekOffset

		// 读取和处理样本
		for track, sample := range demuxer.ReadSample {
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			// 检查是否超出结束时间
			sampleTime := stream.StartTime.Add(time.Duration(sample.Timestamp) * time.Millisecond)
			if !d.EndTime.IsZero() && sampleTime.After(d.EndTime) {
				break
			}

			// 计算样本数据偏移和读取数据
			sampleOffset := int(sample.Offset) - int(demuxer.mdatOffset)
			if sampleOffset < 0 || sampleOffset+sample.Size > len(demuxer.mdat.Data) {
				continue
			}
			data := demuxer.mdat.Data[sampleOffset : sampleOffset+sample.Size]
			sample.Buffers = net.Buffers{data}

			// 计算时间戳
			if int64(sample.Timestamp)+tsOffset < 0 {
				ts = 0
			} else {
				ts = int64(sample.Timestamp) + tsOffset
			}
			sample.Timestamp = uint32(ts)
			if track.Cid.IsAudio() {
				if err := d.OnAudio(sample); err != nil {
					return err
				}
			} else {
				if err := d.OnVideo(sample); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

type DemuxerConverterRange[TA pkg.IAVFrame, TV pkg.IAVFrame] struct {
	DemuxerRange
	OnAudio func(TA) error
	OnVideo func(TV) error
}

func (d *DemuxerConverterRange[TA, TV]) Demux(ctx context.Context) error {
	var targetAudio TA
	var targetVideo TV

	targetAudioType, targetVideoType := reflect.TypeOf(targetAudio).Elem(), reflect.TypeOf(targetVideo).Elem()
	d.DemuxerRange.OnAudio = func(audio box.Sample) error {
		targetAudio = reflect.New(targetAudioType).Interface().(TA) // TODO: reuse
		var audioFrame AudioFrame
		audioFrame.ICodecCtx = d.AudioCodec
		audioFrame.BaseSample = &pkg.BaseSample{}
		audioFrame.Raw = &audio.Memory
		audioFrame.SetTS32(audio.Timestamp)
		err := pkg.ConvertFrameType(&audioFrame, targetAudio)
		if err == nil {
			err = d.OnAudio(targetAudio)
		}
		return err
	}
	d.DemuxerRange.OnVideo = func(video box.Sample) error {
		targetVideo = reflect.New(targetVideoType).Interface().(TV) // TODO: reuse
		var videoFrame VideoFrame
		videoFrame.ICodecCtx = d.VideoCodec
		videoFrame.BaseSample = &pkg.BaseSample{}
		videoFrame.Raw = &video.Memory
		videoFrame.SetTS32(video.Timestamp)
		videoFrame.IDR = video.KeyFrame
		videoFrame.CTS = time.Duration(video.CTS) / time.Millisecond
		err := pkg.ConvertFrameType(&videoFrame, targetVideo)
		if err == nil {
			err = d.OnVideo(targetVideo)
		}
		return err
	}
	return d.DemuxerRange.Demux(ctx)
}
