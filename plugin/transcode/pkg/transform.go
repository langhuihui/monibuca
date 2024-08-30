package transcode

import (
	"fmt"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/config"
	"m7s.live/m7s/v5/pkg/task"
	"m7s.live/m7s/v5/pkg/util"
	flv "m7s.live/m7s/v5/plugin/flv/pkg"
	"net"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// / 定义传输模式的常量
const (
	TRANS_MODE_PIPE TransMode = "pipe"
	TRANS_MODE_RTSP TransMode = "rtsp"
	TRANS_MODE_RTMP TransMode = "rtmp"
	TRANS_MODE_LIB  TransMode = "lib"
)

type (
	TransMode    string
	DecodeConfig struct {
		Codec string `json:"codec" desc:"解码器"`
		Track string `json:"track" desc:"待解码的 track 名称"`
		Args  string `json:"args" desc:"解码参数"`
	}
	EncodeConfig struct {
		Codec string `json:"codec" desc:"编码器"`
		Track string `json:"track" desc:"待编码的 track 名称"`
		Args  string `json:"args" desc:"编码参数"`
		Dest  string `json:"dest" desc:"目标主机路径"`
	}
	TransRule struct {
		From      DecodeConfig   `json:"from"`
		To        []EncodeConfig `json:"to" desc:"编码配置"`            //目标
		Mode      TransMode      `json:"mode" desc:"转码模式"`          //转码模式
		LogToFile bool           `json:"logtofile" desc:"转码是否写入日志"` //转码日志写入文件
		PreStart  bool           `json:"prestart" desc:"是否预转码"`     //预转码
	}
)

func NewTransform() m7s.ITransformer {
	ret := &Transformer{}
	ret.WriteFlvTag = func(flv net.Buffers) (err error) {
		var buffer []byte
		for _, b := range flv {
			buffer = append(buffer, b...)
		}
		select {
		case ret.rBuf <- buffer:
		default:
			ret.Warn("pipe input buffer full")
		}
		return
	}
	return ret
}

type Transformer struct {
	m7s.DefaultTransformer
	TransRule
	rBuf chan []byte
	*util.BufReader
	flv.Live
}

func (t *Transformer) Start() (err error) {
	err = t.TransformJob.Subscribe()
	if err != nil {
		return
	}
	if t.TransformJob.Config.Input != nil {
		switch v := t.TransformJob.Config.Input.(type) {
		case map[string]any:
			config.Parse(&t.TransRule.From, v)
		case string:
			t.From.Args = v
		}
	}
	args := append([]string{"-f", "flv"}, strings.Fields(t.From.Args)...)
	if t.From.Codec != "" {
		args = append(args, "-c:v", t.From.Codec)
	}
	args = append(args, "-i", "pipe:0")
	t.To = make([]EncodeConfig, len(t.TransformJob.Config.Output))
	for i, to := range t.TransformJob.Config.Output {
		var enc EncodeConfig
		if to.Conf != nil {
			switch v := to.Conf.(type) {
			case map[string]any:
				config.Parse(&enc, v)
			case string:
				enc.Args = v
			}
		}
		t.To[i] = enc
		args = append(args, strings.Fields(enc.Args)...)
		var targetUrl *url.URL
		targetUrl, err = url.Parse(to.Target)
		if err != nil {
			return
		}
		switch targetUrl.Scheme {
		case "rtmp":
			args = append(args, "-f", "flv", to.Target)
		case "rtsp":
			args = append(args, "-f", "rtsp", to.Target)
		default:
			args = append(args, to.Target)
		}
	}
	t.Description = task.Description{
		"cmd":    args,
		"config": t.TransRule,
	}
	t.rBuf = make(chan []byte, 100)
	t.BufReader = util.NewBufReaderChan(t.rBuf)
	t.Subscriber = t.TransformJob.Subscriber
	//t.BufReader.Dump, err = os.OpenFile("dump.flv", os.O_CREATE|os.O_WRONLY, 0644)
	var cmdTask CommandTask
	cmdTask.logFileName = fmt.Sprintf("logs/transcode_%s_%s.log", strings.ReplaceAll(t.TransformJob.StreamPath, "/", "_"), time.Now().Format("20060102150405"))
	cmdTask.Cmd = exec.CommandContext(t, "ffmpeg", args...)
	cmdTask.Cmd.Stdin = t.BufReader
	t.AddTask(&cmdTask)
	return
}

func (t *Transformer) Dispose() {
	close(t.rBuf)
	t.BufReader.Recycle()
}
