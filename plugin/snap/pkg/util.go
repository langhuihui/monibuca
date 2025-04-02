package snap

import (
	"fmt"
	"io"
	"os/exec"

	m7s "m7s.live/v5"
	"m7s.live/v5/pkg"
)

// GetVideoFrame 获取视频帧数据
func GetVideoFrame(streamPath string, server *m7s.Server) ([]*pkg.AnnexB, *pkg.AVTrack, error) {
	// 获取发布者
	publisher, err := server.GetPublisher(streamPath)
	if err != nil {
		return nil, nil, err
	}

	if publisher.VideoTrack.AVTrack == nil {
		return nil, nil, pkg.ErrNotFound
	}

	// 等待视频就绪
	if err = publisher.VideoTrack.WaitReady(); err != nil {
		return nil, nil, err
	}

	// 创建读取器并等待 I 帧
	reader := pkg.NewAVRingReader(publisher.VideoTrack.AVTrack, "snapshot")
	if err = reader.StartRead(publisher.VideoTrack.GetIDR()); err != nil {
		return nil, nil, err
	}
	defer reader.StopRead()
	var track pkg.AVTrack
	var annexb pkg.AnnexB
	track.ICodecCtx, track.SequenceFrame, err = annexb.ConvertCtx(publisher.VideoTrack.ICodecCtx)
	if err != nil {
		return nil, nil, err
	}
	if track.ICodecCtx == nil {
		return nil, nil, pkg.ErrUnsupportCodec
	}
	var annexbList []*pkg.AnnexB

	for lastFrameSequence := publisher.VideoTrack.AVTrack.LastValue.Sequence; reader.Value.Sequence <= lastFrameSequence; reader.ReadNext() {
		if reader.Value.Raw == nil {
			if err := reader.Value.Demux(publisher.VideoTrack.ICodecCtx); err != nil {
				return nil, nil, err
			}
		}
		var annexb pkg.AnnexB
		annexb.Mux(track.ICodecCtx, &reader.Value)
		annexbList = append(annexbList, &annexb)
	}
	return annexbList, &track, nil
}

// ProcessWithFFmpeg 使用 FFmpeg 处理视频帧并生成截图
func ProcessWithFFmpeg(annexb []*pkg.AnnexB, output io.Writer) error {
	// 创建ffmpeg命令，使用select过滤器选择最后一帧
	cmd := exec.Command("ffmpeg", "-hide_banner", "-i", "pipe:0", "-vf", fmt.Sprintf("select='eq(n,%d)'", len(annexb)-1), "-vframes", "1", "-f", "mjpeg", "pipe:1")

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
	for _, annex := range annexb {
		if _, err = annex.WriteTo(stdin); err != nil {
			stdin.Close()
			return err
		}
	}
	stdin.Close()

	// 从ffmpeg的stdout读取图片数据并写入到输出
	if _, err = io.Copy(output, stdout); err != nil {
		return err
	}

	// 等待ffmpeg进程结束
	return cmd.Wait()
}
