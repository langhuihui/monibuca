package snap

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"m7s.live/v5/pkg"

	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/task"
)

// 获取视频帧数据
func getVideoFrame(streamPath string, server *m7s.Server) (pkg.AnnexB, *pkg.AVTrack, error) {
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

// 使用 FFmpeg 处理视频帧并生成截图
func processWithFFmpeg(annexb pkg.AnnexB, output io.Writer) error {
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
func saveSnapshot(annexb pkg.AnnexB, savePath string) error {
	var buf bytes.Buffer
	if err := processWithFFmpeg(annexb, &buf); err != nil {
		return fmt.Errorf("process with ffmpeg error: %w", err)
	}

	// 如果配置了水印，添加水印
	if GlobalWatermarkConfig.Text != "" {
		imgData, err := AddWatermark(buf.Bytes(), GlobalWatermarkConfig)
		if err != nil {
			return fmt.Errorf("add watermark error: %w", err)
		}
		return os.WriteFile(savePath, imgData, 0644)
	}

	return os.WriteFile(savePath, buf.Bytes(), 0644)
}

func NewTransform() m7s.ITransformer {
	ret := &Transformer{
		snapChan: make(chan struct{}, 1),
	}
	ret.SetDescription(task.OwnerTypeKey, "Snap")
	return ret
}

type Transformer struct {
	m7s.DefaultTransformer
	ffmpeg       *exec.Cmd
	snapChan     chan struct{}
	snapInterval time.Duration
	savePath     string
}

func (t *Transformer) TriggerSnap() {
	select {
	case t.snapChan <- struct{}{}:
	default:
		// 如果通道已满，移除旧的请求
		<-t.snapChan
		t.snapChan <- struct{}{}
	}
}

func (t *Transformer) Start() (err error) {
	// 获取配置
	t.snapInterval = t.TransformJob.Plugin.Config.Get("SnapInterval").GetValue().(time.Duration)
	t.savePath = t.TransformJob.Plugin.Config.Get("SnapSavePath").GetValue().(string)
	t.Info("savpath", t.savePath)

	// 检查保存路径
	if err := os.MkdirAll(t.savePath, 0755); err != nil {
		return fmt.Errorf("create save path failed: %w", err)
	}

	if t.snapInterval != 0 {
		t.Info("snap interval is set, skipping subscriber mode",
			"interval", t.snapInterval,
			"save_path", t.savePath,
		)
		return nil
	}

	// 订阅流
	if err = t.TransformJob.Subscribe(); err != nil {
		return fmt.Errorf("subscribe stream failed: %w", err)
	}

	t.Info("snap transformer started",
		"stream", t.TransformJob.StreamPath,
		"save_path", t.savePath,
	)
	return nil
}

func (t *Transformer) Run() (err error) {
	// 等待截图触发信号
	<-t.snapChan

	// 获取视频帧
	annexb, _, err := getVideoFrame(t.TransformJob.StreamPath, t.TransformJob.Plugin.Server)
	if err != nil {
		return err
	}

	// 处理视频帧并生成截图
	return processWithFFmpeg(annexb, t.TransformJob.Config.Output[0].Conf.(io.Writer))
}

func (t *Transformer) Go() error {
	// 1. 通过 TransformJob 获取 Subscriber
	subscriber := t.TransformJob.Subscriber

	// 2. 设置数据处理回调
	handleVideo := func(video *pkg.AVFrame) error {
		// 处理视频数据
		if video.IDR {
			t.Debug("received IDR frame",
				"stream", subscriber.StreamPath,
				"timestamp", video.Timestamp,
			)

			// 获取视频帧
			annexb, _, err := getVideoFrame(subscriber.StreamPath, t.TransformJob.Plugin.Server)
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
			filename := fmt.Sprintf("%s_%s.jpg", subscriber.StreamPath, time.Now().Format("20060102150405"))
			filename = strings.ReplaceAll(filename, "/", "_")
			savePath := filepath.Join(t.savePath, filename)

			// 保存截图（带水印）
			if err := saveSnapshot(annexb, savePath); err != nil {
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
				)
			}
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
	)
	go m7s.PlayBlock(subscriber, handleAudio, handleVideo)
	return nil
}

func (t *Transformer) Dispose() {
	t.Info("disposing snap transformer",
		"stream", t.TransformJob.StreamPath,
	)

	// 关闭截图触发通道
	if t.snapChan != nil {
		close(t.snapChan)
		t.snapChan = nil
	}

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
