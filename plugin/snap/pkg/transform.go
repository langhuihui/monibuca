package snap

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"m7s.live/v5/pkg"

	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/task"
)

const (
	SnapModeTimeInterval = iota
	SnapModeIFrameInterval
	SnapModeManual
)

// GetVideoFrame 获取视频帧数据
func GetVideoFrame(streamPath string, server *m7s.Server) (pkg.AnnexB, *pkg.AVTrack, error) {
	// 获取发布者
	publisher, ok := server.Streams.Get(streamPath)
	if !ok || publisher.VideoTrack.AVTrack == nil {
		return pkg.AnnexB{}, nil, pkg.ErrNotFound
	}

	// 等待视频就绪
	if err := publisher.VideoTrack.WaitReady(); err != nil {
		return pkg.AnnexB{}, nil, err
	}

	// 创建读取器并等待 I 帧
	reader := pkg.NewAVRingReader(publisher.VideoTrack.AVTrack, "Origin")
	if err := reader.StartRead(publisher.VideoTrack.GetIDR()); err != nil {
		return pkg.AnnexB{}, nil, err
	}
	defer reader.StopRead()

	if reader.Value.Raw == nil {
		if err := reader.Value.Demux(publisher.VideoTrack.ICodecCtx); err != nil {
			return pkg.AnnexB{}, nil, err
		}
	}

	var annexb pkg.AnnexB
	var track pkg.AVTrack

	var err error
	track.ICodecCtx, track.SequenceFrame, err = annexb.ConvertCtx(publisher.VideoTrack.ICodecCtx)
	if err != nil {
		return pkg.AnnexB{}, nil, err
	}
	if track.ICodecCtx == nil {
		return pkg.AnnexB{}, nil, fmt.Errorf("unsupported codec")
	}
	annexb.Mux(track.ICodecCtx, &reader.Value)

	return annexb, &track, nil
}

// ProcessWithFFmpeg 使用 FFmpeg 处理视频帧并生成截图
func ProcessWithFFmpeg(annexb pkg.AnnexB, output io.Writer) error {
	// 创建ffmpeg命令
	cmd := exec.Command("ffmpeg", "-hide_banner", "-i", "pipe:0", "-vframes", "1", "-f", "mjpeg", "pipe:1")

	// 获取输入和输出pipe
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	// 启动ffmpeg进程
	if err = cmd.Start(); err != nil {
		return err
	}

	// 将annexb数据写入到ffmpeg的stdin
	if _, err = annexb.WriteTo(stdin); err != nil {
		stdin.Close()
		return err
	}
	stdin.Close()

	// 从ffmpeg的stdout读取图片数据并写入到输出
	if _, err = io.Copy(output, stdout); err != nil {
		return err
	}

	// 等待ffmpeg进程结束
	return cmd.Wait()
}

// 保存截图到文件
func saveSnapshot(annexb pkg.AnnexB, savePath string, plugin *m7s.Plugin, streamPath string, snapMode int) error {
	var buf bytes.Buffer
	if err := ProcessWithFFmpeg(annexb, &buf); err != nil {
		return fmt.Errorf("process with ffmpeg error: %w", err)
	}

	// 如果配置了水印，添加水印
	if GlobalWatermarkConfig.Text != "" {
		imgData, err := AddWatermark(buf.Bytes(), GlobalWatermarkConfig)
		if err != nil {
			return fmt.Errorf("add watermark error: %w", err)
		}
		err = os.WriteFile(savePath, imgData, 0644)
		if err != nil {
			return err
		}
	} else {
		err := os.WriteFile(savePath, buf.Bytes(), 0644)
		if err != nil {
			return err
		}
	}

	// 保存记录到数据库
	if plugin != nil && plugin.DB != nil {
		record := SnapRecord{
			StreamName: streamPath,
			SnapMode:   snapMode,
			SnapTime:   time.Now(),
			SnapPath:   savePath,
		}
		if err := plugin.DB.Create(&record).Error; err != nil {
			return fmt.Errorf("save snapshot record failed: %w", err)
		}
	}

	return nil
}

var _ task.TaskGo = (*Transformer)(nil)

func NewTransform() m7s.ITransformer {
	ret := &Transformer{}
	ret.SetDescription(task.OwnerTypeKey, "Snap")
	return ret
}

type Transformer struct {
	m7s.DefaultTransformer
	ffmpeg            *exec.Cmd
	snapTimeInterval  time.Duration
	savePath          string
	filterRegex       *regexp.Regexp
	snapMode          int // 截图模式：0-时间间隔，1-帧间隔
	snapFrameCount    int // 当前帧计数
	snapFrameInterval int // 帧间隔
}

func (t *Transformer) Start() (err error) {
	// 获取配置，带默认值检查
	if t.TransformJob.Plugin.Config.Has("TimeInterval") {
		t.snapTimeInterval = t.TransformJob.Plugin.Config.Get("TimeInterval").GetValue().(time.Duration)
	} else {
		t.snapTimeInterval = time.Minute // 默认1分钟
	}

	if t.TransformJob.Plugin.Config.Has("SavePath") {
		t.savePath = t.TransformJob.Plugin.Config.Get("SavePath").GetValue().(string)
	} else {
		t.savePath = "snaps" // 默认保存路径
	}

	if t.TransformJob.Plugin.Config.Has("Mode") {
		t.snapMode = t.TransformJob.Plugin.Config.Get("Mode").GetValue().(int)
	} else {
		t.snapMode = SnapModeIFrameInterval // 默认使用关键帧模式
	}

	// 检查snapmode是否有效
	if t.snapMode != SnapModeIFrameInterval && t.snapMode != SnapModeTimeInterval {
		t.Debug("invalid snap mode, skip snapshot",
			"mode", t.snapMode,
		)
		return nil
	}

	if t.TransformJob.Plugin.Config.Has("IFrameInterval") {
		t.snapFrameInterval = t.TransformJob.Plugin.Config.Get("IFrameInterval").GetValue().(int)
	} else {
		t.snapFrameInterval = 3 // 默认每3个I帧截图一次
	}

	t.snapFrameCount = 0

	t.Info("snap transformer started",
		"stream", t.TransformJob.StreamPath,
		"save_path", t.savePath,
		"mode", t.snapMode,
		"frame_interval", t.snapFrameInterval,
	)

	// 获取过滤器配置
	if t.TransformJob.Plugin.Config.Has("Filter") {
		filterStr := t.TransformJob.Plugin.Config.Get("Filter").GetValue().(string)
		t.filterRegex = regexp.MustCompile(filterStr)
	}

	// 检查保存路径
	if err := os.MkdirAll(t.savePath, 0755); err != nil {
		return fmt.Errorf("create save path failed: %w", err)
	}

	// 如果是时间间隔模式且间隔时间不为0，则跳过订阅模式
	if t.snapMode == SnapModeTimeInterval && t.snapTimeInterval != 0 {
		t.Info("snap interval is set, skipping subscriber mode",
			"interval", t.snapTimeInterval,
			"save_path", t.savePath,
		)
		return nil
	}

	// 使用 TransformJob 的 Subscribe 方法
	return t.TransformJob.Subscribe()
}

func (t *Transformer) Go() error {
	// 检查snapmode是否有效
	if t.snapMode != SnapModeIFrameInterval && t.snapMode != SnapModeTimeInterval {
		t.Debug("invalid snap mode, skip snapshot",
			"mode", t.snapMode,
		)
		return nil
	}
	if t.snapMode == SnapModeTimeInterval && t.snapTimeInterval != 0 {
		t.Info("snap interval is set, skipping subscriber mode",
			"interval", t.snapTimeInterval,
			"save_path", t.savePath,
		)
		return nil
	}
	// 1. 通过 TransformJob 获取 Subscriber
	subscriber := t.TransformJob.Subscriber

	// 检查流名称是否匹配过滤器
	if t.filterRegex != nil && !t.filterRegex.MatchString(subscriber.StreamPath) {
		t.Info("stream path not match filter, skip",
			"stream", subscriber.StreamPath,
			"filter", t.filterRegex.String(),
		)
		return nil
	}

	// 2. 设置数据处理回调
	handleVideo := func(video *pkg.AVFrame) error {
		// 处理视频数据
		if !video.IDR {
			return nil
		}

		shouldSnap := false
		if t.snapMode == 0 { // 时间间隔模式
			shouldSnap = true
		} else { // 帧间隔模式
			t.snapFrameCount++
			if t.snapFrameCount >= t.snapFrameInterval {
				shouldSnap = true
				t.snapFrameCount = 0
			}
		}

		if !shouldSnap {
			return nil
		}

		t.Debug("received IDR frame",
			"stream", subscriber.StreamPath,
			"timestamp", video.Timestamp,
			"mode", t.snapMode,
			"frame_count", t.snapFrameCount,
		)

		// 获取视频帧
		annexb, _, err := GetVideoFrame(subscriber.StreamPath, t.TransformJob.Plugin.Server)
		if err != nil {
			t.Error("get video frame failed",
				"error", err.Error(),
				"stream", subscriber.StreamPath,
			)
			return nil
		}

		t.Debug("got video frame",
			"stream", subscriber.StreamPath,
			"timestamp", video.Timestamp,
		)

		// 生成文件名
		filename := fmt.Sprintf("%s_%s.jpg", subscriber.StreamPath, time.Now().Format("20060102150405.000"))
		filename = strings.ReplaceAll(filename, "/", "_")
		savePath := filepath.Join(t.savePath, filename)

		// 保存截图（带水印）
		if err := saveSnapshot(annexb, savePath, t.TransformJob.Plugin, subscriber.StreamPath, t.snapMode); err != nil {
			t.Error("save snapshot failed",
				"error", err.Error(),
				"stream", subscriber.StreamPath,
				"path", savePath,
			)
		} else {
			fileInfo, err := os.Stat(savePath)
			size := int64(0)
			if err == nil {
				size = fileInfo.Size()
			}
			t.Info("take snapshot success",
				"stream", subscriber.StreamPath,
				"path", savePath,
				"size", size,
				"watermark", GlobalWatermarkConfig.Text != "",
				"mode", t.snapMode,
			)
		}
		return nil
	}

	handleAudio := func(audio *pkg.AVFrame) error {
		// 处理音频数据
		return nil
	}

	// 3. 开始接收数据
	t.Info("starting stream processing",
		"stream", subscriber.StreamPath,
		"mode", t.snapMode,
		"frame_interval", t.snapFrameInterval,
	)
	return m7s.PlayBlock(subscriber, handleAudio, handleVideo)
}

func (t *Transformer) Dispose() {
	t.Info("disposing snap transformer",
		"stream", t.TransformJob.StreamPath,
	)

	// 清理 FFmpeg 进程
	if t.ffmpeg != nil {
		if err := t.ffmpeg.Process.Kill(); err != nil {
			t.Error("kill ffmpeg process failed",
				"error", err.Error(),
				"pid", t.ffmpeg.Process.Pid,
			)
		} else {
			t.Info("ffmpeg process killed",
				"pid", t.ffmpeg.Process.Pid,
			)
		}
		t.ffmpeg = nil
	}

	t.Info("snap transformer disposed",
		"stream", t.TransformJob.StreamPath,
	)
}
