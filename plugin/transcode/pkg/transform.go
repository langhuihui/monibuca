package transcode

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"m7s.live/v5/pkg"
	"m7s.live/v5/pkg/filerotate"

	m7s "m7s.live/v5"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/pkg/task"
	"m7s.live/v5/pkg/util"
	flv "m7s.live/v5/plugin/flv/pkg"
)

// / 定义传输模式的常量
const (
	TRANS_MODE_PIPE   TransMode = "pipe"
	TRANS_MODE_RTSP   TransMode = "rtsp"
	TRANS_MODE_RTMP   TransMode = "rtmp"
	TRANS_MODE_LIB    TransMode = "lib"
	TRANS_MODE_REMOTE TransMode = "remote"
)

type (
	TransMode    = string
	DecodeConfig struct {
		Mode   TransMode `default:"pipe" json:"mode" desc:"转码模式"` //转码模式
		Codec  string    `json:"codec" desc:"解码器"`
		Args   string    `json:"args" desc:"解码参数"`
		Remote string    `json:"remote" desc:"远程地址"`
	}
	EncodeConfig struct {
		Codec string `json:"codec" desc:"编码器"`
		Args  string `json:"args" desc:"编码参数"`
		Dest  string `json:"dest" desc:"目标主机路径"`
	}
	TransRule struct {
		From      DecodeConfig   `json:"from"`
		To        []EncodeConfig `json:"to" desc:"编码配置"`            //目标
		LogToFile string         `json:"logtofile" desc:"转码是否写入日志"` //转码日志写入文件
	}
)

func NewTransform() m7s.ITransformer {
	ret := &Transformer{}
	ret.SetDescription(task.OwnerTypeKey, "Transcode")
	return ret
}

type Transformer struct {
	m7s.DefaultTransformer
	TransRule
	logFile *filerotate.File
	ffmpeg  *exec.Cmd
}

func (t *Transformer) Start() (err error) {
	if t.TransformJob.Plugin.Config.Has("LogToFile") {
		t.TransRule.LogToFile = t.TransformJob.Plugin.Config.Get("LogToFile").GetValue().(string)
	}
	if t.TransformJob.Config.Input != nil {
		switch v := t.TransformJob.Config.Input.(type) {
		case DecodeConfig:
			t.From = v
		case map[string]any:
			config.Parse(&t.TransRule.From, v)
		case string:
			t.From.Mode = TRANS_MODE_PIPE
			t.From.Args = v
		}
	}
	if t.From.Mode == "" {
		t.From.Mode = TRANS_MODE_PIPE
	}
	args := strings.Fields(t.From.Args)
	if t.From.Codec != "" {
		args = append(args, "-c:v", t.From.Codec)
	}
	switch t.From.Mode {
	case TRANS_MODE_PIPE:
		err = t.TransformJob.Subscribe()
		if err != nil {
			return
		}
		args = append(args, "-f", "flv", "-i", "pipe:0")
	case TRANS_MODE_RTSP:
		if rtspPlugin, ok := t.TransformJob.Plugin.Server.Plugins.Get("RTSP"); ok {
			listenAddr := rtspPlugin.GetCommonConf().TCP.ListenAddr
			if strings.HasPrefix(listenAddr, ":") {
				listenAddr = "localhost" + listenAddr
			}
			args = append(args, "-i", "rtsp://"+listenAddr+"/"+t.TransformJob.StreamPath)
		}
	case TRANS_MODE_RTMP:
		if rtmpPlugin, ok := t.TransformJob.Plugin.Server.Plugins.Get("RTMP"); ok {
			listenAddr := rtmpPlugin.GetCommonConf().TCP.ListenAddr
			if strings.HasPrefix(listenAddr, ":") {
				listenAddr = "localhost" + listenAddr
			}
			args = append(args, "-i", "rtmp://"+listenAddr+"/"+t.TransformJob.StreamPath)
		}
	case TRANS_MODE_REMOTE:
		args = append(args, "-i", t.From.Remote)
	}
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
		//if to.Overlay != "" {
		//	args = append(args, "-i", to.Overlay)
		//}
		//if to.Filter != "" {
		//	args = append(args, "-filter_complex", strings.ReplaceAll(to.Filter, "\n", ""))
		//	args = append(args, "-map", "[out]")
		//	args = append(args, "-map", "0:a")
		//}
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
		case "srt":
			args = append(args, "-f", "mpegts", to.Target)
		default:
			args = append(args, to.Target)
		}
	}
	t.SetDescription("cmd", args)
	t.SetDescription("config", t.TransRule)
	//t.BufReader.Dump, err = os.OpenFile("dump.flv", os.O_CREATE|os.O_WRONLY, 0644)
	t.ffmpeg = exec.CommandContext(t, "ffmpeg", args...)
	if t.TransRule.LogToFile != "" {
		logFileName := fmt.Sprintf(t.TransRule.LogToFile, strings.ReplaceAll(t.TransformJob.StreamPath, "/", "_"))
		t.SetDescription("log", logFileName)
		t.logFile, err = filerotate.NewDaily("logs", logFileName, nil)
		if err != nil {
			t.Error("Could not create transcode log", "err", err)
			return err
		}
		// 将命令的标准输出和标准错误输出重定向到日志文件
		t.ffmpeg.Stdout = t.logFile
		t.ffmpeg.Stderr = t.logFile

	} else {
		// 将命令的标准输出和标准错误输出重定向到操作系统的标准输出和标准错误输出
		t.ffmpeg.Stdout = os.Stdout
		t.ffmpeg.Stderr = os.Stderr
	}
	t.Info("start exec", "cmd", t.ffmpeg.String())
	return
}

func (t *Transformer) Go() error {
	if t.From.Mode == "pipe" {
		bufReader := util.NewBufReaderChan(100)
		t.ffmpeg.Stdin = bufReader
		var live flv.Live
		live.Subscriber = t.TransformJob.Subscriber
		var bufferFull time.Time
		live.WriteFlvTag = func(flv net.Buffers) (err error) {
			var buffer []byte
			for _, b := range flv {
				buffer = append(buffer, b...)
			}
			if bufReader.Feed(buffer) {
				bufferFull = time.Now()
			} else {
				t.Warn("pipe input buffer full")
				if time.Since(bufferFull) > time.Second*5 {
					t.Stop(bufio.ErrBufferFull)
				}
			}
			return
		}
		defer bufReader.Recycle()
		err := t.ffmpeg.Start()
		if err != nil {
			return err
		}
		t.SetDescription("pid", t.ffmpeg.Process.Pid)
		return live.Run()
	} else {
		err := t.ffmpeg.Start()
		if err != nil {
			return err
		}
		t.SetDescription("pid", t.ffmpeg.Process.Pid)
		if err := t.ffmpeg.Wait(); err != nil {
			return err
		}
		return pkg.ErrRestart
	}
}

func (t *Transformer) Dispose() {
	err := t.ffmpeg.Process.Kill()
	t.Error("kill ffmpeg", "err", err)
	if t.logFile != nil {
		_ = t.logFile.Close()
	}
}
