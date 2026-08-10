package plugin_rtsp

import (
	"context"
	"math"

	task "github.com/langhuihui/gotask"
	"m7s.live/v5/plugin/rtsp/pb"
	. "m7s.live/v5/plugin/rtsp/pkg"
)

// isPowerOfTwoScale 校验倍速为 2^n（n 为整数，可为负）：0.25/0.5/1/2/4/8…
// Confirmed via 寸止(REQ-RTSP-002)
func isPowerOfTwoScale(speed float64) bool {
	if speed <= 0 || math.IsInf(speed, 0) || math.IsNaN(speed) {
		return false
	}
	exp := math.Log2(speed)
	return math.Abs(exp-math.Round(exp)) < 1e-9
}

// PlaybackSpeed 对运行中的 RTSP 拉流会话二次 PLAY 注入 Scale
func (p *RTSPPlugin) PlaybackSpeed(ctx context.Context, req *pb.PlaybackSpeedRequest) (*pb.BaseResponse, error) {
	resp := &pb.BaseResponse{}

	if req.StreamPath == "" {
		resp.Code = 400
		resp.Message = "流路径不能为空"
		return resp, nil
	}
	if !isPowerOfTwoScale(req.Speed) {
		resp.Code = 400
		resp.Message = "倍速必须为 2 的整数次幂（如 0.25、0.5、1、2、4、8）"
		return resp, nil
	}

	pullJob, ok := p.Server.Pulls.Get(req.StreamPath)
	if !ok {
		resp.Code = 404
		resp.Message = "未找到对应的拉流会话"
		return resp, nil
	}

	var client *Client
	pullJob.RangeSubTask(func(t task.ITask) bool {
		if c, ok := t.(*Client); ok {
			client = c
			return false
		}
		return true
	})
	if client == nil {
		resp.Code = 404
		resp.Message = "未找到对应的 RTSP 回放会话"
		return resp, nil
	}

	if err := client.PlayScale(req.Speed); err != nil {
		resp.Code = 500
		resp.Message = "发送倍速请求失败: " + err.Error()
		p.Error("rtsp playback speed failed", "streampath", req.StreamPath, "speed", req.Speed, "error", err)
		return resp, nil
	}

	// 同步本地 Publisher 时间戳缩放，与 GB28181 PlaybackSpeed 对齐
	if pub := client.GetPullJob().Publisher; pub != nil {
		pub.Speed = req.Speed
		pub.Scale = req.Speed
		pub.Info("set stream speed", "speed", req.Speed)
	}

	p.Info("rtsp playback speed", "streampath", req.StreamPath, "speed", req.Speed)
	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}
